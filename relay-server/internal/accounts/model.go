// Package accounts contains the account and server-session domain. It does
// not expose raw credentials to repositories: token fields on persisted
// models are always cryptographic hashes.
package accounts

import "time"

const (
	AccountStatusActive  = "active"
	AccountStatusRevoked = "revoked"
)

type Account struct {
	ID          string
	Status      string
	DisplayName string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	RevokedAt   *time.Time
}

func (a Account) Active() bool { return a.Status == "" || a.Status == AccountStatusActive }

type Identity struct {
	ID        string
	AccountID string
	Provider  string
	Subject   string
	CreatedAt time.Time
}

// Session is the persisted portion of a server session. AccessTokenHash and
// RefreshTokenHash are hashes of opaque credentials returned at issuance; the
// raw credentials are never part of this value.
type Session struct {
	ID               string
	AccountID        string
	DeviceID         string
	AccessTokenHash  string
	AccessExpiresAt  time.Time
	RefreshTokenHash string
	TokenFamilyID    string
	ExpiresAt        time.Time
	RevokedAt        *time.Time
	// FamilyRevokedAt distinguishes a family-wide replay revocation from a
	// normal single-session/account revoke for stable replay errors.
	FamilyRevokedAt *time.Time
	ReplacedByHash  string
	RotatedAt       *time.Time
	CreatedAt       time.Time
	LastSeenAt      *time.Time
}

func (s Session) Active(now time.Time) bool {
	return s.RevokedAt == nil && now.Before(s.ExpiresAt)
}

// Credentials contains the one-time raw values sent to a client. Callers
// should not persist or log this value. Session contains only safe persisted
// metadata and hashes.
type Credentials struct {
	Session         Session
	AccessToken     string
	RefreshToken    string
	AccessExpiresAt time.Time
	ExpiresAt       time.Time
}
