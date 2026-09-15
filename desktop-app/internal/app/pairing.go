package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"eve-assistant/desktop-app/internal/storage"
)

// PairingManager owns the non-secret pairing identity and coordinates it with
// the server client. The device token remains in SecretStore and is never persisted
// in PairingState.
type PairingManager struct {
	mu    sync.RWMutex
	store storage.Store
	relay *RelayClient
	state storage.PairingState
}

func NewPairingManager(store storage.Store, relay *RelayClient) *PairingManager {
	return &PairingManager{store: store, relay: relay}
}

// Restore loads the token from the credential vault and, when available, the
// pairing identity from SQLite. Missing state is a normal first-run result.
func (m *PairingManager) Restore(ctx context.Context) error {
	if m.relay == nil {
		return errors.New("relay client is required")
	}
	if m.store == nil {
		return m.relay.RestoreToken()
	}
	p, err := storage.LoadPairing(ctx, m.store)
	if errors.Is(err, storage.ErrPairingNotFound) {
		m.mu.Lock()
		m.state = storage.PairingState{}
		m.mu.Unlock()
		return m.relay.RestoreToken()
	}
	if err != nil {
		return fmt.Errorf("load pairing state: %w", err)
	}
	if err := m.relay.SetURL(p.RelayURL); err != nil {
		return fmt.Errorf("restore server URL: %w", err)
	}
	if err := m.relay.RestoreToken(); err != nil {
		return fmt.Errorf("restore relay token: %w", err)
	}
	m.mu.Lock()
	m.state = p
	m.mu.Unlock()
	return nil
}

// Confirm exchanges a pairing code and persists the resulting non-secret
// identity. The server client persists the token in the platform credential vault.
func (m *PairingManager) Confirm(ctx context.Context, code string) (storage.PairingState, error) {
	if m.relay == nil {
		return storage.PairingState{}, errors.New("relay client is required")
	}
	if m.store == nil {
		return storage.PairingState{}, errors.New("pairing persistence is unavailable")
	}
	deviceID, err := m.relay.ConfirmPairing(ctx, code)
	if err != nil {
		return storage.PairingState{}, err
	}
	p := storage.PairingState{
		RelayURL: m.relay.URL(),
		DeviceID: deviceID,
		PairedAt: time.Now().UTC(),
		Version:  1,
	}
	if err := storage.SavePairing(ctx, m.store, p); err != nil {
		return storage.PairingState{}, fmt.Errorf("save pairing state: %w", err)
	}
	m.mu.Lock()
	m.state = p
	m.mu.Unlock()
	return p, nil
}

func (m *PairingManager) State() (storage.PairingState, error) {
	m.mu.RLock()
	p := m.state
	m.mu.RUnlock()
	if p.DeviceID == "" {
		return storage.PairingState{}, storage.ErrPairingNotFound
	}
	return p, nil
}

// Clear revokes the remote device before deleting local identity/credentials.
// If revocation cannot be confirmed, local state is retained for retry.
func (m *PairingManager) Clear(ctx context.Context) error {
	if m.relay == nil {
		return errors.New("relay client is required")
	}
	m.mu.RLock()
	deviceID := m.state.DeviceID
	m.mu.RUnlock()
	if deviceID != "" {
		if err := m.relay.RevokeAndClear(ctx, deviceID); err != nil {
			return fmt.Errorf("revoke remote device: %w", err)
		}
	} else if err := m.relay.ClearAllCredentials(); err != nil {
		return fmt.Errorf("clear relay credentials: %w", err)
	}
	if m.store != nil {
		if err := storage.ClearPairing(ctx, m.store); err != nil {
			return fmt.Errorf("clear pairing state: %w", err)
		}
	}
	m.mu.Lock()
	m.state = storage.PairingState{}
	m.mu.Unlock()
	return nil
}
