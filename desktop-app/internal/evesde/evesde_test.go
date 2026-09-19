package evesde

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func importFixture(t *testing.T) (*Repository, string) {
	t.Helper()
	b, err := os.ReadFile("testdata/tiny.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "sde.sqlite")
	if err = Import(context.Background(), path, strings.NewReader(string(b)), Manifest{Version: "2025.01", Source: "test-fixture"}, ImportOptions{}); err != nil {
		t.Fatal(err)
	}
	r, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	return r, path
}

func TestImportManifestResolversSearchAndRelations(t *testing.T) {
	r, _ := importFixture(t)
	ctx := context.Background()
	m, err := r.Manifest(ctx)
	if err != nil || m.SchemaVersion != SchemaVersion || m.Version != "2025.01" || len(m.Checksum) != 64 {
		t.Fatalf("manifest=%+v err=%v", m, err)
	}
	typeByID, err := r.TypeByID(ctx, 34)
	if err != nil || typeByID.Name != "Tritanium" {
		t.Fatalf("type=%+v err=%v", typeByID, err)
	}
	typeByName, err := r.TypeByName(ctx, "tritanium")
	if err != nil || typeByName.ID != 34 {
		t.Fatalf("type=%+v err=%v", typeByName, err)
	}
	types, err := r.SearchTypes(ctx, "trit", 10)
	if err != nil || len(types) != 2 {
		t.Fatalf("types=%+v err=%v", types, err)
	}
	sys, err := r.SystemByName(ctx, "JITA")
	if err != nil || sys.ID != 30000142 {
		t.Fatalf("system=%+v err=%v", sys, err)
	}
	systems, err := r.SearchSystems(ctx, "peri", 10)
	if err != nil || len(systems) != 1 {
		t.Fatalf("systems=%+v err=%v", systems, err)
	}
	gates, err := r.StargatesFromSystem(ctx, sys.ID)
	if err != nil || len(gates) != 1 || gates[0].DestinationSystemID != 30000144 {
		t.Fatalf("gates=%+v err=%v", gates, err)
	}
	bp, err := r.BlueprintByTypeID(ctx, 1000)
	if err != nil || len(bp.Materials) != 1 || bp.Materials[0].Quantity != 2 {
		t.Fatalf("bp=%+v err=%v", bp, err)
	}
	if _, err = r.CategoryByName(ctx, "material"); err != nil {
		t.Fatal(err)
	}
	if _, err = r.RegionByID(ctx, 10000002); err != nil {
		t.Fatal(err)
	}
	if _, err = r.TypeByID(ctx, -1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestImportChecksumAndResourceLimits(t *testing.T) {
	b, err := os.ReadFile("testdata/tiny.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	path := filepath.Join(t.TempDir(), "db.sqlite")
	if err = Import(context.Background(), path, strings.NewReader(string(b)), Manifest{Version: "v", Source: "fixture", Checksum: hex.EncodeToString(sum[:])}, ImportOptions{}); err != nil {
		t.Fatal(err)
	}
	if err = Import(context.Background(), path, strings.NewReader(string(b)), Manifest{Version: "v", Source: "fixture", Checksum: "bad"}, ImportOptions{}); err == nil {
		t.Fatal("expected checksum error")
	}
	if err = Import(context.Background(), filepath.Join(t.TempDir(), "small.sqlite"), strings.NewReader(string(b)), Manifest{Version: "v", Source: "fixture"}, ImportOptions{MaxBytes: 10}); !errors.Is(err, ErrResourceLimit) {
		t.Fatalf("got %v", err)
	}
	if err = Import(context.Background(), filepath.Join(t.TempDir(), "count.sqlite"), strings.NewReader(string(b)), Manifest{Version: "v", Source: "fixture"}, ImportOptions{MaxRecords: 1}); !errors.Is(err, ErrResourceLimit) {
		t.Fatalf("got %v", err)
	}
}

func TestInvalidImportDoesNotReplaceExistingDatabase(t *testing.T) {
	r, path := importFixture(t)
	r.Close()
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	bad := `{"kind":"type","id":99,"group_id":999,"name":"Broken"}` + "\n"
	err = Import(context.Background(), path, strings.NewReader(bad), Manifest{Version: "bad", Source: "fixture"}, ImportOptions{})
	if err == nil {
		t.Fatal("expected foreign key failure")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("existing database changed after failed import")
	}
}

func TestValidationAndCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db.sqlite")
	if err := Import(context.Background(), path, strings.NewReader(`{"kind":"wat","id":1}`+"\n"), Manifest{Version: "v", Source: "s"}, ImportOptions{}); !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("got %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Import(ctx, path, strings.NewReader(`{"kind":"category","id":1,"name":"x"}`+"\n"), Manifest{Version: "v", Source: "s"}, ImportOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}
