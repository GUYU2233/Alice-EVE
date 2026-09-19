package eve

import (
	"context"
	"eve-assistant/desktop-app/internal/evesde"
	"os"
	"path/filepath"
	"testing"
)

const normalizedFixture = `{"kind":"category","id":1,"name":"Material"}
{"kind":"group","id":10,"category_id":1,"name":"Mineral"}
{"kind":"type","id":34,"group_id":10,"name":"Tritanium"}
{"kind":"region","id":10000002,"name":"The Forge"}
{"kind":"constellation","id":20000020,"region_id":10000002,"name":"Kimotoro"}
{"kind":"system","id":30000142,"constellation_id":20000020,"name":"Jita","security":0.9}
`

func writeNormalizedFixture(t *testing.T, dir, contents string) string {
	t.Helper()
	path := filepath.Join(dir, "normalized.jsonl")
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSDEIndexSwitchesBetweenNormalizedAndLegacy(t *testing.T) {
	root := t.TempDir()
	normalized := writeNormalizedFixture(t, root, normalizedFixture)
	legacy := filepath.Join(root, "legacy")
	if err := os.Mkdir(legacy, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "types.csv"), []byte("id,name\n35,Pyerite\n"), 0600); err != nil {
		t.Fatal(err)
	}

	idx := NewSDEIndex(filepath.Join(root, "cache"))
	t.Cleanup(func() { _ = idx.Close() })
	idx.SetMetadata("2025.01", "fixture", "", "")
	st, err := idx.ImportNormalized(context.Background(), normalized)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Ready || st.Version != "2025.01" || st.IndexVersion != evesde.SchemaVersion || len(idx.Query("jita", 10)) != 1 {
		t.Fatalf("normalized status/query: %+v %#v", st, idx.Query("jita", 10))
	}
	if err = idx.SetDirectory(legacy); err != nil {
		t.Fatal(err)
	}
	if _, err = idx.Reindex(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := idx.Query("pyerite", 10); len(got) != 1 || got[0].ID != "35" {
		t.Fatalf("legacy query after switch: %#v", got)
	}
	if got := idx.Query("jita", 10); len(got) != 0 {
		t.Fatalf("normalized repository remained selected: %#v", got)
	}
}

func TestSDEIndexFailedNormalizedImportKeepsCurrentBackend(t *testing.T) {
	root := t.TempDir()
	good := writeNormalizedFixture(t, root, normalizedFixture)
	idx := NewSDEIndex(filepath.Join(root, "cache"))
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.ImportNormalized(context.Background(), good); err != nil {
		t.Fatal(err)
	}
	before := idx.Status()
	bad := filepath.Join(root, "bad.jsonl")
	if err := os.WriteFile(bad, []byte(`{"kind":"type","id":99,"group_id":999,"name":"Broken"}`+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := idx.ImportNormalized(context.Background(), bad); err == nil {
		t.Fatal("expected import failure")
	}
	if got := idx.Query("tritanium", 10); len(got) != 1 || got[0].Kind != "type" {
		t.Fatalf("old repository unavailable after failed import: %#v", got)
	}
	if after := idx.Status(); after.Directory != before.Directory || after.Checksum != before.Checksum {
		t.Fatalf("status changed on failed import: before=%+v after=%+v", before, after)
	}
}

func TestNormalizedQueryCombinesKindsAndHonorsLimit(t *testing.T) {
	root := t.TempDir()
	path := writeNormalizedFixture(t, root, normalizedFixture)
	idx := NewSDEIndex(filepath.Join(root, "cache"))
	t.Cleanup(func() { _ = idx.Close() })
	if _, err := idx.ImportNormalized(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	got := idx.Query("i", 2)
	if len(got) != 2 {
		t.Fatalf("query limit ignored: %#v", got)
	}
	byID := idx.Query("30000142", 10)
	if len(byID) != 1 || byID[0].Name != "Jita" || byID[0].Kind != "system" {
		t.Fatalf("ID query: %#v", byID)
	}
	if got := idx.Query("%", 10); len(got) != 0 {
		t.Fatalf("LIKE wildcard was not escaped: %#v", got)
	}
}
