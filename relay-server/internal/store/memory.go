package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
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
	TokenHash string
	Name      string
	Type      string
	PublicKey string
	CreatedAt time.Time
	Revoked   bool
}

type MemoryStore struct {
	mu       sync.Mutex
	pairs    map[string]PairCode
	messages map[string]Message
	devices  map[string]Device
}

func NewMemory() *MemoryStore {
	return &MemoryStore{pairs: map[string]PairCode{}, messages: map[string]Message{}, devices: map[string]Device{}}
}
func (s *MemoryStore) PutMessage(_ context.Context, m Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages[m.ID] = m
	return nil
}
func (s *MemoryStore) HasMessage(_ context.Context, id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.messages[id]
	return ok, nil
}
func (s *MemoryStore) Ack(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.messages, id)
	return nil
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
func (s *MemoryStore) VerifyToken(token string) bool {
	digest := sha256.Sum256([]byte(token))
	h := hex.EncodeToString(digest[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.devices {
		if !d.Revoked && subtle.ConstantTimeCompare([]byte(d.TokenHash), []byte(h)) == 1 {
			return true
		}
	}
	return false
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
func (s *MemoryStore) AddDevice(d Device) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d.ID == "" {
		return errors.New("device id required")
	}
	s.devices[d.ID] = d
	return nil
}
