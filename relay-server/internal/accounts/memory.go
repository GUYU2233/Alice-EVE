package accounts

import (
	"context"
	"strings"
	"sync"
	"time"
)

// MemoryRepository is a concurrency-safe repository used by tests and the
// development server. It deliberately stores only token hashes.
type MemoryRepository struct {
	mu         sync.RWMutex
	accounts   map[string]Account
	identities map[string]Identity
	sessions   map[string]Session
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		accounts:   make(map[string]Account),
		identities: make(map[string]Identity),
		sessions:   make(map[string]Session),
	}
}

// NewMemory is a short compatibility constructor for tests and local callers.
func NewMemory() *MemoryRepository { return NewMemoryRepository() }

func (r *MemoryRepository) CreateAccount(_ context.Context, account Account) error {
	if strings.TrimSpace(account.ID) == "" {
		return ErrInvalidInput
	}
	if account.Status == "" {
		account.Status = AccountStatusActive
	}
	if account.CreatedAt.IsZero() {
		account.CreatedAt = time.Now().UTC()
	}
	if account.UpdatedAt.IsZero() {
		account.UpdatedAt = account.CreatedAt
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.accounts[account.ID]; exists {
		return ErrAlreadyExists
	}
	r.accounts[account.ID] = account
	return nil
}

func (r *MemoryRepository) GetAccount(_ context.Context, id string) (Account, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	account, ok := r.accounts[id]
	if !ok {
		return Account{}, ErrNotFound
	}
	return account, nil
}

func identityKey(provider, subject string) string { return provider + "\x00" + subject }

func (r *MemoryRepository) FindIdentity(_ context.Context, provider, subject string) (Identity, error) {
	if provider == "" || subject == "" {
		return Identity{}, ErrInvalidInput
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	identity, ok := r.identities[identityKey(provider, subject)]
	if !ok {
		return Identity{}, ErrNotFound
	}
	return identity, nil
}

func (r *MemoryRepository) CreateIdentity(_ context.Context, identity Identity) error {
	if identity.ID == "" || identity.AccountID == "" || identity.Provider == "" || identity.Subject == "" {
		return ErrInvalidInput
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.accounts[identity.AccountID]; !ok {
		return ErrNotFound
	}
	key := identityKey(identity.Provider, identity.Subject)
	if existing, ok := r.identities[key]; ok {
		if existing.AccountID == identity.AccountID {
			return ErrAlreadyExists
		}
		return ErrIdentityConflict
	}
	if identity.CreatedAt.IsZero() {
		identity.CreatedAt = time.Now().UTC()
	}
	r.identities[key] = identity
	return nil
}

func (r *MemoryRepository) RevokeAccount(_ context.Context, id string, now time.Time) error {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	account, ok := r.accounts[id]
	if !ok {
		return ErrNotFound
	}
	if account.Status != AccountStatusRevoked {
		account.Status = AccountStatusRevoked
		account.RevokedAt = timePtr(now)
		account.UpdatedAt = now
		r.accounts[id] = account
	}
	for sessionID, session := range r.sessions {
		if session.AccountID == id && session.RevokedAt == nil {
			session.RevokedAt = timePtr(now)
			r.sessions[sessionID] = session
		}
	}
	return nil
}

func (r *MemoryRepository) CreateSession(_ context.Context, session Session) error {
	if session.ID == "" || session.AccountID == "" || session.RefreshTokenHash == "" || session.TokenFamilyID == "" {
		return ErrInvalidInput
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.accounts[session.AccountID]; !ok {
		return ErrNotFound
	}
	if _, exists := r.sessions[session.ID]; exists {
		return ErrAlreadyExists
	}
	for _, existing := range r.sessions {
		if existing.RefreshTokenHash == session.RefreshTokenHash {
			return ErrAlreadyExists
		}
		if session.AccessTokenHash != "" && existing.AccessTokenHash == session.AccessTokenHash {
			return ErrAlreadyExists
		}
	}
	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now().UTC()
	}
	r.sessions[session.ID] = session
	return nil
}

func (r *MemoryRepository) FindSessionByAccessHash(_ context.Context, hash string) (Session, error) {
	if hash == "" {
		return Session{}, ErrNotFound
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, session := range r.sessions {
		if session.AccessTokenHash == hash {
			return session, nil
		}
	}
	return Session{}, ErrNotFound
}

func (r *MemoryRepository) FindSessionByRefreshHash(_ context.Context, hash string) (Session, error) {
	if hash == "" {
		return Session{}, ErrNotFound
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, session := range r.sessions {
		if session.RefreshTokenHash == hash {
			return session, nil
		}
	}
	return Session{}, ErrNotFound
}

func (r *MemoryRepository) RotateRefresh(_ context.Context, presentedHash string, replacement Session, now time.Time) (Session, error) {
	if presentedHash == "" || replacement.ID == "" || replacement.RefreshTokenHash == "" || replacement.TokenFamilyID == "" {
		return Session{}, ErrInvalidInput
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var currentID string
	var current Session
	for id, session := range r.sessions {
		if session.RefreshTokenHash == presentedHash {
			currentID, current = id, session
			break
		}
	}
	if currentID == "" {
		return Session{}, ErrNotFound
	}
	if current.RevokedAt != nil {
		if current.FamilyRevokedAt != nil || current.ReplacedByHash != "" || current.RotatedAt != nil {
			for id, session := range r.sessions {
				if session.TokenFamilyID == current.TokenFamilyID {
					if session.RevokedAt == nil {
						session.RevokedAt = timePtr(now)
					}
					if session.FamilyRevokedAt == nil {
						session.FamilyRevokedAt = timePtr(now)
					}
					r.sessions[id] = session
				}
			}
			return Session{}, ErrRefreshReplay
		}
		return Session{}, ErrSessionRevoked
	}
	if !now.Before(current.ExpiresAt) {
		return Session{}, ErrRefreshExpired
	}
	if replacement.AccountID != current.AccountID || replacement.TokenFamilyID != current.TokenFamilyID {
		return Session{}, ErrInvalidInput
	}
	if _, exists := r.sessions[replacement.ID]; exists {
		return Session{}, ErrAlreadyExists
	}
	for _, existing := range r.sessions {
		if existing.RefreshTokenHash == replacement.RefreshTokenHash ||
			(replacement.AccessTokenHash != "" && existing.AccessTokenHash == replacement.AccessTokenHash) {
			return Session{}, ErrAlreadyExists
		}
	}
	current.RevokedAt = timePtr(now)
	current.ReplacedByHash = replacement.RefreshTokenHash
	current.RotatedAt = timePtr(now)
	r.sessions[currentID] = current
	if replacement.CreatedAt.IsZero() {
		replacement.CreatedAt = now
	}
	r.sessions[replacement.ID] = replacement
	return replacement, nil
}

func (r *MemoryRepository) RevokeSession(_ context.Context, id string, now time.Time) error {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	session, ok := r.sessions[id]
	if !ok {
		return ErrNotFound
	}
	if session.RevokedAt == nil {
		session.RevokedAt = timePtr(now)
		r.sessions[id] = session
	}
	return nil
}

func (r *MemoryRepository) RevokeDeviceSessions(_ context.Context, accountID, deviceID string, now time.Time) error {
	if accountID == "" || deviceID == "" {
		return ErrInvalidInput
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, session := range r.sessions {
		if session.AccountID == accountID && session.DeviceID == deviceID && session.RevokedAt == nil {
			session.RevokedAt = timePtr(now)
			r.sessions[id] = session
		}
	}
	return nil
}

func (r *MemoryRepository) RevokeTokenFamily(_ context.Context, familyID string, now time.Time) error {
	if familyID == "" {
		return ErrInvalidInput
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	found := false
	for id, session := range r.sessions {
		if session.TokenFamilyID == familyID {
			found = true
			if session.RevokedAt == nil {
				session.RevokedAt = timePtr(now)
			}
			if session.FamilyRevokedAt == nil {
				session.FamilyRevokedAt = timePtr(now)
			}
			r.sessions[id] = session
		}
	}
	if !found {
		return ErrNotFound
	}
	return nil
}

func (r *MemoryRepository) ListSessionsByAccount(_ context.Context, accountID string) ([]Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Session, 0)
	for _, session := range r.sessions {
		if session.AccountID == accountID && session.RevokedAt == nil {
			out = append(out, session)
		}
	}
	return out, nil
}

func (r *MemoryRepository) RevokeAccountSessions(_ context.Context, accountID string, now time.Time) error {
	if accountID == "" {
		return ErrInvalidInput
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	found := false
	for id, session := range r.sessions {
		if session.AccountID == accountID {
			found = true
			if session.RevokedAt == nil {
				session.RevokedAt = timePtr(now)
				r.sessions[id] = session
			}
		}
	}
	if !found {
		return nil
	}
	return nil
}

func timePtr(t time.Time) *time.Time { return &t }

var _ AccountRepository = (*MemoryRepository)(nil)
var _ SessionRepository = (*MemoryRepository)(nil)
