package conversations

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestServiceAuthorizesTargetAndMessage(t *testing.T) {
	repo := NewMemoryRepository()
	authz := NewMemoryAuthorizer()
	if err := authz.RegisterDevice("acct-1", "mobile-1", DeviceMobile); err != nil {
		t.Fatal(err)
	}
	if err := authz.RegisterDevice("acct-1", "desktop-1", DeviceDesktop); err != nil {
		t.Fatal(err)
	}
	svc := NewService(repo, authz)
	ctx := context.Background()
	conversation, err := svc.CreateConversation(ctx, Actor{AccountID: "acct-1", DeviceID: "mobile-1", Kind: DeviceMobile}, "desktop-1", "eve-agent")
	if err != nil {
		t.Fatal(err)
	}
	message, duplicate, err := svc.SendMessage(ctx, Actor{AccountID: "acct-1", DeviceID: "mobile-1", Kind: DeviceMobile}, conversation.ID, MessageInput{ClientMessageID: "client-1", Operation: "get_desktop_status", Body: "status?"})
	if err != nil || duplicate || message.Status != MessagePersisted || message.Cursor != 1 {
		t.Fatalf("message=%+v duplicate=%v err=%v", message, duplicate, err)
	}
	retried, duplicate, err := svc.SendMessage(ctx, Actor{AccountID: "acct-1", DeviceID: "mobile-1", Kind: DeviceMobile}, conversation.ID, MessageInput{ClientMessageID: "client-1", Operation: "get_desktop_status", Body: "status?"})
	if err != nil || !duplicate || retried.ID != message.ID {
		t.Fatalf("retry=%+v duplicate=%v err=%v", retried, duplicate, err)
	}
	if _, _, err := svc.SendMessage(ctx, Actor{AccountID: "acct-1", DeviceID: "mobile-1", Kind: DeviceMobile}, conversation.ID, MessageInput{ClientMessageID: "client-2", Operation: "run_command", Body: "no"}); !errors.Is(err, ErrInvalidOperation) {
		t.Fatalf("expected invalid operation, got %v", err)
	}
}

func TestServiceStatusDirectionAndMonotonicity(t *testing.T) {
	repo := NewMemoryRepository()
	authz := NewMemoryAuthorizer()
	_ = authz.RegisterDevice("acct", "mob", DeviceMobile)
	_ = authz.RegisterDevice("acct", "desk", DeviceDesktop)
	svc := NewService(repo, authz)
	ctx := context.Background()
	c, err := svc.CreateConversation(ctx, Actor{AccountID: "acct", DeviceID: "mob", Kind: DeviceMobile}, "desk", "agent")
	if err != nil {
		t.Fatal(err)
	}
	m, _, err := svc.SendMessage(ctx, Actor{AccountID: "acct", DeviceID: "mob", Kind: DeviceMobile}, c.ID, MessageInput{ClientMessageID: "m", Operation: "get_recent_intel", Body: "recent"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateMessageStatus(ctx, Actor{AccountID: "acct", DeviceID: "mob", Kind: DeviceMobile}, m.ID, MessageDelivered); !errors.Is(err, ErrForbidden) {
		t.Fatalf("mobile delivered should be forbidden: %v", err)
	}
	if _, err := svc.UpdateMessageStatus(ctx, Actor{AccountID: "acct", DeviceID: "desk", Kind: DeviceDesktop}, m.ID, MessageDelivered); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateMessageStatus(ctx, Actor{AccountID: "acct", DeviceID: "desk", Kind: DeviceDesktop}, m.ID, MessageAccepted); !errors.Is(err, ErrForbidden) {
		t.Fatalf("desktop accepted should be forbidden: %v", err)
	}
	if _, err := svc.UpdateMessageStatus(ctx, Actor{AccountID: "acct", DeviceID: "desk", Kind: DeviceDesktop}, m.ID, MessageProcessed); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateMessageStatus(ctx, Actor{AccountID: "acct", DeviceID: "desk", Kind: DeviceDesktop}, m.ID, MessageDelivered); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("status rollback should fail: %v", err)
	}
}

func TestServiceLimitsSizeAndRate(t *testing.T) {
	repo := NewMemoryRepository()
	authz := NewMemoryAuthorizer()
	_ = authz.RegisterDevice("acct", "mob", DeviceMobile)
	_ = authz.RegisterDevice("acct", "desk", DeviceDesktop)
	clock := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	svc := NewServiceWithConfig(repo, Config{MaxMessageBytes: 20, MessagesPerWindow: 1, RateWindow: time.Minute, Now: func() time.Time { return clock }}, authz)
	c, err := svc.CreateConversation(context.Background(), Actor{AccountID: "acct", DeviceID: "mob", Kind: DeviceMobile}, "desk", "agent")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = svc.SendMessage(context.Background(), Actor{AccountID: "acct", DeviceID: "mob", Kind: DeviceMobile}, c.ID, MessageInput{ClientMessageID: "too-big", Operation: "get_recent_intel", Body: "large body"})
	if !errors.Is(err, ErrMessageTooLarge) {
		t.Fatalf("expected size limit, got %v", err)
	}
	// Use a sufficiently large limit for the rate test.
	svc = NewServiceWithConfig(repo, Config{MaxMessageBytes: 1000, MessagesPerWindow: 1, RateWindow: time.Minute, Now: func() time.Time { return clock }}, authz)
	if _, _, err = svc.SendMessage(context.Background(), Actor{AccountID: "acct", DeviceID: "mob", Kind: DeviceMobile}, c.ID, MessageInput{ClientMessageID: "one", Operation: "get_recent_intel", Body: "ok"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err = svc.SendMessage(context.Background(), Actor{AccountID: "acct", DeviceID: "mob", Kind: DeviceMobile}, c.ID, MessageInput{ClientMessageID: "two", Operation: "get_recent_intel", Body: "ok"}); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("expected rate limit, got %v", err)
	}
}
