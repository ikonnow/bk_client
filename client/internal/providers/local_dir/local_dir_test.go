package local_dir

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blenderkit/blenderkit/client/internal/index"
	"github.com/blenderkit/blenderkit/client/internal/settings"
)

type fakeCfg struct{ vars map[string]string }

func newFakeCfg() *fakeCfg { return &fakeCfg{vars: map[string]string{}} }

func (f *fakeCfg) GetVariable(_, variable string) (string, bool) {
	v, ok := f.vars[variable]
	return v, ok
}

func (f *fakeCfg) SetVariable(_, variable, value string) (settings.Snapshot, error) {
	f.vars[variable] = value
	return settings.Snapshot{}, nil
}

func serve(t *testing.T, p *Provider, method, path, body string) (int, map[string]any) {
	t.Helper()
	mux := http.NewServeMux()
	p.RegisterRoutes(mux, "v1.13")
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	out := map[string]any{}
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

// The env var is a read-only dev fallback — used only while nothing is stored.
func TestEnvFallbackOnlyWhenUnset(t *testing.T) {
	root := t.TempDir()
	t.Setenv(EnvVar, root)
	p := New(nil, newFakeCfg())

	_, resp := serve(t, p, http.MethodGet, "/providers/local_dir/status", "")
	if resp["roots_source"] != "env" {
		t.Fatalf("roots_source = %v, want env", resp["roots_source"])
	}

	// Once the user stores a list, env no longer applies.
	raw, _ := json.Marshal(map[string]any{"roots": []string{}})
	serve(t, p, http.MethodPost, "/providers/local_dir/roots", string(raw))
	_, resp = serve(t, p, http.MethodGet, "/providers/local_dir/status", "")
	if resp["roots_source"] != "configured" {
		t.Fatalf("stored empty must override env, got %v", resp["roots_source"])
	}
	if roots := resp["roots"].([]any); len(roots) != 0 {
		t.Fatalf("explicit empty must show zero roots, got %v", roots)
	}
}

// End-to-end through the handler: configured root → scan → index hit.
func TestConfiguredRootScansAndIndexes(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "wood.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()

	p := New(idx, newFakeCfg())
	raw, _ := json.Marshal(map[string]any{"roots": []string{root}})
	serve(t, p, http.MethodPost, "/providers/local_dir/roots", string(raw))

	_, resp := serve(t, p, http.MethodGet, "/providers/local_dir/scan", "")
	sum := resp["scanned"].(map[string]any)
	if sum["files"].(float64) != 1 {
		t.Fatalf("files = %v, want 1", sum["files"])
	}
	hits, err := idx.Search(t.Context(), "local_dir", "wood", 10)
	if err != nil || len(hits) != 1 || hits[0].Kind != "texture" {
		t.Fatalf("index search: hits=%v err=%v", hits, err)
	}
}

func TestRootsPostBadJSON(t *testing.T) {
	p := New(nil, newFakeCfg())
	code, _ := serve(t, p, http.MethodPost, "/providers/local_dir/roots", "{oops")
	if code != http.StatusBadRequest {
		t.Fatalf("bad JSON must 400, got %d", code)
	}
}

func TestRootsPostNoSettingsStore(t *testing.T) {
	p := New(nil, nil)
	code, _ := serve(t, p, http.MethodPost, "/providers/local_dir/roots", `{"roots":[]}`)
	if code != http.StatusServiceUnavailable {
		t.Fatalf("missing settings store must 503, got %d", code)
	}
}
