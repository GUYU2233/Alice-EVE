package api

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func mobileCredentials(t *testing.T, s *Server) DeviceResponse {
	t.Helper()
	create := httptest.NewRecorder()
	s.Handler().ServeHTTP(create, httptest.NewRequest("POST", "/api/v1/pair", nil))
	var pair PairResponse
	if err := json.NewDecoder(create.Body).Decode(&pair); err != nil {
		t.Fatal(err)
	}
	confirm := httptest.NewRecorder()
	s.Handler().ServeHTTP(confirm, httptest.NewRequest("POST", "/api/v1/pair/confirm", bytes.NewBufferString(`{"code":"`+pair.Code+`","deviceName":"phone","deviceType":"mobile"}`)))
	if confirm.Code != 200 {
		t.Fatalf("pair status=%d body=%s", confirm.Code, confirm.Body.String())
	}
	var device DeviceResponse
	if err := json.NewDecoder(confirm.Body).Decode(&device); err != nil {
		t.Fatal(err)
	}
	return device
}

func TestNotificationProviderMetadataRoute(t *testing.T) {
	s := NewServer()
	device := mobileCredentials(t, s)
	req := httptest.NewRequest("GET", "/api/v1/notifications/providers", nil)
	req.Header.Set("Authorization", "Bearer "+device.DeviceToken)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, req)
	if response.Code != 200 || !bytes.Contains(response.Body.Bytes(), []byte(`"provider":"xiaomi"`)) {
		t.Fatalf("metadata status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestNotificationPreferencesAndTokenRoutes(t *testing.T) {
	s := NewServer()
	device := mobileCredentials(t, s)
	get := httptest.NewRequest("GET", "/api/v1/notifications/preferences", nil)
	get.Header.Set("Authorization", "Bearer "+device.DeviceToken)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, get)
	if response.Code != 200 {
		t.Fatalf("preferences status=%d body=%s", response.Code, response.Body.String())
	}
	var doc map[string]any
	if err := json.NewDecoder(response.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	updateBody := map[string]any{"preferences": map[string]any{"enabled": false}, "baseVersion": doc["version"], "ifMatch": doc["etag"]}
	updateJSON, _ := json.Marshal(updateBody)
	put := httptest.NewRequest("PUT", "/api/v1/notifications/preferences", bytes.NewReader(updateJSON))
	put.Header.Set("Authorization", "Bearer "+device.DeviceToken)
	updated := httptest.NewRecorder()
	s.Handler().ServeHTTP(updated, put)
	if updated.Code != 200 || !bytes.Contains(updated.Body.Bytes(), []byte(`"enabled":false`)) {
		t.Fatalf("update status=%d body=%s", updated.Code, updated.Body.String())
	}
	register := httptest.NewRequest("POST", "/api/v1/notifications/tokens", bytes.NewBufferString(`{"provider":"fcm","token":"0123456789abcdef0123456789abcdef"}`))
	register.Header.Set("Authorization", "Bearer "+device.DeviceToken)
	tokenResponse := httptest.NewRecorder()
	s.Handler().ServeHTTP(tokenResponse, register)
	if tokenResponse.Code != 201 {
		t.Fatalf("token status=%d body=%s", tokenResponse.Code, tokenResponse.Body.String())
	}
	var token map[string]any
	_ = json.NewDecoder(tokenResponse.Body).Decode(&token)
	if token["tokenHash"] != nil {
		t.Fatal("raw token hash must not be exposed")
	}
	id, _ := token["id"].(string)
	revoke := httptest.NewRequest("DELETE", "/api/v1/notifications/tokens?id="+id, nil)
	revoke.Header.Set("Authorization", "Bearer "+device.DeviceToken)
	revoked := httptest.NewRecorder()
	s.Handler().ServeHTTP(revoked, revoke)
	if revoked.Code != 204 {
		t.Fatalf("revoke status=%d body=%s", revoked.Code, revoked.Body.String())
	}
}
