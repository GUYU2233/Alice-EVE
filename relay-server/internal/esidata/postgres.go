package esidata

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresRepository persists normalized ESI data in PostgreSQL.
type PostgresRepository struct {
	Pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) (*PostgresRepository, error) {
	if pool == nil {
		return nil, errors.New("postgres pool is required")
	}
	return &PostgresRepository{Pool: pool}, nil
}

func (r *PostgresRepository) UpsertCharacterSnapshot(ctx context.Context, snapshot CharacterSnapshot) error {
	if !validCharacterSnapshot(snapshot) {
		return ErrInvalidInput
	}
	_, err := r.Pool.Exec(ctx, `
		INSERT INTO character_snapshots
			(account_id, character_id, domain, payload, fetched_at, expires_at, stale, source, etag)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, ''))
		ON CONFLICT (account_id, character_id, domain) DO UPDATE SET
			payload = EXCLUDED.payload,
			fetched_at = EXCLUDED.fetched_at,
			expires_at = EXCLUDED.expires_at,
			stale = EXCLUDED.stale,
			source = EXCLUDED.source,
			etag = EXCLUDED.etag`,
		snapshot.AccountID, snapshot.CharacterID, snapshot.Domain, []byte(snapshot.Payload),
		snapshot.FetchedAt, snapshot.ExpiresAt, snapshot.Stale, snapshot.Source, snapshot.ETag)
	return err
}

func (r *PostgresRepository) GetCharacterSnapshot(ctx context.Context, accountID string, characterID int64, domain string) (CharacterSnapshot, error) {
	if accountID == "" || characterID <= 0 || domain == "" {
		return CharacterSnapshot{}, ErrInvalidInput
	}
	var snapshot CharacterSnapshot
	var payload []byte
	err := r.Pool.QueryRow(ctx, `
		SELECT account_id::text, character_id, domain, payload, fetched_at, expires_at,
		       stale, source, COALESCE(etag, '')
		FROM character_snapshots
		WHERE account_id = $1 AND character_id = $2 AND domain = $3`,
		accountID, characterID, domain).Scan(
		&snapshot.AccountID, &snapshot.CharacterID, &snapshot.Domain, &payload,
		&snapshot.FetchedAt, &snapshot.ExpiresAt, &snapshot.Stale, &snapshot.Source, &snapshot.ETag)
	if errors.Is(err, pgx.ErrNoRows) {
		return CharacterSnapshot{}, ErrNotFound
	}
	snapshot.Payload = payload
	return snapshot, err
}

func (r *PostgresRepository) ListAccountCharacters(ctx context.Context, accountID string) ([]CharacterSnapshot, error) {
	if accountID == "" {
		return nil, ErrInvalidInput
	}
	rows, err := r.Pool.Query(ctx, `
		SELECT account_id::text, character_id, domain, payload, fetched_at, expires_at,
		       stale, source, COALESCE(etag, '')
		FROM character_snapshots
		WHERE account_id = $1
		ORDER BY character_id, domain`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]CharacterSnapshot, 0)
	for rows.Next() {
		var snapshot CharacterSnapshot
		var payload []byte
		if err := rows.Scan(&snapshot.AccountID, &snapshot.CharacterID, &snapshot.Domain, &payload,
			&snapshot.FetchedAt, &snapshot.ExpiresAt, &snapshot.Stale, &snapshot.Source, &snapshot.ETag); err != nil {
			return nil, err
		}
		snapshot.Payload = payload
		out = append(out, snapshot)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) ListCharacterDomains(ctx context.Context, accountID string, characterID int64) ([]CharacterSnapshot, error) {
	if accountID == "" || characterID <= 0 {
		return nil, ErrInvalidInput
	}
	rows, err := r.Pool.Query(ctx, `
		SELECT account_id::text, character_id, domain, payload, fetched_at, expires_at,
		       stale, source, COALESCE(etag, '')
		FROM character_snapshots
		WHERE account_id = $1 AND character_id = $2
		ORDER BY domain`, accountID, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]CharacterSnapshot, 0)
	for rows.Next() {
		var snapshot CharacterSnapshot
		var payload []byte
		if err := rows.Scan(
			&snapshot.AccountID, &snapshot.CharacterID, &snapshot.Domain, &payload,
			&snapshot.FetchedAt, &snapshot.ExpiresAt, &snapshot.Stale, &snapshot.Source, &snapshot.ETag,
		); err != nil {
			return nil, err
		}
		snapshot.Payload = payload
		out = append(out, snapshot)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) UpsertPublicData(ctx context.Context, entry PublicData) error {
	if !validPublicData(entry) {
		return ErrInvalidInput
	}
	_, err := r.Pool.Exec(ctx, `
		INSERT INTO public_data_cache (kind, cache_key, payload, fetched_at, expires_at, etag)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''))
		ON CONFLICT (kind, cache_key) DO UPDATE SET
			payload = EXCLUDED.payload,
			fetched_at = EXCLUDED.fetched_at,
			expires_at = EXCLUDED.expires_at,
			etag = EXCLUDED.etag`,
		entry.Kind, entry.CacheKey, []byte(entry.Payload), entry.FetchedAt, entry.ExpiresAt, entry.ETag)
	return err
}

func (r *PostgresRepository) GetPublicData(ctx context.Context, kind, cacheKey string) (PublicData, error) {
	if kind == "" || cacheKey == "" {
		return PublicData{}, ErrInvalidInput
	}
	var entry PublicData
	var payload []byte
	err := r.Pool.QueryRow(ctx, `
		SELECT kind, cache_key, payload, fetched_at, expires_at, COALESCE(etag, '')
		FROM public_data_cache WHERE kind = $1 AND cache_key = $2`, kind, cacheKey).Scan(
		&entry.Kind, &entry.CacheKey, &payload, &entry.FetchedAt, &entry.ExpiresAt, &entry.ETag)
	if errors.Is(err, pgx.ErrNoRows) {
		return PublicData{}, ErrNotFound
	}
	entry.Payload = payload
	return entry, err
}

var _ Repository = (*PostgresRepository)(nil)
