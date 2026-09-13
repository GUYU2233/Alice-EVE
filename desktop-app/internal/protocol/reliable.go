package protocol

import "time"

// Ack confirms durable processing of a message. Cursor is the receiver's
// monotonic sync position and is optional for older peers.
type Ack struct {
	MessageID string `json:"messageId"`
	Cursor    uint64 `json:"cursor,omitempty"`
}
type SyncRequest struct {
	Cursor uint64 `json:"cursor"`
}
type SyncResponse struct {
	Cursor uint64          `json:"cursor"`
	Events []EventEnvelope `json:"events"`
}
type RetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}

func (p RetryPolicy) Delay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := p.BaseDelay
	for i := 1; i < attempt; i++ {
		d *= 2
		if d >= p.MaxDelay {
			return p.MaxDelay
		}
	}
	if d > p.MaxDelay {
		return p.MaxDelay
	}
	return d
}
