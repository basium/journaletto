package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"journaletto/internal/config"
	"journaletto/internal/server"
)

func TestHealthz(t *testing.T) {
	h := server.New(config.Config{Version: "1.2.3", Commit: "abc"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" || body["version"] != "1.2.3" || body["commit"] != "abc" {
		t.Fatalf("unexpected body %v", body)
	}
}

func TestHomeServesHTMX(t *testing.T) {
	h := server.New(config.Config{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "hx-get") {
		t.Fatalf("status %d body %q", rec.Code, rec.Body.String())
	}
}
