package worker_test

import (
	"context"
	"strings"
	"testing"

	"journaletto/internal/provider"
	"journaletto/internal/worker"
)

// keywordEmbedder places text on an axis by keyword so nearest-neighbour results are predictable.
type keywordEmbedder struct{ provider.Fake }

func (keywordEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	low := strings.ToLower(text)
	switch {
	case strings.Contains(low, "zebra"):
		return []float32{1, 0, 0}, nil
	case strings.Contains(low, "ada"):
		return []float32{0, 1, 0}, nil
	}
	return []float32{0, 0, 1}, nil
}

func TestEveryChunkEmbeddedWithProvenanceAndNearestNeighbourFindsExpectedChunk(t *testing.T) {
	db, svc := setup(t)
	id := saveBody(t, svc, "Weekly review", "# Plan\nShip the zebra feature.\n\n# Notes\nAda Lovelace reviewed it.")
	start(t, &worker.Worker{DB: db, Steps: worker.DefaultSteps(db, keywordEmbedder{}), MaxRetries: 0})
	waitStatus(t, db, id, "ready")

	chunks := count(t, db, `SELECT COUNT(*) FROM chunks WHERE entry_id=?`, id)
	if n := count(t, db, `SELECT COUNT(*) FROM chunk_embeddings`); n != chunks || n == 0 {
		t.Fatalf("embeddings = %d, chunks = %d", n, chunks)
	}
	var prov, model string
	var dims int
	if err := db.QueryRow(`SELECT provider, model, dims FROM chunk_embeddings LIMIT 1`).Scan(&prov, &model, &dims); err != nil {
		t.Fatal(err)
	}
	if prov != "fake" || model == "" || dims != 3 {
		t.Errorf("provenance: %q %q %d", prov, model, dims)
	}

	hits, err := worker.NearestChunks(context.Background(), db, []float32{0, 0.9, 0.1}, 1)
	if err != nil || len(hits) != 1 {
		t.Fatalf("hits = %v, %v", hits, err)
	}
	var text string
	if err := db.QueryRow(`SELECT text FROM chunks WHERE id=?`, hits[0].ChunkID).Scan(&text); err != nil || !strings.Contains(text, "Ada") {
		t.Errorf("nearest chunk = %q, %v", text, err)
	}
}

func TestReprocessingReplacesEmbeddingsWithoutDuplicates(t *testing.T) {
	db, svc := setup(t)
	id := saveBody(t, svc, "Weekly review", "# Plan\nShip the zebra feature.")
	start(t, &worker.Worker{DB: db, Steps: worker.DefaultSteps(db, keywordEmbedder{}), MaxRetries: 0})
	waitStatus(t, db, id, "ready")
	before := count(t, db, `SELECT COUNT(*) FROM chunk_embeddings`)

	if err := worker.Resync(context.Background(), db, "u", id); err != nil {
		t.Fatal(err)
	}
	waitDone(t, db, id, 2)
	if after := count(t, db, `SELECT COUNT(*) FROM chunk_embeddings`); after != before {
		t.Errorf("embeddings %d -> %d after reprocess", before, after)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM chunk_embeddings e LEFT JOIN chunks c ON c.id=e.chunk_id WHERE c.id IS NULL`); n != 0 {
		t.Errorf("orphan embeddings: %d", n)
	}
}
