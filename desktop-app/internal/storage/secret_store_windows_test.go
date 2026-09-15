//go:build windows

package storage

import "testing"

func TestDPAPISecretStoreRoundTrip(t *testing.T) {
	store := NewDPAPISecretStore(t.TempDir())
	const key = "oauth-roundtrip"
	const value = `{"accessToken":"access","refreshToken":"refresh"}`
	if err := store.Save(key, value); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := store.Load(key)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got != value {
		t.Fatalf("got %q", got)
	}
}
