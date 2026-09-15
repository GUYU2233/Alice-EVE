package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestPreferencesVersionAndETag(t *testing.T) {
	s := NewService()
	ctx := context.Background()
	d, err := s.GetPreferences(ctx, "device-1")
	if err != nil || d.Version != 1 || !d.Preferences.Enabled {
		t.Fatalf("initial preferences: %#v %v", d, err)
	}
	next, err := s.UpdatePreferences(ctx, UpdateRequest{DeviceID: "device-1", BaseVersion: d.Version, IfMatch: d.ETag, Preferences: ReminderPreferences{Enabled: false}})
	if err != nil || next.Version != 2 || next.Preferences.Enabled {
		t.Fatalf("updated preferences: %#v %v", next, err)
	}
	_, err = s.UpdatePreferences(ctx, UpdateRequest{DeviceID: "device-1", BaseVersion: d.Version, IfMatch: d.ETag, Preferences: DefaultPreferences()})
	var conflict *ConflictError
	if !errors.As(err, &conflict) || conflict.Current.Version != 2 {
		t.Fatalf("want conflict, got %v", err)
	}
}

func TestPushTokenRegistrationHashesAndRevokes(t *testing.T) {
	s := NewService()
	ctx := context.Background()
	token, err := s.RegisterToken(ctx, TokenRegistration{DeviceID: "device-1", Provider: "FCM", Token: "0123456789abcdef0123456789abcdef"})
	if err != nil {
		t.Fatal(err)
	}
	if token.TokenHash == "" || token.ID == "" || token.Provider != "fcm" || token.Platform != "android" {
		t.Fatalf("unexpected token: %#v", token)
	}
	if err := s.RevokeToken(ctx, "device-1", token.ID, ""); err != nil {
		t.Fatal(err)
	}
	if got := s.ListTokens(ctx, "device-1"); len(got) != 0 {
		t.Fatalf("revoked token still active: %#v", got)
	}
	if err := s.RevokeToken(ctx, "device-1", token.ID, ""); !errors.Is(err, ErrTokenNotFound) {
		t.Fatalf("want not found, got %v", err)
	}
}

func TestProviderPlatformValidationAndMetadata(t *testing.T) {
	s := NewService()
	ctx := context.Background()
	for _, provider := range []string{ProviderFCM, ProviderXiaomi, ProviderHuawei, ProviderOPPO, ProviderVivo, ProviderAPNs, ProviderWebPush} {
		if _, err := s.RegisterToken(ctx, TokenRegistration{DeviceID: "device-1", Provider: provider, Token: "0123456789abcdef0123456789abcdef"}); err != nil {
			t.Fatalf("provider %s rejected: %v", provider, err)
		}
	}
	if _, err := s.RegisterToken(ctx, TokenRegistration{DeviceID: "device-1", Provider: ProviderXiaomi, Platform: "ios", Token: "0123456789abcdef0123456789abcdef"}); !errors.Is(err, ErrInvalidPlatform) {
		t.Fatalf("want invalid platform, got %v", err)
	}
	metadata := s.ProviderMetadata()
	if len(metadata) != 7 || metadata[1].Provider != ProviderXiaomi || metadata[1].Configured {
		t.Fatalf("unexpected metadata: %#v", metadata)
	}
}

type recordingDispatcher struct{ got Delivery }

func (d *recordingDispatcher) Dispatch(_ context.Context, delivery Delivery) error {
	d.got = delivery
	return nil
}

func TestDispatchRequiresConfigurationAndNeverReceivesRawToken(t *testing.T) {
	s := NewService()
	ctx := context.Background()
	registered, err := s.RegisterToken(ctx, TokenRegistration{DeviceID: "device-1", Provider: ProviderVivo, Token: "0123456789abcdef0123456789abcdef"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Dispatch(ctx, registered.ID, "device-1", "title", "body", nil); !errors.Is(err, ErrProviderNotConfigured) {
		t.Fatalf("want not configured, got %v", err)
	}
	d := &recordingDispatcher{}
	if err := s.SetDispatcher(ProviderVivo, d); err != nil {
		t.Fatal(err)
	}
	if err := s.Dispatch(ctx, registered.ID, "device-1", "title", "body", nil); err != nil {
		t.Fatal(err)
	}
	if d.got.Token.TokenHash == "" || d.got.Title != "title" {
		t.Fatalf("unexpected delivery: %#v", d.got)
	}
	if stringMustNotContainJSON(t, d.got, "0123456789abcdef0123456789abcdef") {
		t.Fatal("raw token leaked into delivery")
	}
}

func stringMustNotContainJSON(t *testing.T, v any, secret string) bool {
	t.Helper()
	b, _ := json.Marshal(v)
	return strings.Contains(string(b), secret)
}
