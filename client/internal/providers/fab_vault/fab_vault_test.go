package fab_vault

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blenderkit/blenderkit/client/internal/settings"
)

// fakeCfg is an in-memory providers.Config for handler tests.
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

// The core safety invariant: with no configured roots, scan walks nothing —
// detected caches on the machine are not touched.
func TestScanUnsetRootsScansNothing(t *testing.T) {
	p := New(nil, newFakeCfg())
	code, resp := serve(t, p, http.MethodGet, "/providers/fab_vault/scan", "")
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	if resp["roots_source"] != "none" {
		t.Fatalf("roots_source = %v, want none", resp["roots_source"])
	}
	sum := resp["scanned"].(map[string]any)
	if sum["files"].(float64) != 0 {
		t.Fatalf("unset roots must scan zero files, got %v", sum["files"])
	}
	if resp["hint"] == nil {
		t.Fatal("scan with no roots should include an adopt hint")
	}
}

// Candidates are exposed for adoption, but never become roots implicitly.
func TestStatusCandidatesNotRoots(t *testing.T) {
	p := New(nil, newFakeCfg())
	_, resp := serve(t, p, http.MethodGet, "/providers/fab_vault/status", "")
	if resp["roots_source"] != "none" {
		t.Fatalf("roots_source = %v, want none", resp["roots_source"])
	}
	if roots := resp["roots"].([]any); len(roots) != 0 {
		t.Fatalf("unset must report zero roots, got %v", roots)
	}
	if _, ok := resp["candidate_roots"].([]any); !ok {
		t.Fatal("candidate_roots field missing")
	}
}

// POST roots makes the list authoritative; scan then walks only those roots.
func TestPostRootsThenConfigured(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "model.fbx"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := New(nil, newFakeCfg())
	body := `{"roots":[` + string('"') + filepath.ToSlash(root) + string('"') + `]}`
	// Escape for JSON via marshal instead of manual quoting.
	raw, _ := json.Marshal(map[string]any{"roots": []string{root}})
	body = string(raw)
	code, resp := serve(t, p, http.MethodPost, "/providers/fab_vault/roots", body)
	if code != 200 || resp["source"] != "configured" {
		t.Fatalf("post roots: code=%d resp=%v", code, resp)
	}

	_, resp = serve(t, p, http.MethodGet, "/providers/fab_vault/scan", "")
	if resp["roots_source"] != "configured" {
		t.Fatalf("roots_source = %v, want configured", resp["roots_source"])
	}
	sum := resp["scanned"].(map[string]any)
	if sum["files"].(float64) != 1 {
		t.Fatalf("configured root with one asset must scan 1 file, got %v", sum["files"])
	}
}

// An explicitly stored empty list is authoritative — no fallback to detected.
func TestExplicitEmptyRootsStaysConfigured(t *testing.T) {
	p := New(nil, newFakeCfg())
	serve(t, p, http.MethodPost, "/providers/fab_vault/roots", `{"roots":[]}`)
	_, resp := serve(t, p, http.MethodGet, "/providers/fab_vault/scan", "")
	if resp["roots_source"] != "configured" {
		t.Fatalf("roots_source = %v, want configured", resp["roots_source"])
	}
	sum := resp["scanned"].(map[string]any)
	if sum["files"].(float64) != 0 {
		t.Fatalf("explicit empty must scan nothing, got %v", sum["files"])
	}
}
