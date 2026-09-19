package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"relay-server/internal/esi"
)

func TestPublicESIHandlerNeedsNoAccount(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/universe/types/34/" {
			t.Errorf("path=%s", r.URL.Path)
		}
		w.Header().Set("ETag", `"type34"`)
		w.Header().Set("Cache-Control", "max-age=60")
		_, _ = w.Write([]byte(`{"type_id":34,"name":"Tritanium","published":true}`))
	}))
	defer upstream.Close()
	gateway, err := esi.NewGateway(&esi.Client{BaseURL: upstream.URL, UserAgent: "test", HTTP: upstream.Client()})
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer()
	defer s.Close()
	if err := s.ConfigurePublicESI(gateway); err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/eve/public/universe/types/34", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("ETag") != `"type34"` {
		t.Fatalf("etag=%q", rr.Header().Get("ETag"))
	}
}

func TestPublicESIHandlerRejectsUnboundedID(t *testing.T) {
	s := NewServer()
	defer s.Close()
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/eve/public/universe/types/99999999999", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}
