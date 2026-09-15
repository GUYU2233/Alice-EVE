package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"eve-assistant/desktop-app/internal/protocol"
)

var (
	ErrAgentSessionRequired = errors.New("agent: valid local session is required")
	ErrAgentOperationDenied = errors.New("agent: operation is not allowed")
	ErrAgentRequestInvalid  = errors.New("agent: invalid request")
	ErrWorkspaceDenied      = errors.New("agent: workspace path is not allowed")
)

// DesktopController is the deliberately small capability boundary used by the
// local Agent. Implementations must not execute shell commands or accept an
// arbitrary executable/URL from a request.
type DesktopController interface {
	Status(context.Context) protocol.DesktopStatus
	Notify(context.Context, protocol.NotificationRequest) error
	OpenWorkspace(context.Context, string) (protocol.WorkspaceResult, error)
}

// LocalDesktopController provides safe, host-independent desktop operations.
// UI code can observe Notifications and OpenWorkspace results and perform the
// actual presentation. No operation invokes a shell or starts a process.
type LocalDesktopController struct {
	mu            sync.RWMutex
	version       string
	workspaceRoot string
	notifications []protocol.NotificationRequest
	lastWorkspace string
}

func NewLocalDesktopController(version, workspaceRoot string) *LocalDesktopController {
	// Resolve and retain the user-data workspace root once. A caller cannot
	// redirect the agent by changing cwd or replacing the configured path later.
	root := strings.TrimSpace(workspaceRoot)
	if root != "" {
		if abs, err := filepath.Abs(root); err == nil {
			if real, err := filepath.EvalSymlinks(abs); err == nil {
				root = real
			} else {
				root = abs
			}
		}
	}
	return &LocalDesktopController{version: strings.TrimSpace(version), workspaceRoot: root}
}

func (c *LocalDesktopController) Status(context.Context) protocol.DesktopStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return protocol.DesktopStatus{Online: true, Version: c.version}
}

func (c *LocalDesktopController) Notify(_ context.Context, n protocol.NotificationRequest) error {
	n.Title = strings.TrimSpace(n.Title)
	n.Body = strings.TrimSpace(n.Body)
	if n.Title == "" || n.Body == "" {
		return fmt.Errorf("notification title and body are required")
	}
	if len([]byte(n.Title))+len([]byte(n.Body)) > 4096 {
		return fmt.Errorf("notification is too large")
	}
	c.mu.Lock()
	c.notifications = append(c.notifications, n)
	c.mu.Unlock()
	return nil
}

func (c *LocalDesktopController) OpenWorkspace(_ context.Context, requested string) (protocol.WorkspaceResult, error) {
	root := c.workspaceRoot
	if strings.TrimSpace(root) == "" {
		return protocol.WorkspaceResult{}, ErrWorkspaceDenied
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return protocol.WorkspaceResult{}, ErrWorkspaceDenied
	}
	path := root
	if strings.TrimSpace(requested) != "" {
		requested = strings.TrimSpace(requested)
		if !filepath.IsAbs(requested) {
			requested = filepath.Join(root, requested)
		}
		path, err = filepath.Abs(requested)
		if err != nil {
			return protocol.WorkspaceResult{}, ErrWorkspaceDenied
		}
	}
	if !isWithinPath(root, path) {
		return protocol.WorkspaceResult{}, ErrWorkspaceDenied
	}
	// Existing targets must resolve inside the real root. This rejects symlink
	// and junction escapes; non-existent paths are allowed only as descendants.
	if info, statErr := os.Lstat(path); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return protocol.WorkspaceResult{}, ErrWorkspaceDenied
		}
		if real, evalErr := filepath.EvalSymlinks(path); evalErr != nil || !isWithinPath(root, real) {
			return protocol.WorkspaceResult{}, ErrWorkspaceDenied
		}
	}
	c.mu.Lock()
	c.lastWorkspace = path
	c.mu.Unlock()
	return protocol.WorkspaceResult{Path: path}, nil
}

func (c *LocalDesktopController) Notifications() []protocol.NotificationRequest {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]protocol.NotificationRequest(nil), c.notifications...)
}

func (c *LocalDesktopController) LastWorkspace() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastWorkspace
}

func isWithinPath(root, candidate string) bool {
	r, c := filepath.Clean(root), filepath.Clean(candidate)
	r = filepath.Clean(r)
	if samePath(r, c) {
		return true
	}
	rel, err := filepath.Rel(r, c)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func samePath(a, b string) bool { return filepath.Clean(a) == filepath.Clean(b) }

type agentSession struct{ expiresAt time.Time }

// AgentSessionManager issues opaque, expiring local capabilities. Session IDs
// are never derived from account/device input and are safe to return to a
// paired client as a bearer capability.
type AgentSessionManager struct {
	mu       sync.Mutex
	ttl      time.Duration
	now      func() time.Time
	sessions map[string]agentSession
}

func NewAgentSessionManager(ttl time.Duration) *AgentSessionManager {
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	return &AgentSessionManager{ttl: ttl, now: time.Now, sessions: make(map[string]agentSession)}
}

func (m *AgentSessionManager) Open() (string, time.Time, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", time.Time{}, err
	}
	now := m.now().UTC()
	expires := now.Add(m.ttl)
	id := hex.EncodeToString(raw[:])
	m.mu.Lock()
	m.sessions[id] = agentSession{expiresAt: expires}
	m.mu.Unlock()
	return id, expires, nil
}

