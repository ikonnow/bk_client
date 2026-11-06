package providers

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/blenderkit/blenderkit/client/internal/index"
)

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Classification, hidden-dir skip, missing root skip, real index writes.
func TestScanRootsIndexesOnlyAssets(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "chair.fbx"))
	writeFile(t, filepath.Join(root, "wood.png"))
	writeFile(t, filepath.Join(root, "readme.txt"))           // unclassified
	writeFile(t, filepath.Join(root, ".hidden", "s.fbx"))     // hidden dir skipped
	writeFile(t, filepath.Join(root, "sub", "deep", "m.glb")) // nested model

	idx, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()

	missing := filepath.Join(root, "disconnected-drive")
	sum := ScanRoots(context.Background(), idx, "local_dir", []string{root, missing})

	if len(sum.Roots) != 2 {
		t.Fatalf("expected 2 root results, got %v", sum.Roots)
	}
	if sum.Roots[0].Indexed != 3 || sum.Roots[0].Skipped {
		t.Fatalf("root must index 3 classified files, got %+v", sum.Roots[0])
	}
	if !sum.Roots[1].Skipped {
		t.Fatal("missing root must be skipped, not fatal")
	}
	if sum.Files != 3 {
		t.Fatalf("files = %d, want 3", sum.Files)
	}

	hits, err := idx.Search(context.Background(), "local_dir", "chair", 10)
	if err != nil || len(hits) != 1 || hits[0].Kind != "model" {
		t.Fatalf("search after scan: hits=%v err=%v", hits, err)
	}
}

// The depth cap keeps the walker out of arbitrarily deep trees.
func TestScanRootsDepthCap(t *testing.T) {
	root := t.TempDir()
	// depth(root, dir) counts separators: d1..d4 descend, d5 (depth 5) is skipped.
	shallow := filepath.Join(root, "d1", "d2", "d3", "d4", "in.fbx")
	deep := filepath.Join(root, "d1", "d2", "d3", "d4", "d5", "out.fbx")
	writeFile(t, shallow)
	writeFile(t, deep)

	sum := ScanRoots(context.Background(), nil, "local_dir", []string{root})
	if sum.Files != 1 {
		t.Fatalf("depth cap must stop at d5, files = %d want 1", sum.Files)
	}
}

// A cancelled context aborts the walk promptly.
func TestScanRootsCancelled(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.fbx"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sum := ScanRoots(ctx, nil, "local_dir", []string{root})
	if sum.Files != 0 {
		t.Fatalf("cancelled scan must index nothing, got %d", sum.Files)
	}
}

// Symlinked directories are never followed (loops / escaping the root).
func TestScanRootsSkipsSymlinkDirs(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "outside.fbx"))
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink privilege unavailable: %v", err)
		}
		t.Fatal(err)
	}
	sum := ScanRoots(context.Background(), nil, "local_dir", []string{root})
	if sum.Files != 0 {
		t.Fatalf("symlinked dir must not be followed, files = %d", sum.Files)
	}
}
