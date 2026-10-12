// Package httpapi wires HTTP handlers to the journal domain package.
package httpapi

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/basium/journaletto/internal/journal"
)

// EntryDeps are the dependencies an entries handler needs from the rest of
// the application.
type EntryDeps struct {
	JournalRoot   string
	DevUserID     string
	MaxTags       int
	MaxEntryBytes int
}

type createEntryRequest struct {
	Title       string   `json:"title"`
	JournalDate string   `json:"journal_date"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Timezone    string   `json:"timezone"`
}

type createEntryResponse struct {
	ID          string `json:"id"`
	JournalDate string `json:"journal_date"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	Path        string `json:"path"`
	Status      string `json:"status"`
}

type errorResponse struct {
	Error string `json:"error"`
	Field string `json:"field,omitempty"`
}

// CreateEntry handles POST /api/entries: it validates the submission,
// persists it as a raw Markdown file (JE-002..JE-005, JE-012), and returns
// the entry's identity and location.
func (d EntryDeps) CreateEntry(w http.ResponseWriter, r *http.Request) {
	var req createEntryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "request body is not valid JSON", "")
		return
	}

	now := time.Now().UTC()

	input := journal.Input{
		Title:       req.Title,
		JournalDate: req.JournalDate,
		Description: req.Description,
		Tags:        req.Tags,
		Timezone:    req.Timezone,
	}
	norm, err := journal.ValidateAndNormalize(input, d.MaxTags, now)
	if err != nil {
		writeValidationOrServerError(w, err)
		return
	}

	id, err := uuid.NewV7()
	if err != nil {
		log.Printf("generating entry id: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to generate entry id", "")
		return
	}

	entry := journal.Entry{
		ID:          id.String(),
		Title:       norm.Title,
		JournalDate: norm.JournalDate,
		Description: norm.Description,
		Tags:        norm.Tags,
		Timezone:    norm.Timezone,
		CreatedAt:   now,
	}

	relPath, err := journal.Save(d.JournalRoot, d.DevUserID, entry, d.MaxEntryBytes)
	if err != nil {
		writeValidationOrServerError(w, err)
		return
	}

	createdAt := entry.CreatedAt.UTC().Format(time.RFC3339)
	resp := createEntryResponse{
		ID:          entry.ID,
		JournalDate: entry.JournalDate,
		CreatedAt:   createdAt,
		UpdatedAt:   createdAt,
		Path:        relPath,
		// Processing (enrichment, indexing) is not implemented yet: this
		// reflects that the raw file is safely persisted, nothing more.
		// It is not tracked in any durable store (see JE-012 scoping note
		// in docs/decisions/0001-slice1-entry-save.md).
		Status: "pending",
	}
	writeJSON(w, http.StatusCreated, resp)
}

func writeValidationOrServerError(w http.ResponseWriter, err error) {
	var ve *journal.ValidationError
	if errors.As(err, &ve) {
		writeError(w, http.StatusBadRequest, ve.Message, ve.Field)
		return
	}
	log.Printf("internal error: %v", err)
	writeError(w, http.StatusInternalServerError, "internal error", "")
}

func writeError(w http.ResponseWriter, status int, message, field string) {
	writeJSON(w, status, errorResponse{Error: message, Field: field})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("encoding response: %v", err)
	}
}
