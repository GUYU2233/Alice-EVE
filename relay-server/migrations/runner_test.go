package migrations

import (
	"testing"
	"testing/fstest"
)

func TestNewFromFSOrdersAndChecksumsMigrations(t *testing.T) {
	fsys := fstest.MapFS{
		"003_last.sql":   {Data: []byte("SELECT 3;")},
		"001_first.sql":  {Data: []byte("SELECT 1;")},
		"002_middle.sql": {Data: []byte("SELECT 2;")},
		"README.md":      {Data: []byte("ignored")},
		"nested/004.sql": {Data: []byte("ignored")},
	}
	runner, err := NewFromFS(fsys)
	if err != nil {
		t.Fatal(err)
	}
	got := runner.Migrations()
	if len(got) != 3 {
		t.Fatalf("got %d migrations, want 3", len(got))
	}
	for i, wantVersion := range []int64{1, 2, 3} {
		if got[i].Version != wantVersion {
			t.Fatalf("migration %d has version %d, want %d", i, got[i].Version, wantVersion)
		}
		if got[i].Checksum == "" || len(got[i].Checksum) != 64 {
			t.Fatalf("migration %d has invalid checksum %q", i, got[i].Checksum)
		}
	}
}

func TestNewFromFSRejectsDuplicateVersions(t *testing.T) {
	_, err := NewFromFS(fstest.MapFS{
		"001_first.sql": {Data: []byte("SELECT 1;")},
		"001_other.sql": {Data: []byte("SELECT 1;")},
	})
	if err == nil {
		t.Fatal("expected duplicate version error")
	}
}

func TestNewFromFSRejectsEmptyMigration(t *testing.T) {
	_, err := NewFromFS(fstest.MapFS{"001_empty.sql": {Data: []byte("\n")}})
	if err == nil {
		t.Fatal("expected empty migration error")
	}
}
