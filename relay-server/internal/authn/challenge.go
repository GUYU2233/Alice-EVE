package authn

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrChallengeNotFound = errors.New("challenge not found or expired")

type Challenge struct {
	Hash        string
	AccountID   string
	Provider    string
	Subject     string
	DisplayName string
	DeviceName  string
	DeviceType  string
	PublicKey   string
	ExpiresAt   time.Time
	CreatedAt   time.Time
}

type ChallengeRepository interface {
	CreateChallenge(context.Context, Challenge) error
	ConsumeChallenge(context.Context, string, time.Time) (Challenge, error)
}

type MemoryChallengeRepository struct {
	mu     sync.Mutex
	values map[string]Challenge
}

// NewMemoryChallengeRepository returns a concurrency-safe one-shot repository.
func NewMemoryChallengeRepository() *MemoryChallengeRepository {
	return &MemoryChallengeRepository{values: make(map[string]Challenge)}
}
func (r *MemoryChallengeRepository) CreateChallenge(_ context.Context, c Challenge) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.values[c.Hash] = c
	return nil
}
func (r *MemoryChallengeRepository) ConsumeChallenge(_ context.Context, hash string, now time.Time) (Challenge, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.values[hash]
	if ok {
		delete(r.values, hash)
	}
	if !ok || !now.Before(c.ExpiresAt) {
		return Challenge{}, ErrChallengeNotFound
	}
	return c, nil
}

// PostgresChallengeRepository atomically consumes a challenge with UPDATE ... RETURNING.
type PostgresChallengeRepository struct{ Pool *pgxpool.Pool }

func NewPostgresChallengeRepository(pool *pgxpool.Pool) (*PostgresChallengeRepository, error) {
	if pool == nil {
		return nil, errors.New("authn: postgres pool is required")
	}
	return &PostgresChallengeRepository{Pool: pool}, nil
}
func (r *PostgresChallengeRepository) CreateChallenge(ctx context.Context, c Challenge) error {
	_, err := r.Pool.Exec(ctx, `INSERT INTO device_auth_challenges(challenge_hash,account_id,provider,subject,display_name,device_name,device_type,public_key,expires_at,created_at) VALUES($1,NULLIF($2,''),$3,$4,$5,$6,$7,$8,$9,$10)`, c.Hash, c.AccountID, c.Provider, c.Subject, c.DisplayName, c.DeviceName, c.DeviceType, c.PublicKey, c.ExpiresAt, c.CreatedAt)
	return err
}
func (r *PostgresChallengeRepository) ConsumeChallenge(ctx context.Context, hash string, now time.Time) (Challenge, error) {
	var c Challenge
	err := r.Pool.QueryRow(ctx, `UPDATE device_auth_challenges SET consumed_at=$2 WHERE challenge_hash=$1 AND consumed_at IS NULL AND expires_at>$2 RETURNING challenge_hash,COALESCE(account_id::text,''),COALESCE(provider,''),COALESCE(subject,''),COALESCE(display_name,''),device_name,device_type,COALESCE(public_key,''),expires_at,created_at`, hash, now).Scan(&c.Hash, &c.AccountID, &c.Provider, &c.Subject, &c.DisplayName, &c.DeviceName, &c.DeviceType, &c.PublicKey, &c.ExpiresAt, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Challenge{}, ErrChallengeNotFound
	}
	return c, err
}

var _ ChallengeRepository = (*MemoryChallengeRepository)(nil)
var _ ChallengeRepository = (*PostgresChallengeRepository)(nil)
