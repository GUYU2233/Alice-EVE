package protocol

import "time"

type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

type EventEnvelope struct {
	Version        int         `json:"v"`
	MessageID      string      `json:"id"`
	ConversationID string      `json:"conversationId,omitempty"`
	Sender         string      `json:"sender"`
	Type           string      `json:"type"`
	CreatedAt      time.Time   `json:"ts"`
	Payload        interface{} `json:"payload"`
}

type IntelAlert struct {
	Title      string   `json:"title"`
	Summary    string   `json:"summary"`
	Severity   Severity `json:"severity"`
	SystemID   *int64   `json:"systemId,omitempty"`
	Confidence float64  `json:"confidence,omitempty"`
}

type DesktopStatus struct {
	Online  bool   `json:"online"`
	Version string `json:"version,omitempty"`
}

const (
	EventIntelAlert    = "intel.alert"
	EventDesktopStatus = "desktop.status"
	EventAck           = "ack"
	EventError         = "error"
)
