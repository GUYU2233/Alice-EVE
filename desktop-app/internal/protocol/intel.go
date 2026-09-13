package protocol

import "time"

// IntelFinding is a deterministic observation extracted from user-provided text.
type IntelFinding struct {
	Kind       string    `json:"kind"`
	Value      string    `json:"value"`
	Stance     string    `json:"stance"`
	Source     string    `json:"source"`
	Summary    string    `json:"summary"`
	ObservedAt time.Time `json:"observedAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
	Confidence float64   `json:"confidence"`
}
