package accounts

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresRepository persists account and session state. Session rotation uses
// a row lock and one transaction so concurrent reuse can never mint two
// replacements from one refresh token.
type PostgresRepository struct {
	Pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) (*PostgresRepository, error) {
	if pool == nil {
		return nil, errors.New("postgres pool is required")
	}
	return &PostgresRepository{Pool: pool}, nil
}

func (r *PostgresRepository) CreateAccount(ctx context.Context, account Account) error {
	_, err := r.Pool.Exec(ctx, `INSERT INTO accounts(id,status,display_name,revoked_at,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6)`, account.ID, statusOrActive(account.Status), account.DisplayName, account.RevokedAt, account.CreatedAt, account.UpdatedAt)
	return mapConstraintError(err)
}

func (r *PostgresRepository) GetAccount(ctx context.Context, id string) (Account, error) {
	var account Account
	err := r.Pool.QueryRow(ctx, `SELECT id::text,status,COALESCE(display_name,''),created_at,updated_at,revoked_at FROM accounts WHERE id=$1`, id).Scan(&account.ID, &account.Status, &account.DisplayName, &account.CreatedAt, &account.UpdatedAt, &account.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	return account, err
}

func (r *PostgresRepository) FindIdentity(ctx context.Context, provider, subject string) (Identity, error) {
	var identity Identity
	err := r.Pool.QueryRow(ctx, `SELECT id::text,account_id::text,provider,subject,created_at FROM identities WHERE provider=$1 AND subject=$2`, provider, subject).Scan(&identity.ID, &identity.AccountID, &identity.Provider, &identity.Subject, &identity.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Identity{}, ErrNotFound
	}
	return identity, err
}

func (r *PostgresRepository) CreateIdentity(ctx context.Context, identity Identity) error {
	_, err := r.Pool.Exec(ctx, `INSERT INTO identities(id,account_id,provider,subject,created_at) VALUES($1,$2,$3,$4,$5)`, identity.ID, identity.AccountID, identity.Provider, identity.Subject, identity.CreatedAt)
	return mapConstraintError(err)
}

func (r *PostgresRepository) RevokeAccount(ctx context.Context, id string, now time.Time) error {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	result, err := tx.Exec(ctx, `UPDATE accounts SET status='revoked',revoked_at=COALESCE(revoked_at,$2),updated_at=$2 WHERE id=$1`, id, now)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	if _, err = tx.Exec(ctx, `UPDATE sessions SET revoked_at=COALESCE(revoked_at,$2) WHERE account_id=$1`, id, now); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) CreateSession(ctx context.Context, session Session) error {
	_, err := r.Pool.Exec(ctx, `INSERT INTO sessions(id,account_id,device_id,access_token_hash,access_expires_at,refresh_token_hash,token_family_id,expires_at,revoked_at,family_revoked_at,replaced_by_hash,rotated_at,created_at,last_seen_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, session.ID, session.AccountID, nullableString(session.DeviceID), session.AccessTokenHash, session.AccessExpiresAt, session.RefreshTokenHash, session.TokenFamilyID, session.ExpiresAt, session.RevokedAt, session.FamilyRevokedAt, nullableString(session.ReplacedByHash), session.RotatedAt, session.CreatedAt, session.LastSeenAt)
	return mapConstraintError(err)
}

func (r *PostgresRepository) FindSessionByAccessHash(ctx context.Context, hash string) (Session, error) {
	return r.findSession(ctx, `SELECT id::text,account_id::text,COALESCE(device_id,''),access_token_hash,access_expires_at,refresh_token_hash,token_family_id::text,expires_at,revoked_at,family_revoked_at,COALESCE(replaced_by_hash,''),rotated_at,created_at,last_seen_at FROM sessions WHERE access_token_hash=$1`, hash)
}

func (r *PostgresRepository) FindSessionByRefreshHash(ctx context.Context, hash string) (Session, error) {
	return r.findSession(ctx, `SELECT id::text,account_id::text,COALESCE(device_id,''),access_token_hash,access_expires_at,refresh_token_hash,token_family_id::text,expires_at,revoked_at,family_revoked_at,COALESCE(replaced_by_hash,''),rotated_at,created_at,last_seen_at FROM sessions WHERE refresh_token_hash=$1`, hash)
}

func (r *PostgresRepository) findSession(ctx context.Context, query string, hash string) (Session, error) {
	var session Session
	err := r.Pool.QueryRow(ctx, query, hash).Scan(&session.ID, &session.AccountID, &session.DeviceID, &session.AccessTokenHash, &session.AccessExpiresAt, &session.RefreshTokenHash, &session.TokenFamilyID, &session.ExpiresAt, &session.RevokedAt, &session.FamilyRevokedAt, &session.ReplacedByHash, &session.RotatedAt, &session.CreatedAt, &session.LastSeenAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	return session, err
}

func (r *PostgresRepository) RotateRefresh(ctx context.Context, presentedHash string, replacement Session, now time.Time) (Session, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return Session{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var current Session
	err = tx.QueryRow(ctx, `SELECT id::text,account_id::text,COALESCE(device_id,''),access_token_hash,access_expires_at,refresh_token_hash,token_family_id::text,expires_at,revoked_at,family_revoked_at,COALESCE(replaced_by_hash,''),rotated_at,created_at,last_seen_at FROM sessions WHERE refresh_token_hash=$1 FOR UPDATE`, presentedHash).Scan(&current.ID, &current.AccountID, &current.DeviceID, &current.AccessTokenHash, &current.AccessExpiresAt, &current.RefreshTokenHash, &current.TokenFamilyID, &current.ExpiresAt, &current.RevokedAt, &current.FamilyRevokedAt, &current.ReplacedByHash, &current.RotatedAt, &current.CreatedAt, &current.LastSeenAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, err
	}
	if current.RevokedAt != nil {
		if current.ReplacedByHash != "" || current.RotatedAt != nil {
			if _, err = tx.Exec(ctx, `UPDATE sessions SET revoked_at=COALESCE(revoked_at,$2),family_revoked_at=COALESCE(family_revoked_at,$2) WHERE token_family_id=$1`, current.TokenFamilyID, now); err != nil {
				return Session{}, err
			}
			if err = tx.Commit(ctx); err != nil {
				return Session{}, err
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
	if _, err = tx.Exec(ctx, `UPDATE sessions SET revoked_at=$2,replaced_by_hash=$3,rotated_at=$2 WHERE id=$1`, current.ID, now, replacement.RefreshTokenHash); err != nil {
		return Session{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO sessions(id,account_id,device_id,access_token_hash,access_expires_at,refresh_token_hash,token_family_id,expires_at,revoked_at,family_revoked_at,replaced_by_hash,rotated_at,created_at,last_seen_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, replacement.ID, replacement.AccountID, nullableString(replacement.DeviceID), replacement.AccessTokenHash, replacement.AccessExpiresAt, replacement.RefreshTokenHash, replacement.TokenFamilyID, replacement.ExpiresAt, replacement.RevokedAt, replacement.FamilyRevokedAt, nullableString(replacement.ReplacedByHash), replacement.RotatedAt, replacement.CreatedAt, replacement.LastSeenAt)
	if err != nil {
		return Session{}, mapConstraintError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return Session{}, err
	}
	return replacement, nil
}

func (r *PostgresRepository) RevokeSession(ctx context.Context, id string, now time.Time) error {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	result, err := r.Pool.Exec(ctx, `UPDATE sessions SET revoked_at=COALESCE(revoked_at,$2) WHERE id=$1`, id, now)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) RevokeDeviceSessions(ctx context.Context, accountID, deviceID string, now time.Time) error {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	_, err := r.Pool.Exec(ctx, `UPDATE sessions SET revoked_at=COALESCE(revoked_at,$3) WHERE account_id=$1 AND device_id=$2`, accountID, deviceID, now)
	return err
}

func (r *PostgresRepository) RevokeTokenFamily(ctx context.Context, familyID string, now time.Time) error {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	result, err := r.Pool.Exec(ctx, `UPDATE sessions SET revoked_at=COALESCE(revoked_at,$2),family_revoked_at=COALESCE(family_revoked_at,$2) WHERE token_family_id=$1`, familyID, now)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) ListSessionsByAccount(ctx context.Context, accountID string) ([]Session, error) {
	rows, err := r.Pool.Query(ctx, `SELECT id::text,account_id::text,COALESCE(device_id,''),access_token_hash,access_expires_at,refresh_token_hash,token_family_id::text,expires_at,revoked_at,family_revoked_at,COALESCE(replaced_by_hash,''),rotated_at,created_at,last_seen_at FROM sessions WHERE account_id=$1 AND revoked_at IS NULL ORDER BY last_seen_at DESC NULLS LAST, created_at DESC`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Session, 0)
	for rows.Next() {
		var session Session
		if err := rows.Scan(&session.ID, &session.AccountID, &session.DeviceID, &session.AccessTokenHash, &session.AccessExpiresAt, &session.RefreshTokenHash, &session.TokenFamilyID, &session.ExpiresAt, &session.RevokedAt, &session.FamilyRevokedAt, &session.ReplacedByHash, &session.RotatedAt, &session.CreatedAt, &session.LastSeenAt); err != nil {
			return nil, err
		}
		out = append(out, session)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) RevokeAccountSessions(ctx context.Context, accountID string, now time.Time) error {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	_, err := r.Pool.Exec(ctx, `UPDATE sessions SET revoked_at=COALESCE(revoked_at,$2) WHERE account_id=$1`, accountID, now)
	return err
}

func statusOrActive(status string) string {
	if status == "" {
		return AccountStatusActive
	}
	return status
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func mapConstraintError(err error) error {
	if err == nil {
		return nil
	}
	// Keep pgx's detailed error available to callers while mapping only the
	// domain-relevant uniqueness conflict that EnsureAccount can recover from.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "23505" {
			return ErrAlreadyExists
		}
	}
	return err
}

var _ AccountRepository = (*PostgresRepository)(nil)
var _ SessionRepository = (*PostgresRepository)(nil)
