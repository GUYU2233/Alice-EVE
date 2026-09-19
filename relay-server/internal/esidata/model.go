// Package esidata provides persistence for normalized ESI data independently
// of transport and ESI client concerns.
package esidata

import (
	"encoding/json"
	"time"
)

// CharacterSnapshot is the latest normalized payload for one character domain.
type CharacterSnapshot struct {
	AccountID   string
	CharacterID int64
	Domain      string
	Payload     json.RawMessage
	FetchedAt   time.Time
	ExpiresAt   time.Time
	Stale       bool
	Source      string
	ETag        string
}

// PublicData is a non-account-scoped cache entry for public ESI data.
type PublicData struct {
	Kind      string
	CacheKey  string
	Payload   json.RawMessage
	FetchedAt time.Time
	ExpiresAt time.Time
	ETag      string
}
