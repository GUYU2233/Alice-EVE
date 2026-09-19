package esi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExtendedCharacterReadOnlyRoutes(t *testing.T) {
	cases := []struct {
		name, path string
		call       func(context.Context, *Gateway) error
	}{
		{"clones", "/characters/42/clones/", func(c context.Context, g *Gateway) error { _, _, e := g.CharacterClones(c, 42, "tok"); return e }},
		{"implants", "/characters/42/implants/", func(c context.Context, g *Gateway) error { _, _, e := g.CharacterImplants(c, 42, "tok"); return e }},
		{"transactions", "/characters/42/wallet/transactions/", func(c context.Context, g *Gateway) error {
			_, _, e := g.CharacterWalletTransactions(c, 42, "tok")
			return e
		}},
		{"order-history", "/characters/42/orders/history/", func(c context.Context, g *Gateway) error { _, _, e := g.CharacterOrders(c, 42, "tok", true); return e }},
		{"contracts", "/characters/42/contracts/", func(c context.Context, g *Gateway) error { _, _, e := g.CharacterContracts(c, 42, "tok"); return e }},
		{"mail-headers", "/characters/42/mail/", func(c context.Context, g *Gateway) error { _, _, e := g.CharacterMailHeaders(c, 42, "tok"); return e }},
		{"fittings", "/characters/42/fittings/", func(c context.Context, g *Gateway) error { _, _, e := g.CharacterFittings(c, 42, "tok"); return e }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != tc.path {
					t.Fatalf("%s %s", r.Method, r.URL.Path)
				}
				if r.Header.Get("Authorization") != "Bearer tok" {
					t.Fatalf("auth=%q", r.Header.Get("Authorization"))
				}
				body := `[]`
				if tc.name == "clones" {
					body = `{}`
				}
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			g, err := NewGateway(&Client{BaseURL: server.URL, UserAgent: "Alice-EVE/test (+ops@example.test)", HTTP: server.Client()})
			if err != nil {
				t.Fatal(err)
			}
			if err := tc.call(context.Background(), g); err != nil {
				t.Fatal(err)
			}
		})
	}
}
