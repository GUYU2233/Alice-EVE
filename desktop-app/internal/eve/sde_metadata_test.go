package eve

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSDEMetadataAndVersionedIndex(t *testing.T) {
	root, cache := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "types.csv"), []byte("id,name\n34,Tritanium\n"), 0600); err != nil {
		t.Fatal(err)
	}
	idx := NewSDEIndex(cache)
	if err := idx.SetDirectory(root); err != nil {
		t.Fatal(err)
	}
	idx.SetMetadata("2025.01", "https://developers.eveonline.com/static-data/", "vendor-sha", "sig")
	st, err := idx.Reindex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Version != "2025.01" || st.Source == "" || st.Checksum != "vendor-sha" || st.Signature != "sig" || st.IndexVersion != sdeIndexVersion {
		t.Fatalf("metadata=%+v", st)
	}
	loaded := NewSDEIndex(cache)
	if err := loaded.LoadCached(idx.CachePath()); err != nil {
		t.Fatal(err)
	}
	if loaded.Status().IndexVersion != sdeIndexVersion {
		t.Fatal(loaded.Status())
	}
}
