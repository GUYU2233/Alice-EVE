package esi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

type marketCache struct {
	body []byte
	etag string
}

func (c *marketCache) Get(string) ([]byte, string, bool) { return c.body, c.etag, len(c.body) > 0 }
func (c *marketCache) Put(_ string, b []byte, e string)  { c.body = b; c.etag = e }

func TestMarketOrdersFiltersType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/markets/10000002/orders/" {
			t.Errorf("path %s", r.URL.Path)
		}
		if r.URL.Query().Get("datasource") != "tranquility" {
			t.Error("missing datasource")
		}
		w.Header().Set("ETag", "m1")
		w.Write([]byte(`[{"order_id":1,"type_id":34,"price":10},{"order_id":2,"type_id":35,"price":20}]`))
	}))
	defer srv.Close()
	q := QueryService{BaseURL: srv.URL, Client: &Client{HTTP: srv.Client(), Cache: &marketCache{}}}
	got, cached, err := q.MarketOrders(context.Background(), 10000002, 34)
	if err != nil || cached || len(got) != 1 || got[0].OrderID != 1 {
		t.Fatalf("got=%+v cached=%v err=%v", got, cached, err)
	}
}
func TestMarketOrdersRejectsInvalidIDs(t *testing.T) {
	if _, _, err := (QueryService{}).MarketOrders(context.Background(), 0, 34); err == nil {
		t.Fatal("expected error")
	}
}
