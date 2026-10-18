package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"journaletto/internal/provider"
)

// Prompt/parser versions are recorded with each chunk; bump when behaviour changes.
const (
	FactsPromptVersion = "facts-v1"
	SectionsVersion    = "sections-v1"
)

// Chunk granularities.
const (
	GranularitySection = "section"
	GranularityFact    = "fact"
)

var factsPrompt = provider.FactPromptPrefix + ` from this journal entry. Each fact must be a short, self-contained sentence that
is understandable without the rest of the entry (name people and things instead of using pronouns). Answer with only a JSON
array of {"fact":F,"section":S} where S is the heading of the section the fact comes from, or "" if none.

`

type section struct {
	path []string
	text string
}

// splitSections splits markdown at headings. Text before the first heading is its own section
// with an empty heading path. Sections with no text of their own are skipped; there is no size limit.
func splitSections(body string) []section {
	var out []section
	var stack []string
	var cur []string
	var curPath []string
	flush := func() {
		text := strings.TrimSpace(strings.Join(cur, "\n"))
		// A heading line alone carries no content.
		if strings.TrimSpace(strings.Join(cur[min(len(cur), headingLines(cur)):], "\n")) != "" {
			out = append(out, section{path: curPath, text: text})
		}
		cur = nil
	}
	inFence := false
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
		}
		if level, title, ok := parseHeading(line); ok && !inFence {
			flush()
			stack = stack[:min(len(stack), level-1)]
			for len(stack) < level-1 { // skipped levels (e.g. # then ###): keep depth consistent
				stack = append(stack, "")
			}
			stack = append(stack, title)
			curPath = compact(stack)
			cur = []string{line}
			continue
		}
		cur = append(cur, line)
	}
	flush()
	return out
}

// headingLines is 1 when the first line of cur is a heading, else 0.
func headingLines(cur []string) int {
	if len(cur) > 0 {
		if _, _, ok := parseHeading(cur[0]); ok {
			return 1
		}
	}
	return 0
}

func parseHeading(line string) (level int, title string, ok bool) {
	for level < len(line) && line[level] == '#' {
		level++
	}
	if level == 0 || level > 6 || level >= len(line) || line[level] != ' ' {
		return 0, "", false
	}
	title = normSpace(strings.TrimRight(strings.TrimSpace(line[level:]), "# "))
	return level, title, title != ""
}

func compact(stack []string) []string {
	out := make([]string, 0, len(stack))
	for _, s := range stack {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func chunkContext(title, path string) string {
	if path == "" {
		return "Entry: " + title
	}
	return "Entry: " + title + " | Section: " + path
}

type rawFact struct{ Fact, Section string }

// UnmarshalJSON accepts either a bare string or {"fact":..., "section":...}.
func (f *rawFact) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		f.Fact = s
		return nil
	}
	var o struct {
		Fact    string `json:"fact"`
		Section string `json:"section"`
	}
	if err := json.Unmarshal(b, &o); err != nil {
		return err
	}
	f.Fact, f.Section = o.Fact, o.Section
	return nil
}

// ChunksStep splits an entry into section chunks (following headings) and small factual chunks
// (from the LLM), each carrying its parent context, and replaces any earlier chunks of the entry
// in one transaction so reprocessing never duplicates. Text is indexed for exact search in
// chunks_fts. Run it after EntitiesStep so chunks can be linked to the entities they mention.
func ChunksStep(db *sql.DB, p provider.Provider) Step {
	return Step{Name: "chunks", Run: func(ctx context.Context, e Entry) error {
		out, err := p.Complete(ctx, factsPrompt+e.Title+"\n\n"+e.Body)
		if err != nil {
			return err
		}
		var facts []rawFact
		if err := json.Unmarshal([]byte(stripFence(out)), &facts); err != nil {
			return fmt.Errorf("chunks: invalid JSON: %w", err)
		}
		name, model := p.Identity()
		now := time.Now().UTC().Format(time.RFC3339)
		secs := splitSections(e.Body)
		known := map[string]string{} // lowercase heading -> full path
		for _, s := range secs {
			if len(s.path) > 0 {
				known[strings.ToLower(s.path[len(s.path)-1])] = strings.Join(s.path, " > ")
			}
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, `DELETE FROM chunks_fts WHERE entry_id=?`, e.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM chunks WHERE entry_id=?`, e.ID); err != nil { // cascades chunk_entities
			return err
		}
		ents, err := entityValues(ctx, tx, e.ID)
		if err != nil {
			return err
		}
		ord := 0
		add := func(gran, path, text, prov, mdl, ver string) error {
			ord++
			var id int64
			if err := tx.QueryRowContext(ctx, `INSERT INTO chunks(entry_id,granularity,ord,title,heading_path,context,text,provider,model,prompt_version,generated_at)
				VALUES(?,?,?,?,?,?,?,?,?,?,?) RETURNING id`,
				e.ID, gran, ord, e.Title, path, chunkContext(e.Title, path), text, prov, mdl, ver, now).Scan(&id); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO chunks_fts(chunk_id,entry_id,text) VALUES(?,?,?)`, id, e.ID, text); err != nil {
				return err
			}
			low := strings.ToLower(text)
			for entID, val := range ents {
				if strings.Contains(low, val) {
					if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO chunk_entities(chunk_id,entity_id) VALUES(?,?)`, id, entID); err != nil {
						return err
					}
				}
			}
			return nil
		}
		for _, s := range secs {
			if err := add(GranularitySection, strings.Join(s.path, " > "), s.text, "parser", "headings", SectionsVersion); err != nil {
				return err
			}
		}
		seen := map[string]bool{}
		for _, f := range facts {
			text := normSpace(f.Fact)
			if text == "" || seen[strings.ToLower(text)] {
				continue
			}
			seen[strings.ToLower(text)] = true
			path := known[strings.ToLower(normSpace(f.Section))]
			if err := add(GranularityFact, path, text, name, model, FactsPromptVersion); err != nil {
				return err
			}
		}
		return tx.Commit()
	}}
}

// entityValues maps the entry's entity ids to their lowercase values.
func entityValues(ctx context.Context, tx *sql.Tx, entryID string) (map[int64]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, norm_value FROM entities WHERE entry_id=?`, entryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[int64]string{}
	for rows.Next() {
		var id int64
		var v string
		if err := rows.Scan(&id, &v); err != nil {
			return nil, err
		}
		m[id] = v
	}
	return m, rows.Err()
}
