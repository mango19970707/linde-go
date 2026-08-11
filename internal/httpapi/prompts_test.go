package httpapi

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"linde-go-v2/internal/graph"
)

func TestPromptAPIListUpdateAndReset(t *testing.T) {
	t.Parallel()

	db := mustOpenPromptDB(t, filepath.Join(t.TempDir(), "prompts.db"))
	store, err := graph.NewSQLitePromptStore(db)
	if err != nil {
		t.Fatalf("NewSQLitePromptStore() error = %v", err)
	}

	server := NewWorkPermitServer(nil, store, 28, 464)

	listRec := doPromptRequest(t, server, http.MethodGet, "/api/prompts", nil)
	if listRec.Code != http.StatusOK {
		t.Fatalf("GET /api/prompts status = %d", listRec.Code)
	}
	var listed []graph.PromptDefinition
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("unmarshal list response: %v", err)
	}
	if len(listed) != 4 {
		t.Fatalf("len(listed) = %d", len(listed))
	}

	updateBody := []byte(`{"content":"new prompt content"}`)
	updateRec := doPromptRequest(t, server, http.MethodPut, "/api/prompts/wp_type_recognition", updateBody)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("PUT /api/prompts status = %d, body = %s", updateRec.Code, updateRec.Body.String())
	}

	getRec := doPromptRequest(t, server, http.MethodGet, "/api/prompts/wp_type_recognition", nil)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /api/prompts/wp_type_recognition status = %d", getRec.Code)
	}
	var got graph.PromptDefinition
	if err := json.Unmarshal(getRec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal get response: %v", err)
	}
	if got.Content != "new prompt content" {
		t.Fatalf("got.Content = %q", got.Content)
	}

	resetRec := doPromptRequest(t, server, http.MethodPost, "/api/prompts/wp_type_recognition/reset", nil)
	if resetRec.Code != http.StatusOK {
		t.Fatalf("POST /api/prompts/wp_type_recognition/reset status = %d", resetRec.Code)
	}

	afterReset := doPromptRequest(t, server, http.MethodGet, "/api/prompts/wp_type_recognition", nil)
	var reset graph.PromptDefinition
	if err := json.Unmarshal(afterReset.Body.Bytes(), &reset); err != nil {
		t.Fatalf("unmarshal reset response: %v", err)
	}
	if reset.Content == "new prompt content" {
		t.Fatal("reset did not restore default prompt")
	}
}

func doPromptRequest(t *testing.T, server *WorkPermitServer, method, target string, body []byte) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	server.Router().ServeHTTP(rec, req)
	return rec
}

func mustOpenPromptDB(t *testing.T, path string) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}
