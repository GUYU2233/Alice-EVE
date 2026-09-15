package conversations

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"relay-server/internal/realtime"
)

// PutMessageWithEvent commits a message and its realtime intent in one
// transaction. Duplicate client IDs return the existing message and do not
// create a second intent.
func (r *PostgresRepository) PutMessageWithEvent(ctx context.Context, m Message, typ string, payload json.RawMessage) (Message, bool, error) {
	metadata, err := json.Marshal(m.Metadata)
	if err != nil {
		return Message{}, false, err
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	if m.UpdatedAt.IsZero() {
		m.UpdatedAt = m.CreatedAt
	}
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return Message{}, false, err
	}
	defer tx.Rollback(ctx)
	var accountID, target string
	if err = tx.QueryRow(ctx, `SELECT account_id,target_device_id FROM conversations WHERE id=$1`, m.ConversationID).Scan(&accountID, &target); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Message{}, false, ErrNotFound
		}
		return Message{}, false, err
	}
	if m.AccountID != "" && m.AccountID != accountID {
		return Message{}, false, ErrForbidden
	}
	var stored Message
	var raw []byte
	err = tx.QueryRow(ctx, `INSERT INTO conversation_messages(id,conversation_id,sender_kind,sender_device_id,client_message_id,operation,body,metadata,status,created_at,updated_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (conversation_id,sender_device_id,client_message_id) DO NOTHING
		RETURNING id,conversation_id,sender_kind,sender_device_id,client_message_id,operation,body,metadata,status,cursor,created_at,updated_at`, m.ID, m.ConversationID, m.SenderKind, m.SenderDeviceID, m.ClientMessageID, m.Operation, m.Body, metadata, messageStatus(m.Status), m.CreatedAt, m.UpdatedAt).Scan(&stored.ID, &stored.ConversationID, &stored.SenderKind, &stored.SenderDeviceID, &stored.ClientMessageID, &stored.Operation, &stored.Body, &raw, &stored.Status, &stored.Cursor, &stored.CreatedAt, &stored.UpdatedAt)
	duplicate := false
	if errors.Is(err, pgx.ErrNoRows) {
		duplicate = true
		err = tx.QueryRow(ctx, `SELECT id,conversation_id,sender_kind,sender_device_id,client_message_id,operation,body,metadata,status,cursor,created_at,updated_at FROM conversation_messages WHERE conversation_id=$1 AND sender_device_id=$2 AND client_message_id=$3`, m.ConversationID, m.SenderDeviceID, m.ClientMessageID).Scan(&stored.ID, &stored.ConversationID, &stored.SenderKind, &stored.SenderDeviceID, &stored.ClientMessageID, &stored.Operation, &stored.Body, &raw, &stored.Status, &stored.Cursor, &stored.CreatedAt, &stored.UpdatedAt)
	}
	if err != nil {
		return Message{}, false, err
	}
	stored.AccountID = accountID
	stored.Metadata = decodeMetadata(raw)
	if !duplicate {
		if _, err = tx.Exec(ctx, `INSERT INTO realtime_outbox(id,account_id,recipient_device_id,event_type,payload,created_at) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT (id) DO NOTHING`, "conversation.message:"+stored.ID, accountID, target, typ, []byte(payload), stored.CreatedAt); err != nil {
			return Message{}, false, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return Message{}, false, err
	}
	return stored, duplicate, nil
}

func (r *PostgresRepository) CreateRunWithEvent(ctx context.Context, run AgentRun, typ string, payload json.RawMessage) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	result, err := tx.Exec(ctx, `INSERT INTO agent_runs(id,conversation_id,input_message_id,status,started_at,completed_at,error_code) SELECT $1,$2,$3,$4,$5,$6,$7 WHERE EXISTS(SELECT 1 FROM conversations WHERE id=$2 AND account_id=$8)`, run.ID, run.ConversationID, run.InputMessageID, runStatus(run.Status), run.StartedAt, run.CompletedAt, run.ErrorCode, run.AccountID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	var target string
	if err = tx.QueryRow(ctx, `SELECT target_device_id FROM conversations WHERE id=$1 AND account_id=$2`, run.ConversationID, run.AccountID).Scan(&target); err != nil {
		return err
	}
	// Keep the transactional payload wire-compatible with the original
	// non-transactional publisher: lifecycle events carry the complete run,
	// not a partial patch supplied by the caller.
	runPayload, err := json.Marshal(run)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO realtime_outbox(id,account_id,recipient_device_id,event_type,payload,created_at) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT (id) DO NOTHING`, "run:"+run.ID+":"+typ, run.AccountID, target, typ, runPayload, run.StartedAt); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) UpdateRunWithEvent(ctx context.Context, accountID, id string, status RunStatus, code string, at time.Time, typ string, payload json.RawMessage) (AgentRun, error) {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return AgentRun{}, err
	}
	defer tx.Rollback(ctx)
	var run AgentRun
	err = tx.QueryRow(ctx, `UPDATE agent_runs r SET status=$3,error_code=$4,completed_at=CASE WHEN $3 IN ('completed','failed','cancelled','timed_out') THEN $5 ELSE completed_at END FROM conversations c WHERE r.id=$1 AND c.id=r.conversation_id AND c.account_id=$2 RETURNING r.id,c.account_id,r.conversation_id,r.input_message_id,r.status,r.started_at,r.completed_at,r.error_code`, id, accountID, status, code, at).Scan(&run.ID, &run.AccountID, &run.ConversationID, &run.InputMessageID, &run.Status, &run.StartedAt, &run.CompletedAt, &run.ErrorCode)
	if errors.Is(err, pgx.ErrNoRows) {
		return AgentRun{}, ErrNotFound
	}
	if err != nil {
		return AgentRun{}, err
	}
	var target string
	if err = tx.QueryRow(ctx, `SELECT target_device_id FROM conversations WHERE id=$1`, run.ConversationID).Scan(&target); err != nil {
		return AgentRun{}, err
	}
	// The legacy publisher sends the updated AgentRun as the event payload.
	// Preserve that shape for transactional outbox consumers as well.
	runPayload, err := json.Marshal(run)
	if err != nil {
		return AgentRun{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO realtime_outbox(id,account_id,recipient_device_id,event_type,payload,created_at) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT (id) DO NOTHING`, "run:"+run.ID+":"+typ, accountID, target, typ, runPayload, at); err != nil {
		return AgentRun{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return AgentRun{}, err
	}
	return run, nil
}

