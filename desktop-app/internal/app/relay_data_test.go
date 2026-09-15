package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"eve-assistant/desktop-app/internal/protocol"
)

func TestFetchAlertsDecodesCursorEnvelopeAndSendsBearer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/alerts" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer desktop-token" {
			t.Fatalf("authorization=%q", got)
		}
		if r.URL.Query().Get("cursor") != "4" || r.URL.Query().Get("limit") != "7" {
			t.Fatalf("query=%v", r.URL.Query())
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"messages": []map[string]any{{"v": 1, "id": "alert-1", "type": "intel.alert", "sender": "relay", "ts": time.Now().UTC(), "payload": map[string]any{"title": "watch"}}},
			"cursor":   5,
		})
	}))
	defer server.Close()
	client := NewRelayClient()
	if err := client.SetURL(server.URL); err != nil {
		t.Fatal(err)
	}
	client.SetToken("desktop-token")
	page, err := client.FetchAlerts(context.Background(), 4, 7)
	if err != nil {
		t.Fatal(err)
	}
	if page.Cursor != 5 || len(page.Items) != 1 || page.Items[0].MessageID != "alert-1" {
		t.Fatalf("page=%+v", page)
	}
}

func TestFetchOutboxAcceptsLegacyBareArray(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/sync" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"v":1,"id":"out-1","type":"intel.alert","sender":"desktop","ts":"2026-01-01T00:00:00Z","payload":{}}]`))
	}))
	defer server.Close()
	client := NewRelayClient()
	_ = client.SetURL(server.URL)
	page, err := client.FetchOutbox(context.Background(), 0, 10)
	if err != nil || len(page.Items) != 1 || page.Items[0].MessageID != "out-1" {
		t.Fatalf("page=%+v err=%v", page, err)
	}
}

func TestFetchConversationsAndSendMessage(t *testing.T) {
	var sawAuth bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer account-token" {
			t.Fatalf("missing bearer: %q", r.Header.Get("Authorization"))
		}
		sawAuth = true
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/conversations":
			_, _ = w.Write([]byte(`{"conversations":[{"id":"conv-1","accountId":"acct-1","targetDeviceId":"desk-1","agentKind":"eve-agent","status":"active"}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/conversations/conv-1/messages":
			var input protocol.ConversationMessageInput
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if input.ClientMessageID != "client-1" || input.Operation != "get_desktop_status" {
				t.Fatalf("input=%+v", input)
			}
			_, _ = w.Write([]byte(`{"message":{"id":"msg-1","conversationId":"conv-1","body":"online","status":"persisted"},"duplicate":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := NewRelayClient()
	_ = client.SetURL(server.URL)
	client.SetToken("account-token")
	conversations, err := client.FetchConversations(context.Background())
	if err != nil || len(conversations) != 1 || conversations[0].ID != "conv-1" {
		t.Fatalf("conversations=%+v err=%v", conversations, err)
	}
	message, duplicate, err := client.SendConversationMessage(context.Background(), "conv-1", protocol.ConversationMessageInput{ClientMessageID: "client-1", Operation: "get_desktop_status", Body: "status?"})
	if err != nil || !duplicate || message.ID != "msg-1" {
		t.Fatalf("message=%+v duplicate=%v err=%v", message, duplicate, err)
	}
	if !sawAuth {
		t.Fatal("server did not observe authenticated requests")
	}
}

func TestRelayURLRejectsCleartextForeignHosts(t *testing.T) {
	client := NewRelayClient()
	if err := client.SetURL("http://relay.example"); err == nil {
		t.Fatal("foreign cleartext relay URL accepted")
	}
	if err := client.SetURL("https://relay.example/path"); err != nil {
		t.Fatalf("HTTPS relay URL rejected: %v", err)
	}
	if err := client.SetURL("http://127.0.0.1:8080"); err != nil {
		t.Fatalf("loopback development URL rejected: %v", err)
	}
}

func TestRelayHTTPErrorIsReadable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"invalid_token","message":"device session expired"}}`))
	}))
	defer server.Close()
	client := NewRelayClient()
	_ = client.SetURL(server.URL)
	_, err := client.FetchOutbox(context.Background(), 0, 1)
	if err == nil || !strings.Contains(err.Error(), "device session expired") {
		t.Fatalf("error=%v", err)
	}
}
