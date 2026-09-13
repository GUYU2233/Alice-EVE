package risk

import "time"

// IntelEvent is a normalized intelligence observation.
type IntelEvent struct {
	ID string `json:"id"`
	Source string `json:"source"`
	Confidence float64 `json:"confidence"`
	ObservedAt time.Time `json:"observed_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Threat string `json:"threat"`
}

func (e IntelEvent) ValidAt(now time.Time) bool {
	return e.Source != "" && e.Confidence >= 0 && e.Confidence <= 1 && !e.ExpiresAt.Before(now)
}

// Score returns a stable 0..100 score. Confidence scales threat severity;
// expired or malformed events contribute zero.
func Score(events []IntelEvent, now time.Time) int {
	total := 0.0
	for _, e := range events {
		if !e.ValidAt(now) { continue }
		severity := 0.0
		switch e.Threat { case "critical": severity=100; case "high": severity=75; case "medium": severity=50; case "low": severity=25 }
		total += severity * e.Confidence
	}
	if total > 100 { total = 100 }
	return int(total + 0.5)
}
