package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"eve-assistant/desktop-app/internal/protocol"
	"fmt"
	"net/http"
	"time"
)

type RelaySender struct {
	Store  Store
	URL    string
	Client *http.Client
	Policy protocol.RetryPolicy
}

// SendPending delivers due messages; only a valid ACK causes deletion.
func (r RelaySender) SendPending(ctx context.Context, limit int) error {
	if r.Client == nil {
		r.Client = &http.Client{Timeout: 10 * time.Second}
	}
	if r.Policy.BaseDelay <= 0 {
		r.Policy.BaseDelay = time.Second
	}
	if r.Policy.MaxDelay <= 0 {
		r.Policy.MaxDelay = time.Minute
	}
	items, err := Pending(ctx, r.Store, limit)
	if err != nil {
		return err
	}
	for _, item := range items {
		body, _ := json.Marshal(item.Envelope)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.URL, bytes.NewReader(body))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
			var resp *http.Response
			resp, err = r.Client.Do(req)
			if err == nil {
				var raw struct {
					MessageID string `json:"messageId"`
					ID        string `json:"id"`
				}
				err = json.NewDecoder(resp.Body).Decode(&raw)
				resp.Body.Close()
				ackID := raw.MessageID
				if ackID == "" {
					ackID = raw.ID
				}
				if resp.StatusCode < 200 || resp.StatusCode >= 300 || ackID != item.ID {
					err = fmt.Errorf("relay did not ACK %s", item.ID)
				}
			}
		}
		if err == nil {
			if err = Ack(ctx, r.Store, item.ID); err != nil {
				return err
			}
		} else {
			attempt := item.Attempts + 1
			if r.Policy.MaxAttempts <= 0 || attempt <= r.Policy.MaxAttempts {
				if e := ScheduleRetry(ctx, r.Store, item.ID, attempt, r.Policy.Delay(attempt)); e != nil {
					return e
				}
			}
		}
	}
	return nil
}
