package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"relay-server/internal/conversations"
	"relay-server/internal/store"
)

func TestConversationReconcileSnapshotIsAuthenticatedScopedAndStable(t *testing.T) {
	s := NewServer()
	ctx := context.Background()
	now := time.Now().UTC()
	acct, err := s.accountService.EnsureAccount(ctx, "test", "subject-reconcile", "Reconcile")
	if err != nil {
		t.Fatal(err)
	}
	issued, err := s.accountService.IssueSession(ctx, acct.ID, "mobile-reconcile")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.store.AddDevice(store.Device{ID: "mobile-reconcile", AccountID: acct.ID, Type: "mobile", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.conversationRepo.CreateConversation(ctx, conversations.Conversation{ID: "c-1", AccountID: acct.ID, TargetDeviceID: "desktop-reconcile", AgentKind: "eve", Status: conversations.ConversationActive, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.conversationRepo.CreateConversation(ctx, conversations.Conversation{ID: "c-other", AccountID: "other", TargetDeviceID: "desktop-other", AgentKind: "eve", Status: conversations.ConversationActive, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	_, _, err = s.conversationRepo.PutMessage(ctx, conversations.Message{ID: "m-1", AccountID: acct.ID, ConversationID: "c-1", SenderKind: conversations.SenderMobile, SenderDeviceID: "mobile-reconcile", ClientMessageID: "client-1", Operation: "get_desktop_status", Body: "hello", Status: conversations.MessagePersisted, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	unauth := httptest.NewRecorder()
	s.Handler().ServeHTTP(unauth, httptest.NewRequest(http.MethodGet, "/api/v1/realtime/snapshot", nil))
	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf("unauth status=%d", unauth.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/realtime/snapshot", nil)
	req.Header.Set("Authorization", "Bearer "+issued.AccessToken)
	got := httptest.NewRecorder()
	s.Handler().ServeHTTP(got, req)
	if got.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", got.Code, got.Body.String())
	}
	var response conversationReconcileResponse
	if err := json.NewDecoder(got.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.SnapshotCursor != 1 || len(response.Conversations) != 1 {
		t.Fatalf("response=%+v", response)
	}
	if response.Conversations[0].ID != "c-1" || len(response.Conversations[0].Messages) != 1 {
		t.Fatalf("conversation=%+v", response.Conversations[0])
	}
	if response.Conversations[0].Messages[0].ClientMessageID != "client-1" {
		t.Fatal("message missing clear JSON fields")
	}
}

func TestConversationReconcileMobileIncludesRunsAndJSONTags(t *testing.T) {
	s := NewServer()
	ctx := context.Background()
	now := time.Now().UTC()
	acct, err := s.accountService.EnsureAccount(ctx, "test", "subject-reconcile-runs", "Runs")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.store.AddDevice(store.Device{ID: "mobile-runs", AccountID: acct.ID, Type: "mobile", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	issued, err := s.accountService.IssueSession(ctx, acct.ID, "mobile-runs")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.conversationRepo.CreateConversation(ctx, conversations.Conversation{ID: "c-runs", AccountID: acct.ID, TargetDeviceID: "desktop-runs", AgentKind: "eve", Status: conversations.ConversationActive, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.conversationRepo.CreateRun(ctx, conversations.AgentRun{ID: "run-1", AccountID: acct.ID, ConversationID: "c-runs", InputMessageID: "m-none", Status: conversations.RunQueued, StartedAt: now}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v2/realtime/snapshot?messageLimit=1&runLimit=1", bytes.NewReader(nil))
	req.Header.Set("Authorization", "Bearer "+issued.AccessToken)
	got := httptest.NewRecorder()
	s.Handler().ServeHTTP(got, req)
	if got.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", got.Code, got.Body.String())
	}
	var raw map[string]any
	if err := json.NewDecoder(got.Body).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["snapshotCursor"]; !ok {
		t.Fatal("snapshotCursor absent")
	}
	if _, ok := raw["conversations"]; !ok {
		t.Fatal("conversations absent")
	}
}
