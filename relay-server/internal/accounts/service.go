package accounts

import (
	"context"
	"errors"
	"strings"
	"time"

	"relay-server/internal/authn"
)

// Config controls server-issued credential lifetimes. Refresh expiry is an
// absolute session-family lifetime: rotating a token does not extend the
// family beyond its original expiry.
type Config struct {
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	Now             func() time.Time
}

func (c Config) withDefaults() Config {
	if c.AccessTokenTTL <= 0 {
		c.AccessTokenTTL = 15 * time.Minute
	}
	if c.RefreshTokenTTL <= 0 {
		c.RefreshTokenTTL = 30 * 24 * time.Hour
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	return c
}

// Service owns account/session state transitions while repositories own
// persistence. It never returns a raw token from a lookup operation.
type Service struct {
	Accounts AccountRepository
	Sessions SessionRepository
	Config   Config
}

func NewService(accountRepo AccountRepository, sessionRepo SessionRepository, config Config) (*Service, error) {
	if accountRepo == nil || sessionRepo == nil {
		return nil, errors.New("account and session repositories are required")
	}
	return &Service{Accounts: accountRepo, Sessions: sessionRepo, Config: config.withDefaults()}, nil
}

// EnsureAccount returns the account linked to an identity, creating both
// account and identity when the identity has not been seen before. A race is
// resolved by re-reading an identity after a uniqueness conflict.
func (s *Service) EnsureAccount(ctx context.Context, provider, subject, displayName string) (Account, error) {
	if strings.TrimSpace(provider) == "" || strings.TrimSpace(subject) == "" {
		return Account{}, ErrInvalidInput
	}
	identity, err := s.Accounts.FindIdentity(ctx, provider, subject)
	if err == nil {
		return s.Accounts.GetAccount(ctx, identity.AccountID)
	}
	if !errors.Is(err, ErrNotFound) {
		return Account{}, err
	}
	accountID, err := newID()
	if err != nil {
		return Account{}, err
	}
	now := s.Config.Now().UTC()
	account := Account{ID: accountID, Status: AccountStatusActive, DisplayName: displayName, CreatedAt: now, UpdatedAt: now}
	if err = s.Accounts.CreateAccount(ctx, account); err != nil && !errors.Is(err, ErrAlreadyExists) {
		return Account{}, err
	}
	identityID, err := newID()
	if err != nil {
		return Account{}, err
	}
	identity = Identity{ID: identityID, AccountID: accountID, Provider: provider, Subject: subject, CreatedAt: now}
	if err = s.Accounts.CreateIdentity(ctx, identity); err != nil {
		if errors.Is(err, ErrIdentityConflict) || errors.Is(err, ErrAlreadyExists) {
			if existing, findErr := s.Accounts.FindIdentity(ctx, provider, subject); findErr == nil {
				return s.Accounts.GetAccount(ctx, existing.AccountID)
			}
		}
		return Account{}, err
	}
	return account, nil
}

// IssueSession creates a new token family and returns credentials exactly once
// to the caller. Only hashes are sent to the repository.
func (s *Service) IssueSession(ctx context.Context, accountID, deviceID string) (Credentials, error) {
	account, err := s.Accounts.GetAccount(ctx, accountID)
	if err != nil {
		return Credentials{}, err
	}
	if !account.Active() {
		return Credentials{}, ErrAccountRevoked
	}
	access, err := authn.GenerateToken()
	if err != nil {
		return Credentials{}, err
	}
	refresh, err := authn.GenerateToken()
	if err != nil {
		return Credentials{}, err
	}
	familyID, err := newID()
	if err != nil {
		return Credentials{}, err
	}
	sessionID, err := newID()
	if err != nil {
		return Credentials{}, err
	}
	now := s.Config.Now().UTC()
	accessExpiresAt := now.Add(s.Config.AccessTokenTTL)
	expiresAt := now.Add(s.Config.RefreshTokenTTL)
	session := Session{
		ID: sessionID, AccountID: accountID, DeviceID: deviceID,
		AccessTokenHash: authn.HashToken(access), AccessExpiresAt: accessExpiresAt,
		RefreshTokenHash: authn.HashToken(refresh), TokenFamilyID: familyID,
		ExpiresAt: expiresAt, CreatedAt: now, LastSeenAt: timePtr(now),
	}
	if err := s.Sessions.CreateSession(ctx, session); err != nil {
		return Credentials{}, err
	}
	return Credentials{Session: session, AccessToken: access, RefreshToken: refresh, AccessExpiresAt: accessExpiresAt, ExpiresAt: expiresAt}, nil
}

// AuthenticateAccess validates both the access-token expiry and the account /
// session revocation state. The raw value is hashed before repository access.
func (s *Service) AuthenticateAccess(ctx context.Context, raw string) (Session, error) {
	if strings.TrimSpace(raw) == "" {
		return Session{}, ErrNotFound
	}
	session, err := s.Sessions.FindSessionByAccessHash(ctx, authn.HashToken(raw))
	if err != nil {
		return Session{}, err
	}
	now := s.Config.Now().UTC()
	if !now.Before(session.AccessExpiresAt) {
		return Session{}, ErrSessionRevoked
	}
	if session.RevokedAt != nil {
		return Session{}, ErrSessionRevoked
	}
	account, err := s.Accounts.GetAccount(ctx, session.AccountID)
	if err != nil {
		return Session{}, err
	}
	if !account.Active() {
		return Session{}, ErrAccountRevoked
	}
	return session, nil
}

// Refresh rotates a refresh credential. If a previously rotated credential is
// presented, repository-level atomic logic revokes every session in its token
// family and returns ErrRefreshReplay.
func (s *Service) Refresh(ctx context.Context, raw string) (Credentials, error) {
	return s.RefreshAt(ctx, raw, s.Config.Now())
}

func (s *Service) RefreshAt(ctx context.Context, raw string, now time.Time) (Credentials, error) {
	if strings.TrimSpace(raw) == "" {
		return Credentials{}, ErrNotFound
	}
	if now.IsZero() {
		now = s.Config.Now()
	}
	presentedHash := authn.HashToken(raw)
	current, err := s.Sessions.FindSessionByRefreshHash(ctx, presentedHash)
	if err != nil {
		return Credentials{}, err
	}
	if current.RevokedAt != nil {
		if current.FamilyRevokedAt != nil || current.ReplacedByHash != "" || current.RotatedAt != nil {
			_ = s.Sessions.RevokeTokenFamily(ctx, current.TokenFamilyID, now.UTC())
			return Credentials{}, ErrRefreshReplay
		}
		return Credentials{}, ErrSessionRevoked
	}
	if !now.Before(current.ExpiresAt) {
		return Credentials{}, ErrRefreshExpired
	}
	account, err := s.Accounts.GetAccount(ctx, current.AccountID)
	if err != nil {
		return Credentials{}, err
	}
	if !account.Active() {
		return Credentials{}, ErrAccountRevoked
	}
	access, err := authn.GenerateToken()
	if err != nil {
		return Credentials{}, err
	}
	refresh, err := authn.GenerateToken()
	if err != nil {
		return Credentials{}, err
	}
	sessionID, err := newID()
	if err != nil {
		return Credentials{}, err
	}
	now = now.UTC()
	accessExpiresAt := now.Add(s.Config.AccessTokenTTL)
	replacement := Session{
		ID: sessionID, AccountID: current.AccountID, DeviceID: current.DeviceID,
		AccessTokenHash: authn.HashToken(access), AccessExpiresAt: accessExpiresAt,
		RefreshTokenHash: authn.HashToken(refresh), TokenFamilyID: current.TokenFamilyID,
		ExpiresAt: current.ExpiresAt, CreatedAt: now, LastSeenAt: timePtr(now),
	}
	rotated, err := s.Sessions.RotateRefresh(ctx, presentedHash, replacement, now)
	if err != nil {
		return Credentials{}, err
	}
	return Credentials{Session: rotated, AccessToken: access, RefreshToken: refresh, AccessExpiresAt: accessExpiresAt, ExpiresAt: rotated.ExpiresAt}, nil
}

func (s *Service) ListSessions(ctx context.Context, accountID string) ([]Session, error) {
	if strings.TrimSpace(accountID) == "" {
		return nil, ErrInvalidInput
	}
	return s.Sessions.ListSessionsByAccount(ctx, accountID)
}

func (s *Service) RevokeSession(ctx context.Context, sessionID string) error {
	return s.Sessions.RevokeSession(ctx, sessionID, s.Config.Now().UTC())
}

// RevokeDevice revokes every modern session belonging to one account/device.
// Device IDs are tenant-scoped by the account ID in the repository predicate.
func (s *Service) RevokeDevice(ctx context.Context, accountID, deviceID string) error {
	if strings.TrimSpace(accountID) == "" || strings.TrimSpace(deviceID) == "" {
		return ErrInvalidInput
	}
	return s.Sessions.RevokeDeviceSessions(ctx, accountID, deviceID, s.Config.Now().UTC())
}

// RevokeAccount changes the account status and revokes all of its sessions.
func (s *Service) RevokeAccount(ctx context.Context, accountID string) error {
	now := s.Config.Now().UTC()
	if err := s.Accounts.RevokeAccount(ctx, accountID, now); err != nil {
		return err
	}
	return s.Sessions.RevokeAccountSessions(ctx, accountID, now)
}
