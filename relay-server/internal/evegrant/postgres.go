package evegrant

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct{ Pool *pgxpool.Pool }

func NewPostgresRepository(pool *pgxpool.Pool) (*PostgresRepository, error) {
	if pool == nil {
		return nil, errors.New("postgres pool is required")
	}
	return &PostgresRepository{Pool: pool}, nil
}

func (r *PostgresRepository) Upsert(ctx context.Context, g EncryptedGrant) error {
	if g.AccountID == "" || g.ProviderSubject == "" || g.KeyID == "" || len(g.Nonce) != 12 || len(g.Ciphertext) == 0 {
		return errors.New("invalid encrypted EVE grant")
	}
	_, err := r.Pool.Exec(ctx, `INSERT INTO eve_refresh_grants(account_id,provider_subject,refresh_ciphertext,refresh_nonce,key_id,scopes,access_expires_at,status,created_at,updated_at)
VALUES($1,$2,$3,$4,$5,COALESCE(string_to_array(NULLIF(btrim($6),''),' '),ARRAY[]::TEXT[]),$7,'active',$8,$9)
ON CONFLICT(account_id) DO UPDATE SET provider_subject=EXCLUDED.provider_subject,refresh_ciphertext=EXCLUDED.refresh_ciphertext,refresh_nonce=EXCLUDED.refresh_nonce,key_id=EXCLUDED.key_id,scopes=EXCLUDED.scopes,access_expires_at=EXCLUDED.access_expires_at,status='active',revoked_at=NULL,token_version=eve_refresh_grants.token_version+1,updated_at=EXCLUDED.updated_at`, g.AccountID, g.ProviderSubject, g.Ciphertext, g.Nonce, g.KeyID, g.Scope, g.ExpiresAt, g.CreatedAt, g.UpdatedAt)
	return err
}

func (r *PostgresRepository) Revoke(ctx context.Context, accountID string) error {
	_, err := r.Pool.Exec(ctx, `UPDATE eve_refresh_grants SET status='revoked',revoked_at=now(),updated_at=now() WHERE account_id=$1`, accountID)
	return err
}

func (r *PostgresRepository) FindByAccount(ctx context.Context, accountID string) (EncryptedGrant, error) {
	var g EncryptedGrant
	var scopes []string
	err := r.Pool.QueryRow(ctx, `SELECT account_id::text,provider_subject,key_id,refresh_nonce,refresh_ciphertext,COALESCE(access_expires_at,'0001-01-01'::timestamptz),scopes,created_at,updated_at FROM eve_refresh_grants WHERE account_id=$1 AND status='active'`, accountID).Scan(&g.AccountID, &g.ProviderSubject, &g.KeyID, &g.Nonce, &g.Ciphertext, &g.ExpiresAt, &scopes, &g.CreatedAt, &g.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return EncryptedGrant{}, ErrNotFound
	}
	g.Scope = joinScopes(scopes)
	return g, err
}
func joinScopes(scopes []string) string {
	var out string
	for _, s := range scopes {
		if s == "" {
			continue
		}
		if out != "" {
			out += " "
		}
		out += s
	}
	return out
}

var _ Repository = (*PostgresRepository)(nil)
