package accounts

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"relay-server/internal/authn"
)

func newTestService(t *testing.T) (*Service, *MemoryRepository) {
	t.Helper()
	repo := NewMemoryRepository()
	now := time.Unix(1_700_000_000, 0).UTC()
	service, err := NewService(repo, repo, Config{
		AccessTokenTTL:  time.Minute,
		RefreshTokenTTL: time.Hour,
		Now:             func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	return service, repo
}

func TestEnsureAccountReusesProviderIdentity(t *testing.T) {
	service, _ := newTestService(t)
	first, err := service.EnsureAccount(context.Background(), "eve", "subject-1", "Alice")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.EnsureAccount(context.Background(), "eve", "subject-1", "Renamed")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("identity created a second account: %q != %q", first.ID, second.ID)
	}
	if second.DisplayName != first.DisplayName {
		t.Fatalf("existing account unexpectedly changed display name: %q", second.DisplayName)
	}
}

func TestSessionOnlyStoresTokenHashes(t *testing.T) {
	service, repo := newTestService(t)
	account, err := service.EnsureAccount(context.Background(), "eve", "subject-2", "Alice")
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := service.IssueSession(context.Background(), account.ID, "device-1")
	if err != nil {
		t.Fatal(err)
	}
	if credentials.AccessToken == "" || credentials.RefreshToken == "" {
		t.Fatal("raw credentials were not issued")
	}
	if credentials.Session.AccessTokenHash != authn.HashToken(credentials.AccessToken) || credentials.Session.RefreshTokenHash != authn.HashToken(credentials.RefreshToken) {
		t.Fatal("stored token hash does not match issued credential")
	}
	if strings.Contains(credentials.Session.AccessTokenHash, credentials.AccessToken) || strings.Contains(credentials.Session.RefreshTokenHash, credentials.RefreshToken) {
		t.Fatal("raw credential was persisted in session")
	}
	stored, err := repo.FindSessionByRefreshHash(context.Background(), credentials.Session.RefreshTokenHash)
	if err != nil {
		t.Fatal(err)
	}
	if stored.RefreshTokenHash == credentials.RefreshToken {
		t.Fatal("raw refresh token persisted")
	}
}

func TestRefreshRotatesAndReplayRevokesFamily(t *testing.T) {
	service, repo := newTestService(t)
	account, err := service.EnsureAccount(context.Background(), "eve", "subject-3", "Alice")
	if err != nil {
		t.Fatal(err)
	}
	initial, err := service.IssueSession(context.Background(), account.ID, "device-1")
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := service.Refresh(context.Background(), initial.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.RefreshToken == initial.RefreshToken || rotated.Session.ID == initial.Session.ID {
		t.Fatal("refresh token was not rotated")
	}
	if _, err = service.Refresh(context.Background(), initial.RefreshToken); !errors.Is(err, ErrRefreshReplay) {
		t.Fatalf("replayed token error = %v, want %v", err, ErrRefreshReplay)
	}
	if _, err = service.Refresh(context.Background(), rotated.RefreshToken); !errors.Is(err, ErrRefreshReplay) {
		t.Fatalf("family token remained usable after replay: %v", err)
	}
	old, err := repo.FindSessionByRefreshHash(context.Background(), initial.Session.RefreshTokenHash)
	if err != nil {
		t.Fatal(err)
	}
	newSession, err := repo.FindSessionByRefreshHash(context.Background(), rotated.Session.RefreshTokenHash)
	if err != nil {
		t.Fatal(err)
	}
	if old.RevokedAt == nil || newSession.RevokedAt == nil {
		t.Fatal("replay did not revoke complete token family")
	}
}

func TestRefreshPreservesAbsoluteExpiry(t *testing.T) {
	service, _ := newTestService(t)
	account, err := service.EnsureAccount(context.Background(), "eve", "subject-4", "Alice")
	if err != nil {
		t.Fatal(err)
	}
	initial, err := service.IssueSession(context.Background(), account.ID, "device-1")
	if err != nil {
		t.Fatal(err)
	}
	refreshAt := initial.Session.CreatedAt.Add(10 * time.Minute)
	rotated, err := service.RefreshAt(context.Background(), initial.RefreshToken, refreshAt)
	if err != nil {
		t.Fatal(err)
	}
	if !rotated.ExpiresAt.Equal(initial.ExpiresAt) {
		t.Fatalf("refresh expiry extended: got %v, want %v", rotated.ExpiresAt, initial.ExpiresAt)
	}
}

func TestAccountRevokeInvalidatesAccessAndRefresh(t *testing.T) {
	service, _ := newTestService(t)
	account, err := service.EnsureAccount(context.Background(), "eve", "subject-5", "Alice")
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := service.IssueSession(context.Background(), account.ID, "device-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.RevokeAccount(context.Background(), account.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AuthenticateAccess(context.Background(), credentials.AccessToken); !errors.Is(err, ErrSessionRevoked) && !errors.Is(err, ErrAccountRevoked) {
		t.Fatalf("revoked access accepted: %v", err)
	}
	if _, err := service.Refresh(context.Background(), credentials.RefreshToken); !errors.Is(err, ErrSessionRevoked) && !errors.Is(err, ErrAccountRevoked) {
		t.Fatalf("revoked refresh accepted: %v", err)
	}
}

func TestExpiredRefreshIsRejected(t *testing.T) {
	service, _ := newTestService(t)
	account, err := service.EnsureAccount(context.Background(), "eve", "subject-6", "Alice")
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := service.IssueSession(context.Background(), account.ID, "device-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RefreshAt(context.Background(), credentials.RefreshToken, credentials.ExpiresAt.Add(time.Nanosecond)); !errors.Is(err, ErrRefreshExpired) {
		t.Fatalf("expired refresh error = %v, want %v", err, ErrRefreshExpired)
	}
}
