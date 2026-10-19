package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"journaletto/internal/provider"
)

// Prompt versions are recorded with each generated artefact; bump when the text changes.
const (
	SummaryPromptVersion = "summary-v2"
	TagsPromptVersion    = "tags-v1"

	summaryPrompt = "Summarise this journal entry in one or two short sentences. Reply with the summary text only: no preamble, no introduction such as \"Here is a summary\", no quotes, no commentary.\n\n"
	tagsPrompt    = "List up to 8 short lowercase topic tags for this journal entry, comma separated, nothing else.\n\n"
)

// Artefact kinds stored in entry_artifacts.
const (
	KindSummary      = "summary"
	KindInternalTags = "internal_tags"
)

const maxInternalTags = 8

// DefaultSteps is the processing pipeline. T04-T07 append their stages here.
func DefaultSteps(db *sql.DB, p provider.Provider) []Step {
	return []Step{
		{
			Name: "embed",
			Run: func(ctx context.Context, e Entry) error {
				_, err := p.Embed(ctx, e.Title+"\n\n"+e.Body)
				return err
			},
		},
		SummaryTagsStep(db, p),
		EntitiesStep(db, p),
		ChunksStep(db, p),
		EmbeddingsStep(db, p),
		RulesStep(db, p),
	}
}

// SummaryTagsStep generates a compact summary and hidden internal tags and stores them in the
// database with provenance. It never touches the raw entry file.
func SummaryTagsStep(db *sql.DB, p provider.Provider) Step {
	return Step{Name: "summary-tags", Run: func(ctx context.Context, e Entry) error {
		text := e.Title + "\n\n" + e.Body
		summary, err := p.Complete(ctx, summaryPrompt+text)
		if err != nil {
			return err
		}
		summary = StripPreamble(summary)
		if summary == "" {
			return errors.New("empty summary from provider")
		}
		rawTags, err := p.Complete(ctx, tagsPrompt+text)
		if err != nil {
			return err
		}
		tags, err := json.Marshal(parseTags(rawTags))
		if err != nil {
			return err
		}
		name, model := p.Identity()
		now := time.Now().UTC().Format(time.RFC3339)
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		for _, a := range []struct{ kind, content, version string }{
			{KindSummary, summary, SummaryPromptVersion},
			{KindInternalTags, string(tags), TagsPromptVersion},
		} {
			if _, err := tx.ExecContext(ctx, `INSERT INTO entry_artifacts(entry_id,kind,content,provider,model,prompt_version,generated_at)
				VALUES(?,?,?,?,?,?,?)
				ON CONFLICT(entry_id,kind) DO UPDATE SET content=excluded.content, provider=excluded.provider,
				model=excluded.model, prompt_version=excluded.prompt_version, generated_at=excluded.generated_at`,
				e.ID, a.kind, a.content, name, model, a.version, now); err != nil {
				return err
			}
		}
		return tx.Commit()
	}}
}

// preambleRe matches chatter an LLM puts before the real answer, e.g. "Here is a summary:".
var preambleRe = regexp.MustCompile(`(?i)^(sure|certainly|of course|okay|ok|here is|here's|here are|below is|this is a summary|summary:)`)

// StripPreamble trims the text and removes a leading preamble line such as "Here is a summary of
// the entry:" when real content follows. A lone line is kept: with nothing after it, it is more
// likely the summary itself than chatter.
func StripPreamble(s string) string {
	s = strings.TrimSpace(s)
	first, rest, ok := strings.Cut(s, "\n")
	if !ok || !preambleRe.MatchString(strings.TrimSpace(first)) {
		return s
	}
	if rest = strings.TrimSpace(rest); rest == "" {
		return s
	}
	return rest
}

func parseTags(raw string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, t := range strings.Split(raw, ",") {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
		if len(out) == maxInternalTags {
			break
		}
	}
	return out
}
