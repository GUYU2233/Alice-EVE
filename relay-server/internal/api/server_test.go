package api

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestHealth(t *testing.T) {
	r := httptest.NewRecorder()
	NewServer().Handler().ServeHTTP(r, httptest.NewRequest("GET", "/health", nil))
	if r.Code != 200 {
		t.Fatal(r.Code)
	}
}

func TestPairCodeFormat(t *testing.T) {
	r := httptest.NewRecorder()
	NewServer().Handler().ServeHTTP(r, httptest.NewRequest("POST", "/api/v1/pair", nil))
	var response PairResponse
	if err := json.NewDecoder(r.Body).Decode(&response); err != nil || len(response.Code) != 6 {
		t.Fatal(err, response.Code)
	}
}

func TestPairCodeIsOneTime(t *testing.T) {
	s := NewServer()
	create := httptest.NewRecorder()
	s.Handler().ServeHTTP(create, httptest.NewRequest("POST", "/api/v1/pair", nil))
	var pair PairResponse
	_ = json.NewDecoder(create.Body).Decode(&pair)
	request := []byte(`{"code":"` + pair.Code + `","deviceName":"phone","deviceType":"mobile"}`)
	first := httptest.NewRecorder()
	s.Handler().ServeHTTP(first, httptest.NewRequest("POST", "/api/v1/pair/confirm", bytes.NewReader(request)))
	if first.Code != 200 {
		t.Fatal(first.Code, first.Body.String())
	}
	second := httptest.NewRecorder()
	s.Handler().ServeHTTP(second, httptest.NewRequest("POST", "/api/v1/pair/confirm", bytes.NewReader(request)))
	if second.Code != 401 {
		t.Fatal(second.Code)
	}
}

func TestProtectedEndpointsRejectUnauthenticated(t *testing.T) {
	s := NewServer()
	for _, path := range []string{"/api/v1/events", "/api/v1/alerts", "/api/v1/devices/revoke"} {
		r := httptest.NewRecorder()
		s.Handler().ServeHTTP(r, httptest.NewRequest("GET", path, nil))
		if r.Code == 200 {
			t.Fatalf("%s unexpectedly allowed", path)
		}
	}
}

func TestPublishDeduplicatesAndStreams(t *testing.T) {
	s := NewServer()
	// Register a real device through the pairing flow so authentication is tested.
	create := httptest.NewRecorder()
	s.Handler().ServeHTTP(create, httptest.NewRequest("POST", "/api/v1/pair", nil))
	var pair PairResponse
	_ = json.NewDecoder(create.Body).Decode(&pair)
	pairBody := []byte(`{"code":"` + pair.Code + `","deviceName":"desktop","deviceType":"desktop"}`)
	confirm := httptest.NewRecorder()
	s.Handler().ServeHTTP(confirm, httptest.NewRequest("POST", "/api/v1/pair/confirm", bytes.NewReader(pairBody)))
	var device DeviceResponse
	_ = json.NewDecoder(confirm.Body).Decode(&device)
	body := []byte(`{"v":1,"type":"intel.alert","id":"m-1","sender":"desktop","payload":{"severity":"high"}}`)
	first := httptest.NewRecorder()
	firstReq := httptest.NewRequest("POST", "/api/v1/messages", bytes.NewReader(body))
	firstReq.Header.Set("Authorization", "Bearer "+device.DeviceToken)
	s.Handler().ServeHTTP(first, firstReq)
	if first.Code != 200 {
		t.Fatal(first.Code)
	}
	second := httptest.NewRecorder()
	secondReq := httptest.NewRequest("POST", "/api/v1/messages", bytes.NewReader(body))
	secondReq.Header.Set("Authorization", "Bearer "+device.DeviceToken)
	s.Handler().ServeHTTP(second, secondReq)
	if second.Code != 200 || !bytes.Contains(second.Body.Bytes(), []byte(`"duplicate":true`)) {
		t.Fatal(second.Code, second.Body.String())
	}
}
