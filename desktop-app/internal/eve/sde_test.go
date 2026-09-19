package eve

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSDEIndexDirectoryBoundaryAndQuery(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "types.csv"), []byte("typeID,name\n34,Tritanium\n35,Pyerite\n"), 0600); err != nil {
		t.Fatal(err)
	}
	idx := NewSDEIndex(filepath.Join(root, "cache"))
	if err := idx.SetDirectory(root); err != nil {
		t.Fatal(err)
	}
	st, err := idx.Reindex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Entries != 2 || len(idx.Query("trit", 10)) != 1 {
		t.Fatalf("unexpected index: %+v", st)
	}
	if err := idx.SetDirectory(filepath.Join(root, "missing")); err == nil {
		t.Fatal("expected invalid path")
	}
}
func TestSDEJSONLocalized(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "x.json"), []byte(`{"34":{"name":{"en":"Tritanium"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	idx := NewSDEIndex("")
	_ = idx.SetDirectory(root)
	st, err := idx.Reindex(context.Background())
	if err != nil || st.Entries < 1 {
		t.Fatalf("reindex: %v %+v", err, st)
	}
}
