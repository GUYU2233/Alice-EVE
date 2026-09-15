package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"relay-server/internal/accounts"
)

func TestDeviceAuthChallengeIsOneShotAndIssuesOpaqueCredentials(t *testing.T) {
	t.Setenv("ALLOW_UNVERIFIED_DEVICE_AUTH", "1")
	s := NewServer()
	startBody := []byte(`{"provider":"test","subject":"account-1","displayName":"Test account","deviceName":"phone","deviceType":"mobile"}`)
	start := httptest.NewRecorder()
	s.Handler().ServeHTTP(start, httptest.NewRequest("POST", "/api/v1/auth/device/start", bytes.NewReader(startBody)))
	if start.Code != 200 {
		t.Fatalf("start status=%d body=%s", start.Code, start.Body.String())
	}
	var challenge DeviceAuthStartResponse
	if err := json.NewDecoder(start.Body).Decode(&challenge); err != nil || challenge.Challenge == "" {
		t.Fatalf("challenge=%+v err=%v", challenge, err)
	}
	completeRequest := func() *httptest.ResponseRecorder {
		r := httptest.NewRecorder()
		s.Handler().ServeHTTP(r, httptest.NewRequest("POST", "/api/v1/auth/device/complete", bytes.NewReader([]byte(`{"challenge":"`+challenge.Challenge+`"}`))))
		return r
	}
	first := completeRequest()
	if first.Code != 200 {
		t.Fatalf("complete status=%d body=%s", first.Code, first.Body.String())
	}
	var issued DeviceAuthResponse
	if err := json.NewDecoder(first.Body).Decode(&issued); err != nil {
		t.Fatal(err)
	}
	if issued.AccessToken == "" || issued.RefreshToken == "" || issued.DeviceToken == "" || issued.AccessToken == issued.RefreshToken || issued.DeviceToken == issued.AccessToken {
		t.Fatalf("credentials are missing or unexpectedly reused: %+v", issued)
	}
	second := completeRequest()
	if second.Code != 401 {
		t.Fatalf("replayed challenge status=%d body=%s", second.Code, second.Body.String())
	}
	request := httptest.NewRequest("GET", "/api/v1/sync", nil)
	request.Header.Set("Authorization", "Bearer "+issued.DeviceToken)
	sync := httptest.NewRecorder()
	s.Handler().ServeHTTP(sync, request)
	if sync.Code != 200 {
		t.Fatalf("device token sync status=%d body=%s", sync.Code, sync.Body.String())
	}
}

func TestDeviceAuthAnonymousBootstrapDisabledByDefault(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("ALLOW_UNVERIFIED_DEVICE_AUTH", "")
	s := NewServer()
	r := httptest.NewRecorder()
	s.Handler().ServeHTTP(r, httptest.NewRequest("POST", "/api/v1/auth/device/start", bytes.NewReader([]byte(`{"provider":"test","subject":"x","deviceName":"phone","deviceType":"mobile"}`))))
	if r.Code != 401 {
		t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
	}
}

func TestDeviceAuthExistingAccountAndRefresh(t *testing.T) {
	t.Setenv("ALLOW_UNVERIFIED_DEVICE_AUTH", "")
	s := NewServer()
	account, err := s.accountService.EnsureAccount(context.Background(), "test", "known", "Known")
	if err != nil {
		t.Fatal(err)
	}
	bootstrap, err := s.accountService.IssueSession(context.Background(), account.ID, "bootstrap")
	if err != nil {
		t.Fatal(err)
	}
	start := httptest.NewRecorder()
	body := []byte(`{"accountId":"` + account.ID + `","deviceName":"desktop","deviceType":"desktop"}`)
	startRequest := httptest.NewRequest("POST", "/api/v1/auth/device/start", bytes.NewReader(body))
	startRequest.Header.Set("Authorization", "Bearer "+bootstrap.AccessToken)
	s.Handler().ServeHTTP(start, startRequest)
	if start.Code != 200 {
		t.Fatalf("start status=%d body=%s", start.Code, start.Body.String())
	}
	var challenge DeviceAuthStartResponse
	_ = json.NewDecoder(start.Body).Decode(&challenge)
	complete := httptest.NewRecorder()
	s.Handler().ServeHTTP(complete, httptest.NewRequest("POST", "/api/v1/auth/device/complete", bytes.NewReader([]byte(`{"challenge":"`+challenge.Challenge+`"}`))))
	if complete.Code != 200 {
		t.Fatalf("complete status=%d body=%s", complete.Code, complete.Body.String())
	}
	var issued DeviceAuthResponse
	_ = json.NewDecoder(complete.Body).Decode(&issued)
	refresh := httptest.NewRecorder()
	s.Handler().ServeHTTP(refresh, httptest.NewRequest("POST", "/api/v1/auth/refresh", bytes.NewReader([]byte(`{"refreshToken":"`+issued.RefreshToken+`"}`))))
	if refresh.Code != 200 {
		t.Fatalf("refresh status=%d body=%s", refresh.Code, refresh.Body.String())
	}
	var rotated refreshResponse
	_ = json.NewDecoder(refresh.Body).Decode(&rotated)
	if rotated.AccessToken == issued.AccessToken || rotated.RefreshToken == issued.RefreshToken {
		t.Fatal("refresh did not rotate credentials")
	}
	if _, err := s.accountService.Refresh(context.Background(), issued.RefreshToken); err != accounts.ErrRefreshReplay {
		t.Fatalf("old refresh error=%v, want replay", err)
	}
}
