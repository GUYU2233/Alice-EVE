// Package evegrant encrypts and persists EVE OAuth refresh grants.
package evegrant

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

var ErrNotFound = errors.New("eve refresh grant not found")

type Grant struct {
	AccountID       string    `json:"-"`
	ProviderSubject string    `json:"-"`
	RefreshToken    string    `json:"-"`
	ExpiresAt       time.Time `json:"-"`
	Scope           string    `json:"-"`
}

type EncryptedGrant struct {
	AccountID       string
	ProviderSubject string
	KeyID           string
	Nonce           []byte
	Ciphertext      []byte
	ExpiresAt       time.Time
	Scope           string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type Repository interface {
	Upsert(context.Context, EncryptedGrant) error
	FindByAccount(context.Context, string) (EncryptedGrant, error)
	Revoke(context.Context, string) error
}

type Keyring struct {
	active string
	keys   map[string][]byte
}

// ParseKeyring parses comma-separated keyID=base64(raw 32-byte key) entries.
// The first entry is used for new ciphertext; remaining entries decrypt old data.
func ParseKeyring(value string) (*Keyring, error) {
	keys := make(map[string][]byte)
	active := ""
	for _, entry := range strings.Split(value, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			return nil, errors.New("invalid EVE grant keyring entry")
		}
		id := strings.TrimSpace(parts[0])
		if _, exists := keys[id]; exists {
			return nil, errors.New("duplicate EVE grant key id")
		}
		key, err := base64.RawStdEncoding.DecodeString(strings.TrimSpace(parts[1]))
		if err != nil {
			key, err = base64.StdEncoding.DecodeString(strings.TrimSpace(parts[1]))
		}
		if err != nil || len(key) != 32 {
			return nil, errors.New("EVE grant keys must be base64-encoded 32-byte values")
		}
		keys[id] = key
		if active == "" {
			active = id
		}
	}
	if active == "" {
		return nil, errors.New("EVE grant keyring is empty")
	}
	return &Keyring{active: active, keys: keys}, nil
}

func (k *Keyring) String() string { return fmt.Sprintf("EVE grant keyring (%d keys)", len(k.keys)) }

type Service struct {
	repo Repository
	keys *Keyring
	now  func() time.Time
}

func NewService(repo Repository, keys *Keyring) (*Service, error) {
	if repo == nil || keys == nil || len(keys.keys) == 0 {
		return nil, errors.New("EVE grant repository and keyring are required")
	}
	return &Service{repo: repo, keys: keys, now: func() time.Time { return time.Now().UTC() }}, nil
}

func aad(accountID, subject string) []byte { return []byte(accountID + "\x00" + subject) }

func (s *Service) Save(ctx context.Context, grant Grant) error {
	if strings.TrimSpace(grant.AccountID) == "" || strings.TrimSpace(grant.ProviderSubject) == "" || grant.RefreshToken == "" {
		return errors.New("invalid EVE refresh grant")
	}
	block, err := aes.NewCipher(s.keys.keys[s.keys.active])
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(grant.RefreshToken), aad(grant.AccountID, grant.ProviderSubject))
	now := s.now()
	return s.repo.Upsert(ctx, EncryptedGrant{AccountID: grant.AccountID, ProviderSubject: grant.ProviderSubject, KeyID: s.keys.active, Nonce: nonce, Ciphertext: ciphertext, ExpiresAt: grant.ExpiresAt, Scope: grant.Scope, CreatedAt: now, UpdatedAt: now})
}

func (s *Service) Revoke(ctx context.Context, accountID string) error {
	if strings.TrimSpace(accountID) == "" {
		return errors.New("account ID is required")
	}
	return s.repo.Revoke(ctx, accountID)
}

func (s *Service) Load(ctx context.Context, accountID string) (Grant, error) {
	record, err := s.repo.FindByAccount(ctx, accountID)
	if err != nil {
		return Grant{}, err
	}
	key, ok := s.keys.keys[record.KeyID]
	if !ok {
		return Grant{}, errors.New("EVE grant encryption key is unavailable")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return Grant{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return Grant{}, err
	}
	plaintext, err := gcm.Open(nil, record.Nonce, record.Ciphertext, aad(record.AccountID, record.ProviderSubject))
	if err != nil {
		return Grant{}, errors.New("EVE refresh grant decryption failed")
	}
	return Grant{AccountID: record.AccountID, ProviderSubject: record.ProviderSubject, RefreshToken: string(plaintext), ExpiresAt: record.ExpiresAt, Scope: record.Scope}, nil
}

type MemoryRepository struct {
	mu        sync.RWMutex
	byAccount map[string]EncryptedGrant
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{byAccount: make(map[string]EncryptedGrant)}
}
func clone(g EncryptedGrant) EncryptedGrant {
	g.Nonce = append([]byte(nil), g.Nonce...)
	g.Ciphertext = append([]byte(nil), g.Ciphertext...)
	return g
}
func (r *MemoryRepository) Upsert(_ context.Context, g EncryptedGrant) error {
	if g.AccountID == "" || g.KeyID == "" || len(g.Nonce) == 0 || len(g.Ciphertext) == 0 {
		return errors.New("invalid encrypted EVE grant")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if old, ok := r.byAccount[g.AccountID]; ok {
		g.CreatedAt = old.CreatedAt
	}
	r.byAccount[g.AccountID] = clone(g)
	return nil
}
func (r *MemoryRepository) Revoke(_ context.Context, accountID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byAccount, accountID)
	return nil
}
func (r *MemoryRepository) FindByAccount(_ context.Context, accountID string) (EncryptedGrant, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	g, ok := r.byAccount[accountID]
	if !ok {
		return EncryptedGrant{}, ErrNotFound
	}
	return clone(g), nil
}
