package conversations

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"time"
)

func (r *PostgresRepository) CreateRun(ctx context.Context, v AgentRun) error {
	result, e := r.Pool.Exec(ctx, `INSERT INTO agent_runs(id,conversation_id,input_message_id,status,started_at,completed_at,error_code) SELECT $1,$2,$3,$4,$5,$6,$7 WHERE EXISTS(SELECT 1 FROM conversations WHERE id=$2 AND account_id=$8)`, v.ID, v.ConversationID, v.InputMessageID, runStatus(v.Status), v.StartedAt, v.CompletedAt, v.ErrorCode, v.AccountID)
	if e == nil && result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return e
}
func (r *PostgresRepository) ListRuns(ctx context.Context, account, conversationID string, limit int) ([]AgentRun, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	rows, err := r.Pool.Query(ctx, `SELECT r.id,c.account_id,r.conversation_id,r.input_message_id,r.status,r.started_at,r.completed_at,r.error_code FROM agent_runs r JOIN conversations c ON c.id=r.conversation_id WHERE c.account_id=$1 AND r.conversation_id=$2 ORDER BY r.started_at ASC,r.id ASC LIMIT $3`, account, conversationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AgentRun, 0, limit)
	for rows.Next() {
		var v AgentRun
		if err := rows.Scan(&v.ID, &v.AccountID, &v.ConversationID, &v.InputMessageID, &v.Status, &v.StartedAt, &v.CompletedAt, &v.ErrorCode); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) GetRun(ctx context.Context, a, id string) (AgentRun, error) {
	var v AgentRun
	e := r.Pool.QueryRow(ctx, `SELECT r.id,c.account_id,r.conversation_id,r.input_message_id,r.status,r.started_at,r.completed_at,r.error_code FROM agent_runs r JOIN conversations c ON c.id=r.conversation_id WHERE c.account_id=$1 AND r.id=$2`, a, id).Scan(&v.ID, &v.AccountID, &v.ConversationID, &v.InputMessageID, &v.Status, &v.StartedAt, &v.CompletedAt, &v.ErrorCode)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrNotFound
	}
	return v, e
}
func (r *PostgresRepository) UpdateRun(ctx context.Context, a, id string, status RunStatus, code string, at time.Time) (AgentRun, error) {
	var v AgentRun
	e := r.Pool.QueryRow(ctx, `UPDATE agent_runs r SET status=$3,error_code=$4,completed_at=CASE WHEN $3 IN ('completed','failed','cancelled','timed_out') THEN $5 ELSE completed_at END FROM conversations c WHERE r.id=$1 AND c.id=r.conversation_id AND c.account_id=$2 RETURNING r.id,c.account_id,r.conversation_id,r.input_message_id,r.status,r.started_at,r.completed_at,r.error_code`, id, a, status, code, at).Scan(&v.ID, &v.AccountID, &v.ConversationID, &v.InputMessageID, &v.Status, &v.StartedAt, &v.CompletedAt, &v.ErrorCode)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrNotFound
	}
	return v, e
}
func (r *PostgresRepository) AppendRunEvent(ctx context.Context, v RunEvent) (RunEvent, error) {
	b, e := json.Marshal(v.Payload)
	if e != nil {
		return RunEvent{}, e
	}
	e = r.Pool.QueryRow(ctx, `INSERT INTO conversation_events(id,conversation_id,run_id,type,payload,created_at) SELECT $1,$2,$3,$4,$5,$6 WHERE EXISTS(SELECT 1 FROM agent_runs r JOIN conversations c ON c.id=r.conversation_id WHERE r.id=$3 AND c.account_id=$7) RETURNING id,conversation_id,run_id,sequence,type,payload,created_at`, v.ID, v.ConversationID, v.RunID, v.Type, b, v.CreatedAt, v.AccountID).Scan(&v.ID, &v.ConversationID, &v.RunID, &v.Sequence, &v.Type, &b, &v.CreatedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrNotFound
	}
	v.Payload = json.RawMessage(b)
	return v, e
}
func (r *PostgresRepository) ListRunEvents(ctx context.Context, a, runID string, after int64, limit int) ([]RunEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	rows, e := r.Pool.Query(ctx, `SELECT e.id,c.account_id,e.conversation_id,e.run_id,e.sequence,e.type,e.payload,e.created_at FROM conversation_events e JOIN conversations c ON c.id=e.conversation_id WHERE c.account_id=$1 AND e.run_id=$2 AND e.sequence>$3 ORDER BY e.sequence LIMIT $4`, a, runID, after, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []RunEvent{}
	for rows.Next() {
		var v RunEvent
		var b []byte
		if e = rows.Scan(&v.ID, &v.AccountID, &v.ConversationID, &v.RunID, &v.Sequence, &v.Type, &b, &v.CreatedAt); e != nil {
			return nil, e
		}
		v.Payload = json.RawMessage(b)
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *PostgresRepository) AppendAudit(ctx context.Context, a AuditRecord) error {
	_, e := r.Pool.Exec(ctx, `INSERT INTO conversation_audit(id,account_id,device_id,request_id,type,status,size,duration_ms,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, a.ID, a.AccountID, a.DeviceID, a.RequestID, a.Type, a.Status, a.Size, a.DurationMillis, a.CreatedAt)
	return e
}
func (r *PostgresRepository) ListAudit(ctx context.Context, a, typ string, limit int) ([]AuditRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	rows, e := r.Pool.Query(ctx, `SELECT id,account_id,device_id,request_id,type,status,size,duration_ms,created_at FROM conversation_audit WHERE account_id=$1 AND ($2='' OR type=$2) ORDER BY created_at DESC LIMIT $3`, a, typ, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []AuditRecord{}
	for rows.Next() {
		var v AuditRecord
		if e = rows.Scan(&v.ID, &v.AccountID, &v.DeviceID, &v.RequestID, &v.Type, &v.Status, &v.Size, &v.DurationMillis, &v.CreatedAt); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func runStatus(s RunStatus) RunStatus {
	if s == "" {
		return RunQueued
	}
	return s
}

var _ ConversationRepository = (*PostgresRepository)(nil)
