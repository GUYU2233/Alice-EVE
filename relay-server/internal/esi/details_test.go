package esi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOnDemandDetailRoutes(t *testing.T) {
	cases := []struct {
		name, path string
		call       func(*Gateway) error
	}{
		{"items", "/characters/42/contracts/9/items/", func(g *Gateway) error {
			_, _, e := g.CharacterContractItems(context.Background(), 42, 9, "tok")
			return e
		}},
		{"bids", "/characters/42/contracts/9/bids/", func(g *Gateway) error {
			_, _, e := g.CharacterContractBids(context.Background(), 42, 9, "tok")
			return e
		}},
		{"mail", "/characters/42/mail/7/", func(g *Gateway) error { _, _, e := g.CharacterMailBody(context.Background(), 42, 7, "tok"); return e }},
		{"killmail", "/killmails/8/abcdefghijklmnopqrst/", func(g *Gateway) error {
			_, _, e := g.KillmailDetail(context.Background(), 8, "abcdefghijklmnopqrst")
			return e
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.path {
					t.Fatalf("path=%s", r.URL.Path)
				}
				if tc.name != "killmail" && r.Header.Get("Authorization") != "Bearer tok" {
					t.Fatalf("missing bearer")
				}
				if tc.name == "mail" {
					_, _ = w.Write([]byte(`{"mail_id":7,"body":"sensitive"}`))
				} else if tc.name == "killmail" {
					_, _ = w.Write([]byte(`{"killmail_id":8}`))
				} else {
					_, _ = w.Write([]byte(`[]`))
				}
			}))
			defer srv.Close()
			g, e := NewGateway(&Client{BaseURL: srv.URL, UserAgent: "test (+a@b.test)", HTTP: srv.Client()})
			if e != nil {
				t.Fatal(e)
			}
			if e = tc.call(g); e != nil {
				t.Fatal(e)
			}
		})
	}
}
func TestKillmailHashValidation(t *testing.T) {
	g, _ := NewGateway(&Client{BaseURL: "https://example.test", UserAgent: "test (+a@b.test)"})
	if _, _, e := g.KillmailDetail(context.Background(), 1, "bad/path"); e == nil {
		t.Fatal("expected invalid hash")
	}
}
