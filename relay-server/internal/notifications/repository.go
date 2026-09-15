package notifications

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	GetPreferences(context.Context, string) (PreferencesDocument, error)
	UpdatePreferences(context.Context, UpdateRequest) (PreferencesDocument, error)
	RegisterToken(context.Context, TokenRegistration) (PushToken, error)
	ListTokens(context.Context, string) ([]PushToken, error)
	RevokeToken(context.Context, string, string, string) error
}

type PostgresRepository struct{ Pool *pgxpool.Pool }

func NewPostgresRepository(pool *pgxpool.Pool) (*PostgresRepository, error) {
	if pool == nil {
		return nil, errors.New("notifications: postgres pool is required")
	}
	return &PostgresRepository{Pool: pool}, nil
}
func (r *PostgresRepository) GetPreferences(ctx context.Context, device string) (PreferencesDocument, error) {
	var d PreferencesDocument
	var raw []byte
	err := r.Pool.QueryRow(ctx, `SELECT device_id,version,etag,preferences,updated_at FROM notification_preferences WHERE device_id=$1`, device).Scan(&d.DeviceID, &d.Version, &d.ETag, &raw, &d.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		p := DefaultPreferences()
		now := time.Now().UTC()
		d = PreferencesDocument{DeviceID: device, Version: 1, ETag: etag(1, p), Preferences: p, UpdatedAt: now}
		b, _ := json.Marshal(p)
		_, err = r.Pool.Exec(ctx, `INSERT INTO notification_preferences(device_id,version,etag,preferences,updated_at) VALUES($1,1,$2,$3,$4) ON CONFLICT (device_id) DO NOTHING`, device, d.ETag, b, now)
		if err != nil {
			return PreferencesDocument{}, err
		}
		return r.GetPreferences(ctx, device)
	}
	if err != nil {
		return d, err
	}
	if json.Unmarshal(raw, &d.Preferences) != nil {
		return PreferencesDocument{}, errors.New("notifications: invalid preferences")
	}
	return d, nil
}
func (r *PostgresRepository) UpdatePreferences(ctx context.Context, req UpdateRequest) (PreferencesDocument, error) {
	current, err := r.GetPreferences(ctx, req.DeviceID)
	if err != nil {
		return PreferencesDocument{}, err
	}
	if req.BaseVersion < 1 || current.Version != req.BaseVersion || strings.Trim(strings.TrimSpace(req.IfMatch), `"`) != current.ETag {
		return PreferencesDocument{}, &ConflictError{Current: current}
	}
	next := current
	next.Version++
	next.Preferences = req.Preferences
	next.ETag = etag(next.Version, next.Preferences)
	next.UpdatedAt = time.Now().UTC()
	b, _ := json.Marshal(next.Preferences)
	var raw []byte
	err = r.Pool.QueryRow(ctx, `UPDATE notification_preferences SET version=version+1,etag=$3,preferences=$4,updated_at=$5 WHERE device_id=$1 AND version=$2 AND etag=$6 RETURNING device_id,version,etag,preferences,updated_at`, req.DeviceID, req.BaseVersion, next.ETag, b, next.UpdatedAt, current.ETag).Scan(&next.DeviceID, &next.Version, &next.ETag, &raw, &next.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		cur, _ := r.GetPreferences(ctx, req.DeviceID)
		return PreferencesDocument{}, &ConflictError{Current: cur}
	}
	if err != nil {
		return PreferencesDocument{}, err
	}
	_ = json.Unmarshal(raw, &next.Preferences)
	return next, nil
}
func hashToken(raw string) string {
	h := sha256.Sum256([]byte(strings.TrimSpace(raw)))
	return hex.EncodeToString(h[:])
}
func (r *PostgresRepository) RegisterToken(ctx context.Context, req TokenRegistration) (PushToken, error) {
	req.Provider = strings.ToLower(strings.TrimSpace(req.Provider))
	req.Platform = strings.ToLower(strings.TrimSpace(req.Platform))
	req.Token = strings.TrimSpace(req.Token)
	ps, ok := providerPlatforms[req.Provider]
	if !ok {
		return PushToken{}, ErrInvalidProvider
	}
	if len(req.Token) < 16 || len(req.Token) > 4096 {
		return PushToken{}, ErrInvalidToken
	}
	if req.Platform == "" {
		req.Platform = ps[0]
	}
	valid := false
	for _, p := range ps {
		if req.Platform == p {
			valid = true
		}
	}
	if !valid {
		return PushToken{}, ErrInvalidPlatform
	}
	h := hashToken(req.Token)
	id := h[:24]
	// IDs are opaque and must not collide when the same provider token is
	// registered by multiple devices. The unique tuple below provides idempotency.
	var idBytes [12]byte
	if _, err := rand.Read(idBytes[:]); err == nil {
		id = hex.EncodeToString(idBytes[:])
	}
	now := time.Now().UTC()
	var t PushToken
	err := r.Pool.QueryRow(ctx, `INSERT INTO push_tokens(id,device_id,provider,platform,token_hash,app_version,created_at,updated_at,revoked_at) VALUES($1,$2,$3,$4,$5,$6,$7,$7,NULL) ON CONFLICT(device_id,provider,token_hash) DO UPDATE SET platform=EXCLUDED.platform,app_version=EXCLUDED.app_version,updated_at=EXCLUDED.updated_at,revoked_at=NULL RETURNING id,device_id,provider,platform,token_hash,COALESCE(app_version,''),created_at,updated_at,revoked_at`, id, req.DeviceID, req.Provider, req.Platform, h, strings.TrimSpace(req.AppVersion), now).Scan(&t.ID, &t.DeviceID, &t.Provider, &t.Platform, &t.TokenHash, &t.AppVersion, &t.CreatedAt, &t.UpdatedAt, &t.RevokedAt)
	return t, err
}
func (r *PostgresRepository) ListTokens(ctx context.Context, device string) ([]PushToken, error) {
	rows, err := r.Pool.Query(ctx, `SELECT id,device_id,provider,platform,token_hash,COALESCE(app_version,''),created_at,updated_at,revoked_at FROM push_tokens WHERE device_id=$1 AND revoked_at IS NULL ORDER BY created_at DESC`, device)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PushToken{}
	for rows.Next() {
		var t PushToken
		if err := rows.Scan(&t.ID, &t.DeviceID, &t.Provider, &t.Platform, &t.TokenHash, &t.AppVersion, &t.CreatedAt, &t.UpdatedAt, &t.RevokedAt); err != nil {
			return nil, err
		}
		t.TokenHash = ""
		out = append(out, t)
	}
	return out, rows.Err()
}
func (r *PostgresRepository) RevokeToken(ctx context.Context, device, id, raw string) error {
	if strings.TrimSpace(id) == "" && raw != "" {
		id = hashToken(raw)[:24]
	}
	result, err := r.Pool.Exec(ctx, `UPDATE push_tokens SET revoked_at=COALESCE(revoked_at,now()),updated_at=now() WHERE id=$1 AND device_id=$2 AND revoked_at IS NULL`, id, device)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrTokenNotFound
	}
	return nil
}

var _ Repository = (*PostgresRepository)(nil)
