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

// EntitiesPromptVersion is recorded with each extracted entity; bump when the prompt changes.
const EntitiesPromptVersion = "entities-v1"

// EntityTypes are the 16 supported entity types (JE-008).
var EntityTypes = []string{
	"Projects", "People", "Brands", "Features", "Releases", "Tickets", "Commits", "Merges",
	"Pull Requests", "Companies", "Systems", "Decisions", "Incidents", "Tasks", "Concepts", "Dates",
}

var entitiesPrompt = provider.EntityPromptPrefix + ` from this journal entry. Answer with only a JSON array of
{"type":T,"value":V,"relations":[{"to":V2,"kind":K}]} where T is one of: ` + strings.Join(EntityTypes, ", ") +
	`; relations point to the value of another entity in the same array and K is a short lowercase verb phrase.

`

type rawEntity struct {
	Type      string `json:"type"`
	Value     string `json:"value"`
	Relations []struct {
		To   string `json:"to"`
		Kind string `json:"kind"`
	} `json:"relations"`
}

func normSpace(s string) string { return strings.Join(strings.Fields(s), " ") }

func canonicalType(t string) (string, bool) {
	t = normSpace(t)
	for _, c := range EntityTypes {
		if strings.EqualFold(c, t) {
			return c, true
		}
	}
	return "", false
}

func stripFence(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimPrefix(s, "json")
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	}
	return strings.TrimSpace(s)
}

// EntitiesStep extracts entities and relations from an entry and stores them, replacing any
// earlier extraction of the same entry, so reprocessing never duplicates rows. Entities with an
// unknown type or blank value are rejected, as are relations to entities that were not extracted.
func EntitiesStep(db *sql.DB, p provider.Provider) Step {
	return Step{Name: "entities", Run: func(ctx context.Context, e Entry) error {
		out, err := p.Complete(ctx, entitiesPrompt+e.Title+"\n\n"+e.Body)
		if err != nil {
			return err
		}
		var raw []rawEntity
		if err := json.Unmarshal([]byte(stripFence(out)), &raw); err != nil {
			return fmt.Errorf("entities: invalid JSON: %w", err)
		}
		name, model := p.Identity()
		now := time.Now().UTC().Format(time.RFC3339)
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, `DELETE FROM entities WHERE entry_id=?`, e.ID); err != nil {
			return err
		}
		ids := map[string]int64{} // normalised value -> id of first accepted entity
		type rel struct{ from, to, kind string }
		var rels []rel
		for _, r := range raw {
			typ, ok := canonicalType(r.Type)
			val := normSpace(r.Value)
			if !ok || val == "" {
				continue
			}
			norm := strings.ToLower(val)
			var id int64
			err := tx.QueryRowContext(ctx, `INSERT INTO entities(entry_id,type,value,norm_value,provider,model,prompt_version,generated_at)
				VALUES(?,?,?,?,?,?,?,?)
				ON CONFLICT(entry_id,type,norm_value) DO UPDATE SET generated_at=excluded.generated_at
				RETURNING id`, e.ID, typ, val, norm, name, model, EntitiesPromptVersion, now).Scan(&id)
			if err != nil {
				return err
			}
			if _, seen := ids[norm]; !seen {
				ids[norm] = id
			}
			for _, rr := range r.Relations {
				kind := strings.ToLower(normSpace(rr.Kind))
				to := strings.ToLower(normSpace(rr.To))
				if kind != "" && to != "" {
					rels = append(rels, rel{norm, to, kind})
				}
			}
		}
		for _, r := range rels {
			from, ok1 := ids[r.from]
			to, ok2 := ids[r.to]
			if !ok1 || !ok2 {
				continue
			}
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO entity_relations(from_id,to_id,kind,provider,model,prompt_version,generated_at)
				VALUES(?,?,?,?,?,?,?)`, from, to, r.kind, name, model, EntitiesPromptVersion, now); err != nil {
				return err
			}
		}
		return tx.Commit()
	}}
}
