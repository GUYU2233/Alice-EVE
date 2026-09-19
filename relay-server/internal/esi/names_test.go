package esi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUniverseNamesPOSTIsPublicBoundedAndTyped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method=%s", r.Method)
		}
		if r.Header.Get("Authorization") != "" {
			t.Fatalf("bearer leaked")
		}
		if r.Header.Get("User-Agent") != "alice-test" {
			t.Fatalf("ua=%q", r.Header.Get("User-Agent"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":34,"name":"Tritanium","category":"inventory_type"}]`))
	}))
	defer srv.Close()
	g, err := NewGateway(&Client{BaseURL: srv.URL + "/", UserAgent: "alice-test", MaxBodyBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := g.UniverseNames(context.Background(), []int64{34, 34})
	if err != nil || len(got) != 1 || got[0].Category != "inventory_type" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if _, _, err = g.UniverseNames(context.Background(), nil); err == nil {
		t.Fatal("accepted empty")
	}
	ids := make([]int64, 1001)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	if _, _, err = g.UniverseNames(context.Background(), ids); err == nil {
		t.Fatal("accepted 1001 IDs")
	}
}

func TestUniverseNamesClassifiesOversizedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`[{"id":34,"name":"long"}]`)) }))
	defer srv.Close()
	g, _ := NewGateway(&Client{BaseURL: srv.URL + "/", UserAgent: "alice-test", MaxBodyBytes: 8})
	_, _, err := g.UniverseNames(context.Background(), []int64{34})
	var ee *Error
	if !errors.As(err, &ee) || ee.Kind != ErrTooLarge {
		t.Fatalf("err=%v", err)
	}
}
