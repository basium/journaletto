package worker

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"math"
	"sort"
	"time"

	"journaletto/internal/provider"
)

// Embeddings are sensitive: they are never logged, and errors here never include vector contents.

func encodeVector(v []float32) []byte {
	b := make([]byte, 4*len(v))
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	return b
}

func decodeVector(b []byte) []float32 {
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}

// EmbeddingsStep embeds every chunk of the entry (parent context + text) with the provider and
// stores the vectors with provenance, replacing any earlier ones for the entry in one transaction.
// Run it after ChunksStep.
func EmbeddingsStep(db *sql.DB, p provider.Provider) Step {
	return Step{Name: "embeddings", Run: func(ctx context.Context, e Entry) error {
		type chunk struct {
			id   int64
			text string
		}
		rows, err := db.QueryContext(ctx, `SELECT id, context, text FROM chunks WHERE entry_id=? ORDER BY ord`, e.ID)
		if err != nil {
			return err
		}
		var chunks []chunk
		for rows.Next() {
			var c chunk
			var cx string
			if err := rows.Scan(&c.id, &cx, &c.text); err != nil {
				rows.Close()
				return err
			}
			c.text = cx + "\n" + c.text
			chunks = append(chunks, c)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		rows.Close()

		vecs := make([][]float32, len(chunks))
		for i, c := range chunks {
			v, err := p.Embed(ctx, c.text)
			if err != nil {
				return err
			}
			if len(v) == 0 {
				return errors.New("embeddings: empty vector from provider")
			}
			vecs[i] = v
		}
		name, model := p.Identity()
		now := time.Now().UTC().Format(time.RFC3339)
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, `DELETE FROM chunk_embeddings WHERE entry_id=?`, e.ID); err != nil {
			return err
		}
		for i, c := range chunks {
			if _, err := tx.ExecContext(ctx, `INSERT INTO chunk_embeddings(chunk_id,entry_id,dims,vector,provider,model,generated_at)
				VALUES(?,?,?,?,?,?,?)`, c.id, e.ID, len(vecs[i]), encodeVector(vecs[i]), name, model, now); err != nil {
				return err
			}
		}
		return tx.Commit()
	}}
}

// Hit is a nearest-neighbour result; Score is cosine similarity (higher is closer).
type Hit struct {
	ChunkID int64
	EntryID string
	Score   float64
}

// NearestChunks returns the k chunks whose stored embedding is most similar (cosine) to query.
// Vectors of a different dimension are skipped. This is a brute-force scan over stored vectors.
func NearestChunks(ctx context.Context, db *sql.DB, query []float32, k int) ([]Hit, error) {
	var qn float64
	for _, f := range query {
		qn += float64(f) * float64(f)
	}
	if qn == 0 || k <= 0 {
		return nil, nil
	}
	qn = math.Sqrt(qn)
	rows, err := db.QueryContext(ctx, `SELECT chunk_id, entry_id, vector FROM chunk_embeddings WHERE dims=?`, len(query))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []Hit
	for rows.Next() {
		var h Hit
		var blob []byte
		if err := rows.Scan(&h.ChunkID, &h.EntryID, &blob); err != nil {
			return nil, err
		}
		var dot, vn float64
		for i, f := range decodeVector(blob) {
			dot += float64(f) * float64(query[i])
			vn += float64(f) * float64(f)
		}
		if vn == 0 {
			continue
		}
		h.Score = dot / (qn * math.Sqrt(vn))
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	if len(hits) > k {
		hits = hits[:k]
	}
	return hits, nil
}
