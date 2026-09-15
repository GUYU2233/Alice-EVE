package app

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"eve-assistant/desktop-app/internal/storage"
	_ "modernc.org/sqlite"
)

func TestPairingManagerRestoreAndClear(t *testing.T) {
	revoked := false
	revokeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/devices/revoke" || r.Method != http.MethodPost {
			t.Fatalf("unexpected revoke request: %s %s", r.Method, r.URL.Path)
		}
		revoked = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer revokeServer.Close()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := storage.NewSQLiteStore(db)
	ctx := context.Background()
	if err := storage.EnsureReliableSchema(ctx, store); err != nil {
		t.Fatal(err)
	}
	secrets := storage.NewMemorySecretStore()
	relay := NewRelayClientWithSecrets(secrets)
	if err := relay.SetURL(revokeServer.URL); err != nil {
		t.Fatal(err)
	}
	original := storage.PairingState{RelayURL: relay.URL(), DeviceID: "device-1", PairedAt: time.Now().UTC(), Version: 1}
	if err := storage.SavePairing(ctx, store, original); err != nil {
		t.Fatal(err)
	}
	if err := secrets.Save("relay-device-token", "token-1"); err != nil {
		t.Fatal(err)
	}
	manager := NewPairingManager(store, NewRelayClientWithSecrets(secrets))
	if err := manager.Restore(ctx); err != nil {
		t.Fatal(err)
	}
	state, err := manager.State()
	if err != nil || state.DeviceID != original.DeviceID || manager.relay.URL() != original.RelayURL || !manager.relay.HasToken() {
		t.Fatalf("restore failed: state=%+v err=%v url=%q token=%v", state, err, manager.relay.URL(), manager.relay.HasToken())
	}
	if err := manager.Clear(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.LoadPairing(ctx, store); err != storage.ErrPairingNotFound {
		t.Fatalf("pairing state remains: %v", err)
	}
	if _, err := secrets.Load("relay-device-token"); err != storage.ErrSecretNotFound {
		t.Fatalf("token remains: %v", err)
	}
	if !revoked {
		t.Fatal("remote device was not revoked before deletion")
	}
}

func TestRelayConfirmPairingAcceptsDeviceTokenAndPersistsSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/pair/confirm" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"deviceId":"device-2","deviceToken":"token-2"}`))
	}))
	defer server.Close()
	secrets := storage.NewMemorySecretStore()
	relay := NewRelayClientWithSecrets(secrets)
	if err := relay.SetURL(server.URL); err != nil {
		t.Fatal(err)
	}
	deviceID, err := relay.ConfirmPairing(context.Background(), "123456")
	if err != nil || deviceID != "device-2" {
		t.Fatalf("confirm failed: id=%q err=%v", deviceID, err)
	}
	if token, err := secrets.Load("relay-device-token"); err != nil || token != "token-2" {
		t.Fatalf("token not persisted: %q %v", token, err)
	}
}
