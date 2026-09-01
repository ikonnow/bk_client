package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBundledToolsIncludeMtlxPackage guards the `all:tools/bk_mtlx` embed
// directive. The vendored MaterialX exporter is a multi-file Python package
// whose __init__.py files begin with `_`; without the `all:` prefix go:embed
// silently drops them, breaking `import bk_mtlx` at runtime. This test fails
// loudly if the package (or its __init__.py markers) ever stops being embedded.
func TestBundledToolsIncludeMtlxPackage(t *testing.T) {
	required := []string{
		"tools/bk_mtlx/__init__.py",
		"tools/bk_mtlx/blender_materialx_exporter.py",
		"tools/bk_mtlx/materialx_library_core.py",
		"tools/bk_mtlx/mtlxutils/__init__.py",
		"tools/bk_mtlx/mtlxutils/mxbase.py",
		"tools/bk_mtlx/mtlxutils/mxfile.py",
		"tools/bk_mtlx/mtlxutils/mxnodegraph.py",
		"tools/bk_mtlx/mtlxutils/mxtraversal.py",
	}
	for _, name := range required {
		b, err := bundledTools.ReadFile(name)
		if err != nil {
			t.Errorf("embedded file missing: %s: %v", name, err)
			continue
		}
		if len(b) == 0 && !strings.HasSuffix(name, "__init__.py") {
			t.Errorf("embedded file is empty: %s", name)
		}
	}
}

// TestExtractEmbeddedTreeMirrorsPackage verifies extractEmbeddedTree recreates
// the bk_mtlx package (with subpackage) under a destination tools dir so the
// recipe's `import bk_mtlx` resolves next to the extracted script.
func TestExtractEmbeddedTreeMirrorsPackage(t *testing.T) {
	dest := t.TempDir()
	if err := extractEmbeddedTree("tools/bk_mtlx", dest); err != nil {
		t.Fatalf("extractEmbeddedTree: %v", err)
	}
	want := []string{
		"bk_mtlx/__init__.py",
		"bk_mtlx/blender_materialx_exporter.py",
		"bk_mtlx/mtlxutils/__init__.py",
		"bk_mtlx/mtlxutils/mxtraversal.py",
	}
	for _, rel := range want {
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(rel))); err != nil {
			t.Errorf("expected mirrored file %s: %v", rel, err)
		}
	}
}
