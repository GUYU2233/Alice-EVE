package updates

import (
	"context"
	"errors"
	"testing"
	"time"
)

func manifest(id, version string, mandatory bool) Manifest {
	return Manifest{ReleaseID: id, App: "alice-eve", Version: version, Platform: "windows", Arch: "amd64", Channel: "stable", Mandatory: mandatory, ArtifactURL: "https://cdn.example.invalid/" + id, SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", Signature: SignatureMetadata{Algorithm: "ed25519", KeyID: "release-1", Value: "signature"}, PublishedAt: time.Now()}
}
func TestManifestSelectionAndRollback(t *testing.T) {
	r := NewMemoryRepository()
	_ = r.Add(manifest("r1", "1.0.0", false))
	_ = r.Add(manifest("r2", "1.2.0", false))
	_ = r.Add(manifest("r3", "2.0.0", true))
	s := NewService(r)
	got, err := s.Manifest(context.Background(), Query{App: "alice-eve", Platform: "windows", Arch: "amd64", Channel: "stable", CurrentVersion: "1.0.0"})
	if err != nil || got.Manifest.Version != "2.0.0" || got.Rollback {
		t.Fatalf("upgrade selection %#v %v", got, err)
	}
	got, err = s.Manifest(context.Background(), Query{App: "alice-eve", Platform: "windows", Arch: "amd64", Channel: "stable", CurrentVersion: "2.0.0", AllowRollback: true})
	if err != nil || got.Manifest.Version != "1.2.0" || !got.Rollback {
		t.Fatalf("rollback selection %#v %v", got, err)
	}
	_, err = s.Manifest(context.Background(), Query{App: "alice-eve", Platform: "windows", Arch: "amd64", Channel: "stable", CurrentVersion: "2.0.0"})
	if !errors.Is(err, ErrNoUpdate) {
		t.Fatalf("expected no rollback, got %v", err)
	}
}
func TestManifestValidationAndAck(t *testing.T) {
	r := NewMemoryRepository()
	s := NewService(r)
	m := manifest("r", "1.0.0", false)
	m.ArtifactURL = "http://bad"
	if err := r.Add(m); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("expected URL validation, got %v", err)
	}
	if err := s.Ack(context.Background(), Ack{ReleaseID: "r", AccountID: "a", DeviceID: "d", Status: "installed"}); err != nil {
		t.Fatal(err)
	}
	if len(r.Acks()) != 1 {
		t.Fatal("ack not recorded")
	}
}
func TestVersionComparison(t *testing.T) {
	a, _ := ParseVersion("v1.2.3")
	b, _ := ParseVersion("1.2.4")
	if !a.Less(b) {
		t.Fatal("version ordering")
	}
	if a.Compare(a) != 0 {
		t.Fatal("version equality")
	}
}
