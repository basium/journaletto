// Package store opens SQLite and applies schema migrations.
package store

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // CGO-free SQLite driver
)

var migrations = []string{
	`CREATE TABLE entries (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		title TEXT NOT NULL,
		body TEXT NOT NULL,
		journal_date TEXT NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		timezone TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'active',
		processing_status TEXT NOT NULL DEFAULT 'pending',
		file_path TEXT NOT NULL
	);
	CREATE INDEX entries_user_date ON entries(user_id, journal_date);
	CREATE TABLE entry_tags (
		entry_id TEXT NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
		user_id TEXT NOT NULL,
		tag TEXT NOT NULL,
		PRIMARY KEY (entry_id, tag)
	);
	CREATE INDEX entry_tags_user_tag ON entry_tags(user_id, tag);
	CREATE VIRTUAL TABLE entries_fts USING fts5(entry_id UNINDEXED, title, body, tags);`,
	// Background job queue: one row per processing run of an entry.
	// state: queued | running | done | failed. run_after is unix milliseconds.
	`CREATE TABLE jobs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		entry_id TEXT NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
		state TEXT NOT NULL DEFAULT 'queued',
		attempts INTEGER NOT NULL DEFAULT 0,
		run_after INTEGER NOT NULL DEFAULT 0,
		last_error TEXT NOT NULL DEFAULT ''
	);
	CREATE UNIQUE INDEX jobs_one_active ON jobs(entry_id) WHERE state IN ('queued','running');
	CREATE INDEX jobs_claim ON jobs(state, run_after);`,
	// Generated artefacts (summary, internal tags) with provenance. Never written to the raw
	// entry file and not user-editable; one current row per (entry, kind).
	`CREATE TABLE entry_artifacts (
		entry_id TEXT NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
		kind TEXT NOT NULL,
		content TEXT NOT NULL,
		provider TEXT NOT NULL,
		model TEXT NOT NULL,
		prompt_version TEXT NOT NULL,
		generated_at TEXT NOT NULL,
		PRIMARY KEY (entry_id, kind)
	);`,
	// Extracted entities and relations (graph source data) with provenance. Entities are unique
	// per (entry, type, normalised value); relations are unique per (from, to, kind).
	`CREATE TABLE entities (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		entry_id TEXT NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
		type TEXT NOT NULL,
		value TEXT NOT NULL,
		norm_value TEXT NOT NULL,
		provider TEXT NOT NULL,
		model TEXT NOT NULL,
		prompt_version TEXT NOT NULL,
		generated_at TEXT NOT NULL,
		UNIQUE (entry_id, type, norm_value)
	);
	CREATE TABLE entity_relations (
		from_id INTEGER NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
		to_id INTEGER NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
		kind TEXT NOT NULL,
		provider TEXT NOT NULL,
		model TEXT NOT NULL,
		prompt_version TEXT NOT NULL,
		generated_at TEXT NOT NULL,
		PRIMARY KEY (from_id, to_id, kind)
	);
	CREATE INDEX entities_type_value ON entities(type, norm_value);`,
	// Semantic chunks: granularity section|fact, each with parent context (entry title and
	// heading path) and provenance; text is exact-indexed in chunks_fts. chunk_entities links a
	// chunk to the entities it mentions. Replaced wholesale per entry on reprocessing.
	`CREATE TABLE chunks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			entry_id TEXT NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
			granularity TEXT NOT NULL CHECK (granularity IN ('section','fact')),
			ord INTEGER NOT NULL,
			title TEXT NOT NULL,
			heading_path TEXT NOT NULL,
			context TEXT NOT NULL,
			text TEXT NOT NULL,
			provider TEXT NOT NULL,
			model TEXT NOT NULL,
			prompt_version TEXT NOT NULL,
			generated_at TEXT NOT NULL
		);
		CREATE INDEX chunks_entry ON chunks(entry_id, ord);
		CREATE TABLE chunk_entities (
			chunk_id INTEGER NOT NULL REFERENCES chunks(id) ON DELETE CASCADE,
			entity_id INTEGER NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
			PRIMARY KEY (chunk_id, entity_id)
		);
		CREATE VIRTUAL TABLE chunks_fts USING fts5(chunk_id UNINDEXED, entry_id UNINDEXED, text);`,
	// Chunk embeddings with provenance. Vectors are little-endian float32 BLOBs searched by
	// brute-force cosine similarity (sqlite-vec is not available with the CGO-free driver).
	// Deleted with their chunk, so reprocessing never leaves duplicates.
	`CREATE TABLE chunk_embeddings (
		chunk_id INTEGER PRIMARY KEY REFERENCES chunks(id) ON DELETE CASCADE,
		entry_id TEXT NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
		dims INTEGER NOT NULL,
		vector BLOB NOT NULL,
		provider TEXT NOT NULL,
		model TEXT NOT NULL,
		generated_at TEXT NOT NULL
	);
	CREATE INDEX chunk_embeddings_entry ON chunk_embeddings(entry_id);`,
}

// Open opens (creating if needed) the database with foreign keys on and migrates it.
func Open(path string) (*sql.DB, error) {
	dsn := "file:" + path + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func migrate(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY)`); err != nil {
		return err
	}
	var cur int
	if err := db.QueryRow(`SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&cur); err != nil {
		return err
	}
	for i := cur; i < len(migrations); i++ {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations(version) VALUES (?)`, i+1); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
