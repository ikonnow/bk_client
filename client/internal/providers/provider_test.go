package providers

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/blenderkit/blenderkit/client/internal/settings"
)

// fakeCfg is an in-memory Config for tests.
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

func TestRootsFromVarUnsetVsEmpty(t *testing.T) {
	cfg := newFakeCfg()

	if _, ok := RootsFromVar(cfg, "roots"); ok {
		t.Fatal("unset variable must report ok=false (never configured)")
	}

	cfg.vars["roots"] = "[]"
	roots, ok := RootsFromVar(cfg, "roots")
	if !ok || len(roots) != 0 {
		t.Fatalf("explicit empty list must report ok=true with zero roots, got %v ok=%v", roots, ok)
	}
}

func TestRootsFromVarDecodesList(t *testing.T) {
	cfg := newFakeCfg()
	cfg.vars["roots"] = `["/a","/b"]`
	roots, ok := RootsFromVar(cfg, "roots")
	if !ok || len(roots) != 2 || roots[0] != "/a" {
		t.Fatalf("got %v ok=%v", roots, ok)
	}
}

func TestRootsFromVarBadJSONIsEmpty(t *testing.T) {
	cfg := newFakeCfg()
	cfg.vars["roots"] = "{not json"
	roots, ok := RootsFromVar(cfg, "roots")
	if !ok || len(roots) != 0 {
		t.Fatalf("corrupt value must decode to empty configured list, got %v", roots)
	}
}

func TestRootsFromVarNilCfg(t *testing.T) {
	if _, ok := RootsFromVar(nil, "roots"); ok {
		t.Fatal("nil config must report ok=false")
	}
}

func TestSaveRootsNormalizesAndPersists(t *testing.T) {
	cfg := newFakeCfg()
	tmp := t.TempDir()
	stored, err := SaveRoots(cfg, "roots", []string{tmp, tmp + string(filepath.Separator), "", tmp})
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 {
		t.Fatalf("duplicate/empty roots must collapse to one, got %v", stored)
	}
	var persisted []string
	if err := json.Unmarshal([]byte(cfg.vars["roots"]), &persisted); err != nil {
		t.Fatalf("stored value must be a JSON list: %v", err)
	}
	if len(persisted) != 1 || persisted[0] != stored[0] {
		t.Fatalf("persisted %v != stored %v", persisted, stored)
	}
}

func TestNormalizeRootsSortsAndCleans(t *testing.T) {
	tmp := t.TempDir()
	b := filepath.Join(tmp, "b")
	a := filepath.Join(tmp, "a")
	out := NormalizeRoots([]string{b, a, b})
	if len(out) != 2 || out[0] != a || out[1] != b {
		t.Fatalf("expected sorted deduped [a b], got %v", out)
	}
}

func TestClassifyExt(t *testing.T) {
	cases := map[string]string{
		"chair.FBX":     "model",
		"rock.glb":      "model",
		"wood.png":      "texture",
		"sky.EXR":       "hdr",
		"prop.uasset":   "ue_asset",
		"meta.json":     "metadata",
		"readme.txt":    "",
		"noext":         "",
		"archive.blend": "model",
	}
	for name, want := range cases {
		if got := ClassifyExt(name); got != want {
			t.Errorf("ClassifyExt(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestProbeRoots(t *testing.T) {
	tmp := t.TempDir()
	missing := filepath.Join(tmp, "nope")
	res := ProbeRoots([]string{tmp, missing})
	if len(res) != 2 || !res[0].Exists || res[1].Exists {
		t.Fatalf("got %+v", res)
	}
}

func TestDepth(t *testing.T) {
	root := filepath.Join("a", "b")
	if d := depth(root, filepath.Join(root, "c", "d")); d != 2 {
		t.Fatalf("depth = %d, want 2", d)
	}
	if d := depth(root, root); d != 0 {
		t.Fatalf("depth of root = %d, want 0", d)
	}
}
