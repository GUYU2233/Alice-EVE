package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct{ Pool *pgxpool.Pool }

func NewPostgres(ctx context.Context, dsn string) (*PostgresStore, error) {
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil {
		return nil, e
	}
	// Keep spare connections for authentication and short control-plane reads
	// while market planning performs CPU/SQL-heavy snapshot calculations.
	if cfg.MaxConns < 120 {
		cfg.MaxConns = 120
	}
	if cfg.MinConns < 4 {
		cfg.MinConns = 4
	}
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.HealthCheckPeriod = 30 * time.Second
	p, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		return nil, e
	}
	if e = p.Ping(ctx); e != nil {
		p.Close()
		return nil, e
	}
	return &PostgresStore{p}, nil
}
func nullableOwner(id string) any {
	if id == "" {
		return nil
	}
	return id
}
func (s *PostgresStore) Close() { s.Pool.Close() }
func (s *PostgresStore) PutMessage(c context.Context, m Message) error {
	_, e := s.Pool.Exec(c, `INSERT INTO messages(id,account_id,body,owner_device_id) VALUES($1,$2,$3,$4) ON CONFLICT (account_id,id) DO NOTHING`, m.ID, m.AccountID, m.Body, nullableOwner(m.OwnerDeviceID))
	return e
}
func (s *PostgresStore) HasMessage(c context.Context, accountID, id string) (bool, error) {
	var n int
	e := s.Pool.QueryRow(c, "SELECT 1 FROM messages WHERE account_id=$1 AND id=$2", accountID, id).Scan(&n)
	if e == pgx.ErrNoRows {
		return false, nil
	}
	return e == nil, e
}
func (s *PostgresStore) Ack(c context.Context, id string) error {
	_, e := s.Pool.Exec(c, "DELETE FROM messages WHERE id=$1", id)
	return e
}
func (s *PostgresStore) AckMessage(c context.Context, accountID, id, owner string) (bool, error) {
	var n int
	e := s.Pool.QueryRow(c, `UPDATE messages SET acknowledged_at=COALESCE(acknowledged_at, now()) WHERE account_id=$1 AND id=$2 AND owner_device_id=$3 AND acknowledged_at IS NULL RETURNING 1`, accountID, id, owner).Scan(&n)
	if e == pgx.ErrNoRows {
		return false, nil
	}
	return e == nil, e
}
func (s *PostgresStore) ListMessages(c context.Context, accountID, owner string, after int64, limit int) ([]Message, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, e := s.Pool.Query(c, `SELECT id, account_id, body, owner_device_id, cursor, acknowledged_at FROM messages WHERE account_id=$1 AND owner_device_id=$2 AND acknowledged_at IS NULL AND cursor>$3 ORDER BY cursor LIMIT $4`, accountID, owner, after, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := make([]Message, 0, limit)
	for rows.Next() {
		var m Message
		if e := rows.Scan(&m.ID, &m.AccountID, &m.Body, &m.OwnerDeviceID, &m.Cursor, &m.AcknowledgedAt); e != nil {
			return nil, e
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
func HashToken(t string) string { h := sha256.Sum256([]byte(t)); return hex.EncodeToString(h[:]) }
func (s *PostgresStore) DeviceIDForToken(token string) (string, bool) {
	var id string
	e := s.Pool.QueryRow(context.Background(), "SELECT id FROM devices WHERE token_hash=$1 AND revoked=false", HashToken(token)).Scan(&id)
	return id, e == nil
}
func (s *PostgresStore) RevokeDeviceForAccount(accountID, id string) bool {
	_, e := s.Pool.Exec(context.Background(), "UPDATE devices SET revoked=true WHERE id=$1 AND account_id=$2 AND revoked=false", id, accountID)
	return e == nil
}
func (s *PostgresStore) RevokeDevice(id string) bool {
	_, e := s.Pool.Exec(context.Background(), "UPDATE devices SET revoked=true WHERE id=$1", id)
	return e == nil
}
func (s *PostgresStore) VerifyTokenType(token, typ string) bool {
	var revoked bool
	e := s.Pool.QueryRow(context.Background(), "SELECT revoked FROM devices WHERE token_hash=$1 AND type=$2", HashToken(token), typ).Scan(&revoked)
	return e == nil && !revoked
}
func (s *PostgresStore) OwnsToken(token, id string) bool {
	var revoked bool
	e := s.Pool.QueryRow(context.Background(), "SELECT revoked FROM devices WHERE id=$1 AND token_hash=$2", id, HashToken(token)).Scan(&revoked)
	return e == nil && !revoked
}
func (s *PostgresStore) DeviceType(id string) (string, bool) {
	var typ string
	e := s.Pool.QueryRow(context.Background(), "SELECT type FROM devices WHERE id=$1 AND revoked=false", id).Scan(&typ)
	return typ, e == nil
}
func (s *PostgresStore) DeviceAccountID(id string) (string, bool) {
	var accountID string
	e := s.Pool.QueryRow(context.Background(), "SELECT account_id FROM devices WHERE id=$1 AND revoked=false AND account_id IS NOT NULL", id).Scan(&accountID)
	return accountID, e == nil
}
func (s *PostgresStore) DeviceExists(id string) bool {
	var n int
	e := s.Pool.QueryRow(context.Background(), "SELECT 1 FROM devices WHERE id=$1 AND revoked=false", id).Scan(&n)
	return e == nil
}
func (s *PostgresStore) VerifyToken(token string) bool { _, ok := s.DeviceIDForToken(token); return ok }
func (s *PostgresStore) GetDevice(id string) (Device, bool) {
	var d Device
	err := s.Pool.QueryRow(context.Background(), `SELECT id, account_id, name, type, created_at, revoked FROM devices WHERE id=$1 AND revoked=false`, id).Scan(&d.ID, &d.AccountID, &d.Name, &d.Type, &d.CreatedAt, &d.Revoked)
	return d, err == nil
}
func (s *PostgresStore) GetDeviceForAccount(accountID, id string) (Device, bool) {
	var d Device
	err := s.Pool.QueryRow(context.Background(), `SELECT id, account_id, name, type, created_at, revoked FROM devices WHERE id=$1 AND account_id=$2 AND revoked=false`, id, accountID).Scan(&d.ID, &d.AccountID, &d.Name, &d.Type, &d.CreatedAt, &d.Revoked)
	return d, err == nil
}

func (s *PostgresStore) ListDevices(accountID string) []Device {
	rows, err := s.Pool.Query(context.Background(), `SELECT id, account_id, name, type, created_at, revoked FROM devices WHERE revoked=false AND ($1='' OR account_id=$1) ORDER BY created_at DESC`, accountID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := make([]Device, 0)
	for rows.Next() {
		var d Device
		if rows.Scan(&d.ID, &d.AccountID, &d.Name, &d.Type, &d.CreatedAt, &d.Revoked) == nil {
			out = append(out, d)
		}
	}
	return out
}

func (s *PostgresStore) AddDevice(d Device) error {
	accountID := d.AccountID
	if accountID == "" {
		accountID = "legacy"
	}
	_, e := s.Pool.Exec(context.Background(), `INSERT INTO devices(id,account_id,token_hash,name,type,public_key,created_at,revoked) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, d.ID, accountID, d.TokenHash, d.Name, d.Type, d.PublicKey, d.CreatedAt, d.Revoked)
	return e
}
func (s *PostgresStore) AddPair(c context.Context, p PairCode) error {
	_, e := s.Pool.Exec(c, `INSERT INTO pairing_codes(code,expires_at,device_name,device_type) VALUES($1,$2,$3,$4)`, p.Code, p.ExpiresAt, p.DeviceName, p.DeviceType)
	return e
}
func (s *PostgresStore) ConsumePair(c context.Context, code string) (PairCode, bool) {
	var p PairCode
	e := s.Pool.QueryRow(c, `DELETE FROM pairing_codes WHERE code=$1 AND expires_at>now() RETURNING code,expires_at,device_name,device_type`, code).Scan(&p.Code, &p.ExpiresAt, &p.DeviceName, &p.DeviceType)
	return p, e == nil
}

var _ = time.Now
