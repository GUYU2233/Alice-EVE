package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"sort"
	"sync"
	"time"
)

type PairCode struct {
	Code       string
	ExpiresAt  time.Time
	DeviceName string
	DeviceType string
}
type Device struct {
	ID        string
	AccountID string
	TokenHash string
	Name      string
	Type      string
	PublicKey string
	CreatedAt time.Time
	Revoked   bool
}

type MemoryStore struct {
	mu         sync.Mutex
	pairs      map[string]PairCode
	messages   map[string]Message
	devices    map[string]Device
	nextCursor int64
}

func NewMemory() *MemoryStore {
	return &MemoryStore{pairs: map[string]PairCode{}, messages: map[string]Message{}, devices: map[string]Device{}}
}
func messageKey(accountID, id string) string { return accountID + "\x00" + id }
func (s *MemoryStore) PutMessage(_ context.Context, m Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := messageKey(m.AccountID, m.ID)
	if _, exists := s.messages[key]; exists {
		return nil
	}
	if m.Cursor == 0 {
		s.nextCursor++
		m.Cursor = s.nextCursor
	} else if m.Cursor > s.nextCursor {
		s.nextCursor = m.Cursor
	}
	s.messages[key] = m
	return nil
}
func (s *MemoryStore) HasMessage(_ context.Context, accountID, id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.messages[messageKey(accountID, id)]
	return ok && m.AccountID == accountID, nil
}
func (s *MemoryStore) Ack(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, m := range s.messages {
		if m.ID == id {
			delete(s.messages, key)
		}
	}
	return nil
}
func (s *MemoryStore) AckMessage(_ context.Context, accountID, id, owner string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := messageKey(accountID, id)
	m, ok := s.messages[key]
	if !ok || m.AccountID != accountID || m.OwnerDeviceID == "" || m.OwnerDeviceID != owner {
		return false, nil
	}
	delete(s.messages, key)
	return true, nil
}
func (s *MemoryStore) ListMessages(_ context.Context, accountID, owner string, after int64, limit int) ([]Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	out := make([]Message, 0, limit)
	for _, m := range s.messages {
		if m.AccountID == accountID && m.OwnerDeviceID == owner && m.Cursor > after {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Cursor < out[j].Cursor })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
func NewDeviceID() string { b := make([]byte, 8); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func (s *MemoryStore) AddPair(_ context.Context, p PairCode) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pairs[p.Code] = p
	return nil
}
func (s *MemoryStore) ConsumePair(_ context.Context, code string) (PairCode, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.pairs[code]
	if !ok || time.Now().After(p.ExpiresAt) {
		delete(s.pairs, code)
		return PairCode{}, false
	}
	delete(s.pairs, code)
	return p, true
}
func tokenHash(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}
func (s *MemoryStore) DeviceIDForToken(token string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := tokenHash(token)
	for _, d := range s.devices {
		if !d.Revoked && subtle.ConstantTimeCompare([]byte(d.TokenHash), []byte(h)) == 1 {
			return d.ID, true
		}
	}
	return "", false
}
func (s *MemoryStore) DeviceType(id string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.devices[id]
	return d.Type, ok && !d.Revoked
}
func (s *MemoryStore) DeviceAccountID(id string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.devices[id]
	return d.AccountID, ok && !d.Revoked && d.AccountID != ""
}
func (s *MemoryStore) DeviceExists(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.devices[id]
	return ok && !d.Revoked
}
func (s *MemoryStore) VerifyToken(token string) bool { _, ok := s.DeviceIDForToken(token); return ok }
func (s *MemoryStore) VerifyTokenType(token, typ string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := tokenHash(token)
	for _, d := range s.devices {
		if !d.Revoked && d.Type == typ && subtle.ConstantTimeCompare([]byte(d.TokenHash), []byte(h)) == 1 {
			return true
		}
	}
	return false
}
func (s *MemoryStore) OwnsToken(token, id string) bool {
	got, ok := s.DeviceIDForToken(token)
	return ok && got == id
}
func (s *MemoryStore) RevokeDeviceForAccount(accountID, id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.devices[id]
	if !ok || d.AccountID != accountID || d.Revoked {
		return false
	}
	d.Revoked = true
	s.devices[id] = d
	return true
}
func (s *MemoryStore) RevokeDevice(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.devices[id]
	if !ok {
		return false
	}
	d.Revoked = true
	s.devices[id] = d
	return true
}
func (s *MemoryStore) GetDevice(id string) (Device, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.devices[id]
	return d, ok && !d.Revoked
}
func (s *MemoryStore) GetDeviceForAccount(accountID, id string) (Device, bool) {
	d, ok := s.GetDevice(id)
	return d, ok && d.AccountID == accountID
}

func (s *MemoryStore) ListDevices(accountID string) []Device {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Device, 0, len(s.devices))
	for _, d := range s.devices {
		if !d.Revoked && (accountID == "" || d.AccountID == accountID) {
			out = append(out, d)
		}
	}
	return out
}

func (s *MemoryStore) AddDevice(d Device) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// AccountID is optional for legacy pairing records. New account-bound
	// sessions populate it; keeping the old shape readable preserves the
	// compatibility /pair endpoint until all clients migrate.
	if d.ID == "" {
		return errors.New("device id required")
	}
	if d.AccountID == "" {
		d.AccountID = "legacy"
	}
	s.devices[d.ID] = d
	return nil
}
