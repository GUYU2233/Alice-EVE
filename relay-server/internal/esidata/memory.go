package esidata

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
)

type characterKey struct {
	accountID   string
	characterID int64
	domain      string
}

type publicKey struct {
	kind     string
	cacheKey string
}

// MemoryRepository is a concurrency-safe in-memory Repository.
type MemoryRepository struct {
	mu         sync.RWMutex
	characters map[characterKey]CharacterSnapshot
	public     map[publicKey]PublicData
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		characters: make(map[characterKey]CharacterSnapshot),
		public:     make(map[publicKey]PublicData),
	}
}

func NewMemory() *MemoryRepository { return NewMemoryRepository() }

func (r *MemoryRepository) UpsertCharacterSnapshot(ctx context.Context, snapshot CharacterSnapshot) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validCharacterSnapshot(snapshot) {
		return ErrInvalidInput
	}
	snapshot.Payload = clonePayload(snapshot.Payload)
	key := characterKey{snapshot.AccountID, snapshot.CharacterID, snapshot.Domain}
	r.mu.Lock()
	r.characters[key] = snapshot
	r.mu.Unlock()
	return nil
}

func (r *MemoryRepository) GetCharacterSnapshot(ctx context.Context, accountID string, characterID int64, domain string) (CharacterSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return CharacterSnapshot{}, err
	}
	if strings.TrimSpace(accountID) == "" || characterID <= 0 || strings.TrimSpace(domain) == "" {
		return CharacterSnapshot{}, ErrInvalidInput
	}
	r.mu.RLock()
	snapshot, ok := r.characters[characterKey{accountID, characterID, domain}]
	r.mu.RUnlock()
	if !ok {
		return CharacterSnapshot{}, ErrNotFound
	}
	snapshot.Payload = clonePayload(snapshot.Payload)
	return snapshot, nil
}

func (r *MemoryRepository) ListAccountCharacters(ctx context.Context, accountID string) ([]CharacterSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(accountID) == "" {
		return nil, ErrInvalidInput
	}
	r.mu.RLock()
	out := make([]CharacterSnapshot, 0)
	for key, snapshot := range r.characters {
		if key.accountID == accountID {
			snapshot.Payload = clonePayload(snapshot.Payload)
			out = append(out, snapshot)
		}
	}
	r.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].CharacterID == out[j].CharacterID {
			return out[i].Domain < out[j].Domain
		}
		return out[i].CharacterID < out[j].CharacterID
	})
	return out, nil
}

func (r *MemoryRepository) ListCharacterDomains(ctx context.Context, accountID string, characterID int64) ([]CharacterSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(accountID) == "" || characterID <= 0 {
		return nil, ErrInvalidInput
	}
	r.mu.RLock()
	out := make([]CharacterSnapshot, 0)
	for key, snapshot := range r.characters {
		if key.accountID == accountID && key.characterID == characterID {
			snapshot.Payload = clonePayload(snapshot.Payload)
			out = append(out, snapshot)
		}
	}
	r.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Domain < out[j].Domain })
	return out, nil
}

func (r *MemoryRepository) UpsertPublicData(ctx context.Context, entry PublicData) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validPublicData(entry) {
		return ErrInvalidInput
	}
	entry.Payload = clonePayload(entry.Payload)
	r.mu.Lock()
	r.public[publicKey{entry.Kind, entry.CacheKey}] = entry
	r.mu.Unlock()
	return nil
}

func (r *MemoryRepository) GetPublicData(ctx context.Context, kind, cacheKey string) (PublicData, error) {
	if err := ctx.Err(); err != nil {
		return PublicData{}, err
	}
	if strings.TrimSpace(kind) == "" || strings.TrimSpace(cacheKey) == "" {
		return PublicData{}, ErrInvalidInput
	}
	r.mu.RLock()
	entry, ok := r.public[publicKey{kind, cacheKey}]
	r.mu.RUnlock()
	if !ok {
		return PublicData{}, ErrNotFound
	}
	entry.Payload = clonePayload(entry.Payload)
	return entry, nil
}

func validCharacterSnapshot(snapshot CharacterSnapshot) bool {
	return strings.TrimSpace(snapshot.AccountID) != "" && snapshot.CharacterID > 0 &&
		strings.TrimSpace(snapshot.Domain) != "" && strings.TrimSpace(snapshot.Source) != "" &&
		validPayload(snapshot.Payload) && !snapshot.FetchedAt.IsZero() &&
		!snapshot.ExpiresAt.Before(snapshot.FetchedAt)
}

func validPublicData(entry PublicData) bool {
	return strings.TrimSpace(entry.Kind) != "" && strings.TrimSpace(entry.CacheKey) != "" &&
		validPayload(entry.Payload) && !entry.FetchedAt.IsZero() &&
		!entry.ExpiresAt.Before(entry.FetchedAt)
}

func validPayload(payload json.RawMessage) bool {
	if !json.Valid(payload) {
		return false
	}
	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		return false
	}
	switch value.(type) {
	case map[string]any, []any:
		return true
	default:
		return false
	}
}

func clonePayload(payload json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), payload...)
}

var _ Repository = (*MemoryRepository)(nil)
