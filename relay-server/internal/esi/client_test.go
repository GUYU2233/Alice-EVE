package esi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

type mc struct {
	b []byte
	e string
}

func (m *mc) Get(string) ([]byte, string, bool) { return m.b, m.e, m.b != nil }
func (m *mc) Put(_ string, b []byte, e string)  { m.b = b; m.e = e }
func TestClientETag304(t *testing.T) {
	n := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		if r.Header.Get("User-Agent") == "" {
			t.Error("ua")
		}
		if r.Header.Get("If-None-Match") == "v1" {
			w.WriteHeader(304)
			return
		}
		w.Header().Set("ETag", "v1")
		w.Write([]byte("ok"))
	}))
	defer s.Close()
	c := &Client{Cache: &mc{}}
	b, _, _ := c.Get(context.Background(), s.URL)
	if string(b) != "ok" {
		t.Fatal()
	}
	b, cached, e := c.Get(context.Background(), s.URL)
	if e != nil || !cached || string(b) != "ok" || n != 2 {
		t.Fatal(b, cached, e, n)
	}
}
