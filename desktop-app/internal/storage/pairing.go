package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrPairingNotFound = errors.New("pairing state not found")

// PairingState contains only the non-secret identity needed to restore a pairing.
// Device credentials are deliberately not serialized here; callers should keep them
// in the platform credential vault and clear them when pairing is removed.
type PairingState struct {
	RelayURL string    `json:"relay_url"`
	DeviceID string    `json:"device_id"`
	PairedAt time.Time `json:"paired_at"`
	Version  int       `json:"version"`
}

func (p PairingState) validate() error {
	if strings.TrimSpace(p.RelayURL) == "" || strings.TrimSpace(p.DeviceID) == "" {
		return errors.New("pairing state requires relay URL and device ID")
	}
	if p.Version <= 0 {
		return errors.New("pairing state version must be positive")
	}
	if p.PairedAt.IsZero() {
		return errors.New("pairing state requires paired-at timestamp")
	}
	return nil
}

func SavePairing(ctx context.Context, s Store, p PairingState) error {
	if err := p.validate(); err != nil {
		return err
	}
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = s.Exec(ctx, `INSERT INTO pairing_state(id,payload,updated_ms) VALUES(1,?,?) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload,updated_ms=excluded.updated_ms`, b, time.Now().UnixMilli())
	return err
}

func LoadPairing(ctx context.Context, s Store) (PairingState, error) {
	rows, err := s.Query(ctx, "SELECT payload FROM pairing_state WHERE id=1")
	if err != nil {
		return PairingState{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return PairingState{}, err
		}
		return PairingState{}, ErrPairingNotFound
	}
	var b []byte
	if err := rows.Scan(&b); err != nil {
		return PairingState{}, err
	}
	var p PairingState
	if err := json.Unmarshal(b, &p); err != nil {
		return PairingState{}, fmt.Errorf("decode pairing state: %w", err)
	}
	if err := p.validate(); err != nil {
		return PairingState{}, fmt.Errorf("invalid pairing state: %w", err)
	}
	return p, rows.Err()
}

func ClearPairing(ctx context.Context, s Store) error {
	_, err := s.Exec(ctx, "DELETE FROM pairing_state WHERE id=1")
	return err
}
