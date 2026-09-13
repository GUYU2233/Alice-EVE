package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type PostgresStore struct{ Pool *pgxpool.Pool }

func NewPostgres(ctx context.Context, dsn string) (*PostgresStore, error) {
	p, e := pgxpool.New(ctx, dsn)
	if e != nil {
		return nil, e
	}
	return &PostgresStore{p}, p.Ping(ctx)
}
func (s *PostgresStore) Close() { s.Pool.Close() }
func (s *PostgresStore) PutMessage(c context.Context, m Message) error {
	_, e := s.Pool.Exec(c, "INSERT INTO messages(id,body) VALUES($1,$2) ON CONFLICT (id) DO NOTHING", m.ID, m.Body)
	return e
}
func (s *PostgresStore) HasMessage(c context.Context, id string) (bool, error) {
	var n int
	e := s.Pool.QueryRow(c, "SELECT 1 FROM messages WHERE id=$1", id).Scan(&n)
	if e == pgx.ErrNoRows {
		return false, nil
	}
	return e == nil, e
}
func (s *PostgresStore) Ack(c context.Context, id string) error {
	_, e := s.Pool.Exec(c, "DELETE FROM messages WHERE id=$1", id)
	return e
}
func (s *PostgresStore) AddPair(c context.Context, p PairCode) error {
	_, e := s.Pool.Exec(c, "INSERT INTO pairing_codes(code,expires_at,device_name,device_type) VALUES($1,$2,$3,$4)", p.Code, p.ExpiresAt, p.DeviceName, p.DeviceType)
	return e
}
func (s *PostgresStore) ConsumePair(c context.Context, code string) (PairCode, bool) {
	var p PairCode
	e := s.Pool.QueryRow(c, "DELETE FROM pairing_codes WHERE code=$1 AND expires_at>now() RETURNING code,expires_at,device_name,device_type", code).Scan(&p.Code, &p.ExpiresAt, &p.DeviceName, &p.DeviceType)
	return p, e == nil
}
func HashToken(t string) string { h := sha256.Sum256([]byte(t)); return hex.EncodeToString(h[:]) }
func (s *PostgresStore) RevokeDevice(id string) bool {
	_, e := s.Pool.Exec(context.Background(), "UPDATE devices SET revoked=true WHERE id=$1", id)
	return e == nil
}
func (s *PostgresStore) VerifyToken(token string) bool {
	var revoked bool
	e := s.Pool.QueryRow(context.Background(), "SELECT revoked FROM devices WHERE token_hash=$1", HashToken(token)).Scan(&revoked)
	return e == nil && !revoked
}
func (s *PostgresStore) AddDevice(d Device) error {
	_, e := s.Pool.Exec(context.Background(), "INSERT INTO devices(id,token_hash,name,type,public_key,created_at,revoked) VALUES($1,$2,$3,$4,$5,$6,$7)", d.ID, d.TokenHash, d.Name, d.Type, d.PublicKey, d.CreatedAt, d.Revoked)
	return e
}

var _ = time.Now
