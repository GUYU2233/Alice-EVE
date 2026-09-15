package conversations

import (
	"context"
	"encoding/json"
	"sort"
	"time"
)

func (r *MemoryRepository) CreateRun(_ context.Context, run AgentRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if run.ID == "" || run.AccountID == "" || run.ConversationID == "" {
		return ErrInvalid
	}
	c, ok := r.conversations[run.ConversationID]
	if !ok || c.AccountID != run.AccountID {
		return ErrNotFound
	}
	if _, ok := r.runs[run.ID]; ok {
		return ErrConflict
	}
	if run.Status == "" {
		run.Status = RunQueued
	}
	r.runs[run.ID] = run
	return nil
}
func (r *MemoryRepository) ListRuns(_ context.Context, account, conversationID string, limit int) ([]AgentRun, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	out := make([]AgentRun, 0, limit)
	for _, run := range r.runs {
		if run.AccountID == account && run.ConversationID == conversationID {
			out = append(out, run)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].StartedAt.Before(out[j].StartedAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *MemoryRepository) GetRun(_ context.Context, account, id string) (AgentRun, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	run, ok := r.runs[id]
	if !ok || run.AccountID != account {
		return AgentRun{}, ErrNotFound
	}
	return run, nil
}
func (r *MemoryRepository) UpdateRun(_ context.Context, account, id string, status RunStatus, code string, at time.Time) (AgentRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run, ok := r.runs[id]
	if !ok || run.AccountID != account {
		return AgentRun{}, ErrNotFound
	}
	if !validRunStatus(status) {
		return AgentRun{}, ErrInvalid
	}
	if !runTransition(run.Status, status) {
		return AgentRun{}, ErrInvalidStatus
	}
	run.Status = status
	run.ErrorCode = code
	if at.IsZero() {
		at = time.Now().UTC()
	}
	if status == RunCompleted || status == RunFailed || status == RunCancelled || status == RunTimedOut {
		run.CompletedAt = &at
	}
	r.runs[id] = run
	return run, nil
}
func (r *MemoryRepository) AppendRunEvent(_ context.Context, e RunEvent) (RunEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run, ok := r.runs[e.RunID]
	if !ok || run.AccountID != e.AccountID || run.ConversationID != e.ConversationID {
		return RunEvent{}, ErrNotFound
	}
	key := e.RunID
	r.runSeq[key]++
	e.Sequence = r.runSeq[key]
	if e.ID == "" {
		e.ID = NewID()
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	e.Payload = append(json.RawMessage(nil), e.Payload...)
	r.runEvents[key] = append(r.runEvents[key], e)
	return e, nil
}
func (r *MemoryRepository) ListRunEvents(_ context.Context, account, runID string, after int64, limit int) ([]RunEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	run, ok := r.runs[runID]
	if !ok || run.AccountID != account {
		return nil, ErrNotFound
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	out := []RunEvent{}
	for _, e := range r.runEvents[runID] {
		if e.Sequence > after {
			e.Payload = append(json.RawMessage(nil), e.Payload...)
			out = append(out, e)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}
func (r *MemoryRepository) AppendAudit(_ context.Context, a AuditRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a.AccountID == "" || a.Type == "" {
		return ErrInvalid
	}
	if a.ID == "" {
		a.ID = NewID()
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	r.audit[a.AccountID] = append(r.audit[a.AccountID], a)
	return nil
}
func (r *MemoryRepository) ListAudit(_ context.Context, account, typ string, limit int) ([]AuditRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if limit <= 0 {
		limit = 100
	}
	out := []AuditRecord{}
	for _, a := range r.audit[account] {
		if typ == "" || a.Type == typ {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
func validRunStatus(s RunStatus) bool {
	switch s {
	case RunQueued, RunRunning, RunCompleted, RunFailed, RunCancelled, RunTimedOut:
		return true
	}
	return false
}
func runTransition(from, to RunStatus) bool {
	if from == to {
		return true
	}
	switch from {
	case RunQueued:
		return to == RunRunning || to == RunCancelled || to == RunTimedOut
	case RunRunning:
		return to == RunCompleted || to == RunFailed || to == RunCancelled || to == RunTimedOut
	}
	return false
}

var _ ConversationRepository = (*MemoryRepository)(nil)
