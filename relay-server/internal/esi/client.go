package esi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Cache interface {
	Get(key string) (body []byte, etag string, ok bool)
	Put(key string, body []byte, etag string)
}
type ErrorKind string

const (
	ErrTimeout ErrorKind = "timeout"
	ErrHTTP    ErrorKind = "http"
	ErrNetwork ErrorKind = "network"
)

type Error struct {
	Kind   ErrorKind
	Status int
	Err    error
}

func (e *Error) Error() string {
	if e.Status > 0 {
		return fmt.Sprintf("esi %s: http %d", e.Kind, e.Status)
	}
	return fmt.Sprintf("esi %s: %v", e.Kind, e.Err)
}
func (e *Error) Unwrap() error { return e.Err }

type Client struct {
	HTTP      *http.Client
	UserAgent string
	Cache     Cache
}

func (c *Client) Get(ctx context.Context, url string) ([]byte, bool, error) {
	h := c.HTTP
	if h == nil {
		h = &http.Client{Timeout: 10 * time.Second}
	}
	var etag string
	if c.Cache != nil {
		if b, e, ok := c.Cache.Get(url); ok {
			etag = e
			_ = b
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, &Error{Kind: ErrNetwork, Err: err}
	}
	ua := c.UserAgent
	if ua == "" {
		ua = "Alice-EVE/1.0 (ESI client)"
	}
	req.Header.Set("User-Agent", ua)
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := h.Do(req)
	if err != nil {
		k := ErrNetwork
		if ctx.Err() != nil || err == context.DeadlineExceeded {
			k = ErrTimeout
		}
		return nil, false, &Error{Kind: k, Err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		if c.Cache != nil {
			if b, _, ok := c.Cache.Get(url); ok {
				return b, true, nil
			}
		}
		return nil, true, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, false, &Error{Kind: ErrHTTP, Status: resp.StatusCode}
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, &Error{Kind: ErrNetwork, Err: err}
	}
	if c.Cache != nil {
		c.Cache.Put(url, b, resp.Header.Get("ETag"))
	}
	return b, false, nil
}