func (r *PostgresRepository) AppendRunEventWithEvent(ctx context.Context, event RunEvent, typ string, payload json.RawMessage) (RunEvent, error) {
	b, err := json.Marshal(event.Payload)
	if err != nil {
		return RunEvent{}, err
	}
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return RunEvent{}, err
	}
	defer tx.Rollback(ctx)
	var out RunEvent
	var raw []byte
	err = tx.QueryRow(ctx, `INSERT INTO conversation_events(id,conversation_id,run_id,type,payload,created_at) SELECT $1,$2,$3,$4,$5,$6 WHERE EXISTS(SELECT 1 FROM agent_runs r JOIN conversations c ON c.id=r.conversation_id WHERE r.id=$3 AND c.account_id=$7) RETURNING id,conversation_id,run_id,sequence,type,payload,created_at`, event.ID, event.ConversationID, event.RunID, event.Type, b, event.CreatedAt, event.AccountID).Scan(&out.ID, &out.ConversationID, &out.RunID, &out.Sequence, &out.Type, &raw, &out.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return RunEvent{}, ErrNotFound
	}
	if err != nil {
		return RunEvent{}, err
	}
	out.AccountID = event.AccountID
	out.Payload = json.RawMessage(raw)
	var target string
	if err = tx.QueryRow(ctx, `SELECT c.target_device_id FROM conversations c WHERE c.id=$1`, event.ConversationID).Scan(&target); err != nil {
		return RunEvent{}, err
	}
	// Preserve the legacy publisher's full RunEvent payload shape. The event
	// body includes identity, sequence and timestamps in addition to payload.
	eventPayload, err := json.Marshal(out)
	if err != nil {
		return RunEvent{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO realtime_outbox(id,account_id,recipient_device_id,event_type,payload,created_at) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT (id) DO NOTHING`, "run-event:"+event.RunID+":"+itoa(out.Sequence), event.AccountID, target, typ, eventPayload, out.CreatedAt); err != nil {
		return RunEvent{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return RunEvent{}, err
	}
	return out, nil
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	const digits = "0123456789"
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = digits[v%10]
		v /= 10
	}
	return string(b[i:])
}

// ClaimOutbox leases pending and expired processing rows using SKIP LOCKED.
func (r *PostgresRepository) ClaimOutbox(ctx context.Context, limit int, now time.Time, leaseDuration time.Duration) ([]realtime.OutboxRecord, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if leaseDuration <= 0 {
		leaseDuration = realtime.DefaultOutboxLease
	}
	lease := now.Add(leaseDuration)
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `WITH claimed AS (SELECT id FROM realtime_outbox WHERE (status='pending' AND available_at<=$1) OR (status='processing' AND available_at<=$1) OR (status='failed' AND available_at<=$1) ORDER BY created_at,id LIMIT $3 FOR UPDATE SKIP LOCKED), updated AS (UPDATE realtime_outbox o SET status='processing',attempts=o.attempts+1,claim_token=md5(random()::text || clock_timestamp()::text),available_at=$2 FROM claimed c WHERE o.id=c.id RETURNING o.id,o.account_id,o.recipient_device_id,o.event_type,o.payload,o.created_at,o.attempts,o.claim_token) SELECT id,account_id,recipient_device_id,event_type,payload,created_at,attempts,claim_token FROM updated`, now, lease, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []realtime.OutboxRecord{}
	for rows.Next() {
		var x realtime.OutboxRecord
		if err := rows.Scan(&x.ID, &x.AccountID, &x.TargetDeviceID, &x.Type, &x.Payload, &x.CreatedAt, &x.AttemptCount, &x.ClaimToken); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}
func (r *PostgresRepository) CompleteOutbox(ctx context.Context, id, claimToken string) error {
	result, err := r.Pool.Exec(ctx, `UPDATE realtime_outbox SET status='published',published_at=COALESCE(published_at,now()),last_error=NULL,claim_token=NULL WHERE id=$1 AND status='processing' AND claim_token=$2`, id, claimToken)
	if err == nil && result.RowsAffected() == 0 {
		return realtime.ErrOutboxLeaseLost
	}
	return err
}
func (r *PostgresRepository) FailOutbox(ctx context.Context, id, claimToken, lastError string, at time.Time) error {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	result, err := r.Pool.Exec(ctx, `UPDATE realtime_outbox SET status='failed',last_error=$3,available_at=$4,claim_token=NULL WHERE id=$1 AND status='processing' AND claim_token=$2`, id, claimToken, lastError, at.Add(time.Second))
	if err == nil && result.RowsAffected() == 0 {
		return realtime.ErrOutboxLeaseLost
	}
	return err
}

var _ TransactionalEventWriter = (*PostgresRepository)(nil)
var _ realtime.OutboxRepository = (*PostgresRepository)(nil)
