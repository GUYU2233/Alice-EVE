package protocol

import "time"

// Agent protocol messages are exchanged logically between a paired mobile
// device and the desktop. The server transports these messages, but the Agent
// itself always executes in the desktop process.
const (
	AgentProtocolVersion = 1
	AgentRequestType     = "agent.request"
	AgentResponseType    = "agent.response"

	OperationGetDesktopStatus = "get_desktop_status"
	OperationShowNotification = "show_notification"
	OperationNotify           = "notify" // backwards-compatible alias
	OperationOpenWorkspace    = "open_workspace"
)

// AgentRequest is the untrusted request accepted by the local desktop Agent.
// SessionID is an opaque, short-lived local session credential and must never
// be treated as a device or account identity.
type AgentRequest struct {
	Version        int                    `json:"v"`
	MessageID      string                 `json:"id"`
	SessionID      string                 `json:"sessionId"`
	ConversationID string                 `json:"conversationId,omitempty"`
	Operation      string                 `json:"operation"`
	Body           string                 `json:"body,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// AgentError is intentionally a small public error shape. Internal paths,
// command lines, tokens and stack traces must not cross this boundary.
type AgentError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// NotificationRequest is deliberately a presentation-only payload. It does
// not contain a URL, command, executable or arbitrary platform instruction.
type NotificationRequest struct {
	Title    string   `json:"title"`
	Body     string   `json:"body"`
	Severity Severity `json:"severity,omitempty"`
}

type WorkspaceRequest struct {
	Path string `json:"path"`
}

type WorkspaceResult struct {
	Path string `json:"path"`
}

// AgentResponse is safe to serialize back over the relay/mobile protocol.
type AgentResponse struct {
	Version        int         `json:"v"`
	MessageID      string      `json:"id"`
	ConversationID string      `json:"conversationId,omitempty"`
	Type           string      `json:"type"`
	CreatedAt      time.Time   `json:"ts"`
	OK             bool        `json:"ok"`
	Operation      string      `json:"operation,omitempty"`
	Payload        interface{} `json:"payload,omitempty"`
	Error          *AgentError `json:"error,omitempty"`
}
