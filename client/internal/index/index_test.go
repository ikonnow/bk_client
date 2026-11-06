package index

import (
	"context"
	"path/filepath"
	"testing"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestUpsertAndSearch(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	a := Asset{Provider: "local_dir", Path: "/x/chair.fbx", Name: "chair", Kind: "model", Size: 10, MTime: 2}
	if err := s.UpsertAsset(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertAsset(ctx, Asset{Provider: "fab_vault", Path: "/y/chair2.fbx", Name: "chair2", Kind: "model", MTime: 3}); err != nil {
		t.Fatal(err)
	}

	hits, err := s.Search(ctx, "", "chair", 0)
	if err != nil || len(hits) != 2 {
		t.Fatalf("cross-provider search: hits=%v err=%v", hits, err)
	}
	if hits[0].Name != "chair2" {
		t.Fatalf("results must order newest first, got %v", hits[0].Name)
	}

	hits, err = s.Search(ctx, "local_dir", "chair", 0)
	if err != nil || len(hits) != 1 || hits[0].Provider != "local_dir" {
		t.Fatalf("provider filter: hits=%v err=%v", hits, err)
	}

	hits, err = s.Search(ctx, "", "chair", 1)
	if err != nil || len(hits) != 1 {
		t.Fatalf("limit: hits=%v err=%v", hits, err)
	}
}

func TestUpsertReplaces(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	p := "/x/a.fbx"
	s.UpsertAsset(ctx, Asset{Provider: "local_dir", Path: p, Name: "old", Kind: "model"})
	s.UpsertAsset(ctx, Asset{Provider: "local_dir", Path: p, Name: "new", Kind: "model"})
	hits, _ := s.Search(ctx, "", "old", 0)
	if len(hits) != 0 {
		t.Fatal("upsert must replace the old row")
	}
	hits, _ = s.Search(ctx, "", "new", 0)
	if len(hits) != 1 {
		t.Fatal("upserted row not found")
	}
}

func TestDeleteProvider(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	s.UpsertAsset(ctx, Asset{Provider: "a", Path: "/p/1.fbx", Name: "one", Kind: "model"})
	s.UpsertAsset(ctx, Asset{Provider: "b", Path: "/p/2.fbx", Name: "two", Kind: "model"})
	if err := s.DeleteProvider(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	hits, _ := s.Search(ctx, "", "", 0)
	if len(hits) != 1 || hits[0].Provider != "b" {
		t.Fatalf("only provider b must remain, got %v", hits)
	}
	if err := s.DeleteProvider(ctx, ""); err == nil {
		t.Fatal("empty provider must error")
	}
}
