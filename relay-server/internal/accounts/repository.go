package accounts

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound         = errors.New("account or session not found")
	ErrIdentityConflict = errors.New("identity already belongs to another account")
	ErrAlreadyExists    = errors.New("account or session already exists")
	ErrAccountRevoked   = errors.New("account is revoked")
	ErrSessionRevoked   = errors.New("session is revoked")
	ErrRefreshExpired   = errors.New("refresh token is expired")
	ErrRefreshReplay    = errors.New("refresh token replay detected; token family revoked")
	ErrInvalidInput     = errors.New("invalid account or session input")
)

// AccountRepository is the persistence boundary for accounts and external
// identities. Implementations must make identity lookup unique by provider
// and subject.
type AccountRepository interface {
	CreateAccount(context.Context, Account) error
	GetAccount(context.Context, string) (Account, error)
	FindIdentity(context.Context, string, string) (Identity, error)
	CreateIdentity(context.Context, Identity) error
	RevokeAccount(context.Context, string, time.Time) error
}

// SessionRepository is the persistence boundary for server sessions. The
// RotateRefresh operation must be atomic: it invalidates presentedHash and
// inserts replacement as one state transition. A previously rotated token
// must revoke its complete token family before returning ErrRefreshReplay.
type SessionRepository interface {
	CreateSession(context.Context, Session) error
	FindSessionByAccessHash(context.Context, string) (Session, error)
	FindSessionByRefreshHash(context.Context, string) (Session, error)
	RotateRefresh(context.Context, string, Session, time.Time) (Session, error)
	RevokeSession(context.Context, string, time.Time) error
	RevokeDeviceSessions(context.Context, string, string, time.Time) error
	RevokeTokenFamily(context.Context, string, time.Time) error
	RevokeAccountSessions(context.Context, string, time.Time) error
	ListSessionsByAccount(context.Context, string) ([]Session, error)
}
