package conversations

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresRepository is the production repository seam for conversations. It
// intentionally accepts a pgx pool only at the infrastructure boundary; the
// Service and ConversationRepository interfaces remain database agnostic.
type PostgresRepository struct {
	Pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) (*PostgresRepository, error) {
	if pool == nil {
		return nil, errors.New("conversations: postgres pool is required")
	}
	return &PostgresRepository{Pool: pool}, nil
}

func (r *PostgresRepository) CreateConversation(ctx context.Context, c Conversation) error {
	_, err := r.Pool.Exec(ctx, `INSERT INTO conversations(id,account_id,target_device_id,agent_kind,status,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, c.ID, c.AccountID, c.TargetDeviceID, c.AgentKind, conversationStatus(c.Status), c.CreatedAt, c.UpdatedAt)
	return err
}

func (r *PostgresRepository) GetConversation(ctx context.Context, accountID, id string) (Conversation, error) {
	var c Conversation
	err := r.Pool.QueryRow(ctx, `SELECT id,account_id,target_device_id,agent_kind,status,created_at,updated_at FROM conversations WHERE account_id=$1 AND id=$2`, accountID, id).Scan(&c.ID, &c.AccountID, &c.TargetDeviceID, &c.AgentKind, &c.Status, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Conversation{}, ErrNotFound
	}
	return c, err
}

func (r *PostgresRepository) ListConversations(ctx context.Context, accountID string, limit int) ([]Conversation, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	rows, err := r.Pool.Query(ctx, `SELECT id,account_id,target_device_id,agent_kind,status,created_at,updated_at FROM conversations WHERE account_id=$1 ORDER BY updated_at DESC LIMIT $2`, accountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Conversation, 0, limit)
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.ID, &c.AccountID, &c.TargetDeviceID, &c.AgentKind, &c.Status, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) CloseConversation(ctx context.Context, accountID, id string, status ConversationStatus) error {
	result, err := r.Pool.Exec(ctx, `UPDATE conversations SET status=$3,updated_at=now() WHERE account_id=$1 AND id=$2`, accountID, id, status)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) PutMessage(ctx context.Context, m Message) (Message, bool, error) {
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
	var accountID string
	if err := r.Pool.QueryRow(ctx, `SELECT account_id FROM conversations WHERE id=$1`, m.ConversationID).Scan(&accountID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Message{}, false, ErrNotFound
		}
		return Message{}, false, err
	}
	if m.AccountID != "" && m.AccountID != accountID {
		return Message{}, false, ErrForbidden
	}
	var stored Message
	var rawMetadata []byte
	err = r.Pool.QueryRow(ctx, `
		INSERT INTO conversation_messages(id,conversation_id,sender_kind,sender_device_id,client_message_id,operation,body,metadata,status,created_at,updated_at)
		SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11
		WHERE EXISTS (SELECT 1 FROM conversations WHERE id=$2 AND account_id=$12)
		ON CONFLICT (conversation_id,sender_device_id,client_message_id) DO NOTHING
		RETURNING id,conversation_id,sender_kind,sender_device_id,client_message_id,operation,body,metadata,status,cursor,created_at,updated_at`, m.ID, m.ConversationID, m.SenderKind, m.SenderDeviceID, m.ClientMessageID, m.Operation, m.Body, metadata, messageStatus(m.Status), m.CreatedAt, m.UpdatedAt, accountID).Scan(&stored.ID, &stored.ConversationID, &stored.SenderKind, &stored.SenderDeviceID, &stored.ClientMessageID, &stored.Operation, &stored.Body, &rawMetadata, &stored.Status, &stored.Cursor, &stored.CreatedAt, &stored.UpdatedAt)
	if err == nil {
		stored.Metadata = decodeMetadata(rawMetadata)
		return stored, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Message{}, false, err
	}
	// INSERT ... RETURNING yields no row for both a duplicate and a missing
	// conversation. Read the idempotency key first; a missing key is a useful
	// not-found result for the service adapter.
	err = r.Pool.QueryRow(ctx, `SELECT id,conversation_id,sender_kind,sender_device_id,client_message_id,operation,body,metadata,status,cursor,created_at,updated_at FROM conversation_messages WHERE conversation_id=$1 AND sender_device_id=$2 AND client_message_id=$3`, m.ConversationID, m.SenderDeviceID, m.ClientMessageID).Scan(&stored.ID, &stored.ConversationID, &stored.SenderKind, &stored.SenderDeviceID, &stored.ClientMessageID, &stored.Operation, &stored.Body, &rawMetadata, &stored.Status, &stored.Cursor, &stored.CreatedAt, &stored.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Message{}, false, ErrNotFound
	}
	if err != nil {
		return Message{}, false, err
	}
	stored.Metadata = decodeMetadata(rawMetadata)
	return stored, true, nil
}

func (r *PostgresRepository) GetMessage(ctx context.Context, accountID, id string) (Message, error) {
	var m Message
	var rawMetadata []byte
	err := r.Pool.QueryRow(ctx, `SELECT m.id,m.conversation_id,m.sender_kind,m.sender_device_id,m.client_message_id,m.operation,m.body,m.metadata,m.status,m.cursor,m.created_at,m.updated_at FROM conversation_messages m JOIN conversations c ON c.id=m.conversation_id WHERE c.account_id=$1 AND m.id=$2`, accountID, id).Scan(&m.ID, &m.ConversationID, &m.SenderKind, &m.SenderDeviceID, &m.ClientMessageID, &m.Operation, &m.Body, &rawMetadata, &m.Status, &m.Cursor, &m.CreatedAt, &m.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Message{}, ErrNotFound
	}
	m.Metadata = decodeMetadata(rawMetadata)
	return m, err
}

func (r *PostgresRepository) SnapshotCursor(ctx context.Context, accountID string) (int64, error) {
	var cursor int64
	err := r.Pool.QueryRow(ctx, `SELECT COALESCE(MAX(m.cursor),0) FROM conversation_messages m JOIN conversations c ON c.id=m.conversation_id WHERE c.account_id=$1`, accountID).Scan(&cursor)
	return cursor, err
}

func (r *PostgresRepository) ListMessages(ctx context.Context, accountID, conversationID string, after int64, limit int) (MessagePage, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	rows, err := r.Pool.Query(ctx, `SELECT m.id,m.conversation_id,m.sender_kind,m.sender_device_id,m.client_message_id,m.operation,m.body,m.metadata,m.status,m.cursor,m.created_at,m.updated_at FROM conversation_messages m JOIN conversations c ON c.id=m.conversation_id WHERE c.account_id=$1 AND m.conversation_id=$2 AND m.cursor>$3 ORDER BY m.cursor LIMIT $4`, accountID, conversationID, after, limit+1)
	if err != nil {
		return MessagePage{}, err
	}
	defer rows.Close()
	items := make([]Message, 0, limit)
	for rows.Next() {
		var m Message
		var rawMetadata []byte
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.SenderKind, &m.SenderDeviceID, &m.ClientMessageID, &m.Operation, &m.Body, &rawMetadata, &m.Status, &m.Cursor, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return MessagePage{}, err
		}
		m.Metadata = decodeMetadata(rawMetadata)
		items = append(items, m)
	}
	if err := rows.Err(); err != nil {
		return MessagePage{}, err
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	next := after
	if len(items) > 0 {
		next = items[len(items)-1].Cursor
	}
	return MessagePage{Messages: items, NextCursor: next, HasMore: hasMore}, nil
}

func (r *PostgresRepository) UpdateMessageStatus(ctx context.Context, accountID, id string, status MessageStatus, at time.Time) (Message, error) {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	var m Message
	var rawMetadata []byte
	err := r.Pool.QueryRow(ctx, `
		UPDATE conversation_messages m SET status=$3,updated_at=$4
		FROM conversations c
		WHERE m.id=$1 AND c.id=m.conversation_id AND c.account_id=$2
		AND CASE m.status
			WHEN 'accepted' THEN CASE $3 WHEN 'accepted' THEN true WHEN 'persisted' THEN true WHEN 'delivered' THEN true WHEN 'processed' THEN true WHEN 'failed' THEN true ELSE false END
			WHEN 'persisted' THEN CASE $3 WHEN 'persisted' THEN true WHEN 'delivered' THEN true WHEN 'processed' THEN true WHEN 'failed' THEN true ELSE false END
			WHEN 'delivered' THEN CASE $3 WHEN 'delivered' THEN true WHEN 'processed' THEN true ELSE false END
			WHEN 'processed' THEN $3='processed'
			WHEN 'failed' THEN $3='failed'
			ELSE false END
		RETURNING m.id,m.conversation_id,m.sender_kind,m.sender_device_id,m.client_message_id,m.operation,m.body,m.metadata,m.status,m.cursor,m.created_at,m.updated_at`, id, accountID, status, at).Scan(&m.ID, &m.ConversationID, &m.SenderKind, &m.SenderDeviceID, &m.ClientMessageID, &m.Operation, &m.Body, &rawMetadata, &m.Status, &m.Cursor, &m.CreatedAt, &m.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Message{}, ErrNotFound
	}
	m.Metadata = decodeMetadata(rawMetadata)
	return m, err
}

func conversationStatus(status ConversationStatus) ConversationStatus {
	if status == "" {
		return ConversationActive
	}
	return status
}

func messageStatus(status MessageStatus) MessageStatus {
	if status == "" {
		return MessagePersisted
	}
	return status
}

func decodeMetadata(raw []byte) map[string]any {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var metadata map[string]any
	if json.Unmarshal(raw, &metadata) != nil {
		return nil
	}
	return metadata
}

var _ ConversationRepository = (*PostgresRepository)(nil)
