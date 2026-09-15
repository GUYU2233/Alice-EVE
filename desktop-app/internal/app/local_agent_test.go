package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"eve-assistant/desktop-app/internal/protocol"
)

func TestLocalAgentStatusAndSessionLifecycle(t *testing.T) {
	controller := NewLocalDesktopController("test", t.TempDir())
	agent := NewLocalAgent(controller, NewAgentSessionManager(time.Minute))
	session, _, err := agent.OpenSession()
	if err != nil {
		t.Fatal(err)
	}
	resp := agent.Handle(context.Background(), protocol.AgentRequest{Version: 1, MessageID: "m-1", SessionID: session, Operation: protocol.OperationGetDesktopStatus})
	if !resp.OK || resp.Error != nil {
		t.Fatalf("response=%+v", resp)
	}
	status, ok := resp.Payload.(protocol.DesktopStatus)
	if !ok || !status.Online || status.Version != "test" {
		t.Fatalf("payload=%#v", resp.Payload)
	}
	agent.CloseSession(session)
	resp = agent.Handle(context.Background(), protocol.AgentRequest{Version: 1, MessageID: "m-2", SessionID: session, Operation: protocol.OperationGetDesktopStatus})
	if resp.OK || resp.Error == nil || resp.Error.Code != "session_required" {
		t.Fatalf("closed session response=%+v", resp)
	}
}

func TestLocalAgentWhitelistRejectsCommand(t *testing.T) {
	agent := NewLocalAgent(NewLocalDesktopController("test", t.TempDir()), nil)
	session, _, err := agent.OpenSession()
	if err != nil {
		t.Fatal(err)
	}
	resp := agent.Handle(context.Background(), protocol.AgentRequest{Version: 1, MessageID: "m", SessionID: session, Operation: "run_command", Body: "echo unsafe"})
	if resp.OK || resp.Error == nil || resp.Error.Code != "operation_denied" {
		t.Fatalf("response=%+v", resp)
	}
}

func TestLocalAgentNotificationAndWorkspace(t *testing.T) {
	root := t.TempDir()
	controller := NewLocalDesktopController("test", root)
	agent := NewLocalAgent(controller, nil)
	session, _, _ := agent.OpenSession()

	notification := protocol.NotificationRequest{Title: "Alert", Body: "Safe local notice", Severity: protocol.SeverityInfo}
	body, _ := json.Marshal(notification)
	resp := agent.Handle(context.Background(), protocol.AgentRequest{Version: 1, MessageID: "n", SessionID: session, Operation: protocol.OperationShowNotification, Body: string(body)})
	if !resp.OK || len(controller.Notifications()) != 1 {
		t.Fatalf("notification response=%+v notifications=%+v", resp, controller.Notifications())
	}

	inside := filepath.Join(root, "notes")
	workspaceBody, _ := json.Marshal(protocol.WorkspaceRequest{Path: inside})
	resp = agent.Handle(context.Background(), protocol.AgentRequest{Version: 1, MessageID: "w", SessionID: session, Operation: protocol.OperationOpenWorkspace, Body: string(workspaceBody)})
	if !resp.OK || controller.LastWorkspace() != inside {
		t.Fatalf("workspace response=%+v last=%q", resp, controller.LastWorkspace())
	}

	outside := filepath.Join(root, "..", "not-allowed")
	workspaceBody, _ = json.Marshal(protocol.WorkspaceRequest{Path: outside})
	resp = agent.Handle(context.Background(), protocol.AgentRequest{Version: 1, MessageID: "w2", SessionID: session, Operation: protocol.OperationOpenWorkspace, Body: string(workspaceBody)})
	if resp.OK || resp.Error == nil || resp.Error.Code != "workspace_denied" {
		t.Fatalf("outside response=%+v", resp)
	}
	// Symlink/junction targets must not escape the explicit workspace root.
	outsideDir := t.TempDir()
	link := filepath.Join(root, "escape-link")
	if err := os.Symlink(outsideDir, link); err == nil {
		workspaceBody, _ = json.Marshal(protocol.WorkspaceRequest{Path: link})
		resp = agent.Handle(context.Background(), protocol.AgentRequest{Version: 1, MessageID: "w3", SessionID: session, Operation: protocol.OperationOpenWorkspace, Body: string(workspaceBody)})
		if resp.OK || resp.Error == nil || resp.Error.Code != "workspace_denied" {
			t.Fatalf("symlink escape accepted: %+v", resp)
		}
	}
}

func TestLocalAgentHandleJSONSanitizesErrors(t *testing.T) {
	agent := NewLocalAgent(NewLocalDesktopController("test", t.TempDir()), nil)
	session, _, _ := agent.OpenSession()
	request, _ := json.Marshal(protocol.AgentRequest{Version: 1, MessageID: "json-1", SessionID: session, Operation: "rm -rf /"})
	encoded, err := agent.HandleJSON(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	var response protocol.AgentResponse
	if err := json.Unmarshal(encoded, &response); err != nil {
		t.Fatal(err)
	}
	if response.OK || response.Error == nil || response.Error.Code != "operation_denied" {
		t.Fatalf("response=%s", encoded)
	}
	if response.Error.Message == "" || response.Error.Message == "rm -rf /" {
		t.Fatalf("unsafe error leakage: %+v", response.Error)
	}
	if _, err := agent.HandleJSON(context.Background(), []byte("not-json")); !errors.Is(err, ErrAgentRequestInvalid) {
		t.Fatalf("invalid JSON error=%v", err)
	}
}

func TestLocalAgentExpiresSessions(t *testing.T) {
	now := time.Now().UTC()
	manager := NewAgentSessionManager(time.Minute)
	manager.now = func() time.Time { return now }
	id, expires, err := manager.Open()
	if err != nil || !expires.Equal(now.Add(time.Minute)) || !manager.Valid(id) {
		t.Fatalf("id=%q expires=%v err=%v", id, expires, err)
	}
	manager.now = func() time.Time { return now.Add(time.Minute) }
	if manager.Valid(id) {
		t.Fatal("expired session accepted")
	}
}