func (m *AgentSessionManager) Valid(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	now := m.now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok || !now.Before(s.expiresAt) {
		delete(m.sessions, id)
		return false
	}
	return true
}

func (m *AgentSessionManager) Close(id string) {
	m.mu.Lock()
	delete(m.sessions, strings.TrimSpace(id))
	m.mu.Unlock()
}

// LocalAgent is the local request processor. It accepts only protocol-defined
// operations and returns sanitized responses suitable for relay/mobile use.
type LocalAgent struct {
	sessions   *AgentSessionManager
	controller DesktopController
	now        func() time.Time
}

func NewLocalAgent(controller DesktopController, sessions *AgentSessionManager) *LocalAgent {
	if controller == nil {
		controller = NewLocalDesktopController("", "")
	}
	if sessions == nil {
		sessions = NewAgentSessionManager(0)
	}
	return &LocalAgent{controller: controller, sessions: sessions, now: time.Now}
}

func (a *LocalAgent) OpenSession() (string, time.Time, error) { return a.sessions.Open() }
func (a *LocalAgent) CloseSession(id string)                  { a.sessions.Close(id) }

func (a *LocalAgent) HandleJSON(ctx context.Context, raw []byte) ([]byte, error) {
	var req protocol.AgentRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, ErrAgentRequestInvalid
	}
	resp := a.Handle(ctx, req)
	return json.Marshal(resp)
}

func (a *LocalAgent) Handle(ctx context.Context, req protocol.AgentRequest) protocol.AgentResponse {
	resp := protocol.AgentResponse{Version: protocol.AgentProtocolVersion, MessageID: req.MessageID, ConversationID: req.ConversationID, Type: protocol.AgentResponseType, Operation: strings.TrimSpace(req.Operation), CreatedAt: a.now().UTC()}
	if req.Version != 0 && req.Version != protocol.AgentProtocolVersion {
		return agentFailure(resp, "invalid_version", "unsupported agent protocol version")
	}
	if strings.TrimSpace(req.MessageID) == "" || !a.sessions.Valid(req.SessionID) {
		return agentFailure(resp, "session_required", ErrAgentSessionRequired.Error())
	}
	if len([]byte(req.Body)) > 8192 {
		return agentFailure(resp, "request_too_large", "agent request is too large")
	}
	op := strings.TrimSpace(req.Operation)
	switch op {
	case protocol.OperationGetDesktopStatus:
		resp.OK, resp.Payload = true, a.controller.Status(ctx)
	case protocol.OperationShowNotification, protocol.OperationNotify:
		var n protocol.NotificationRequest
		if err := decodePayload(req, &n); err != nil {
			return agentFailure(resp, "invalid_notification", "notification payload is invalid")
		}
		if err := a.controller.Notify(ctx, n); err != nil {
			return agentFailure(resp, "notification_denied", "notification was rejected")
		}
		resp.OK, resp.Payload = true, n
	case protocol.OperationOpenWorkspace:
		var in protocol.WorkspaceRequest
		if err := decodePayload(req, &in); err != nil {
			return agentFailure(resp, "invalid_workspace", "workspace payload is invalid")
		}
		result, err := a.controller.OpenWorkspace(ctx, in.Path)
		if err != nil {
			return agentFailure(resp, "workspace_denied", "workspace operation was rejected")
		}
		resp.OK, resp.Payload = true, result
	default:
		return agentFailure(resp, "operation_denied", ErrAgentOperationDenied.Error())
	}
	return resp
}

func decodePayload(req protocol.AgentRequest, out interface{}) error {
	if len(req.Metadata) > 0 {
		b, err := json.Marshal(req.Metadata)
		if err != nil {
			return err
		}
		return json.Unmarshal(b, out)
	}
	if strings.TrimSpace(req.Body) == "" {
		return ErrAgentRequestInvalid
	}
	return json.Unmarshal([]byte(req.Body), out)
}

func agentFailure(resp protocol.AgentResponse, code, message string) protocol.AgentResponse {
	resp.OK = false
	resp.Error = &protocol.AgentError{Code: code, Message: message}
	return resp
}

// Compile-time guard against accidentally introducing an execution-capable
// controller into this package's default implementation.
var _ DesktopController = (*LocalDesktopController)(nil)
