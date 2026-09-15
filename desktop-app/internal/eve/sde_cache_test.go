package eve

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSDECachePathMatchesPersistedArtifact(t *testing.T) {
	root := t.TempDir()
	cache := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "types.csv"), []byte("id,name\n34,Tritanium\n"), 0600); err != nil {
		t.Fatal(err)
	}
	idx := NewSDEIndex(cache)
	if err := idx.SetDirectory(root); err != nil {
		t.Fatal(err)
	}
	if _, err := idx.Reindex(context.Background()); err != nil {
		t.Fatal(err)
	}
	path := idx.CachePath()
	if path == "" {
		t.Fatal("cache path is empty")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("cache artifact missing at CachePath %q: %v", path, err)
	}
	loaded := NewSDEIndex(cache)
	if err := loaded.LoadCached(path); err != nil {
		t.Fatal(err)
	}
	if got := loaded.Query("Tritanium", 1); len(got) != 1 || got[0].ID != "34" {
		t.Fatalf("loaded entries=%v", got)
	}
}

func TestSDECacheCorruptionRollbackAndLimits(t *testing.T) {
	root, cache := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "types.csv"), []byte("id,name\n34,Tritanium\n"), 0600); err != nil {
		t.Fatal(err)
	}
	idx := NewSDEIndex(cache)
	idx.MaxFileBytes = 2
	_ = idx.SetDirectory(root)
	if _, err := idx.Reindex(context.Background()); err != ErrSDEResourceLimit {
		t.Fatalf("limit err=%v", err)
	}
	idx = NewSDEIndex(cache)
	_ = idx.SetDirectory(root)
	if _, err := idx.Reindex(context.Background()); err != nil {
		t.Fatal(err)
	}
	path := idx.CachePath()
	b, _ := os.ReadFile(path)
	b[len(b)-2] ^= 1
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	loaded := NewSDEIndex(cache)
	if err := loaded.LoadCached(path); err == nil {
		t.Fatal("expected checksum rejection")
	}
	if loaded.Status().Ready {
		t.Fatal("corrupt cache must not become ready")
	}
}
