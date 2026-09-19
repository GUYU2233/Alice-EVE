package esi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultBaseURL      = "https://esi.evetech.net/latest/"
	DefaultMaxBodyBytes = int64(16 << 20)
)

// CacheEntry is a complete cached representation. Body must be treated as immutable.
type CacheEntry struct {
	Body      []byte
	ETag      string
	ExpiresAt time.Time
}

// Cache stores ESI representations by their complete URL (including query string).
type Cache interface {
	Get(ctx context.Context, key string) (CacheEntry, bool, error)
	Put(ctx context.Context, key string, entry CacheEntry) error
}

type ErrorKind string

const (
	ErrTimeout  ErrorKind = "timeout"
	ErrHTTP     ErrorKind = "http"
	ErrNetwork  ErrorKind = "network"
	ErrDecode   ErrorKind = "decode"
	ErrTooLarge ErrorKind = "too_large"
)

// Error contains operational retry metadata returned by ESI. It never contains credentials.
type Error struct {
	Kind                ErrorKind
	Status              int
	RetryAfter          time.Duration
	RetryAt             time.Time
	ErrorLimitRemain    int
	ErrorLimitReset     time.Duration
	ErrorLimitRemainSet bool
	ErrorLimitResetSet  bool
	Err                 error
}

func (e *Error) Error() string {
	if e.Status != 0 {
		return fmt.Sprintf("esi %s: HTTP %d", e.Kind, e.Status)
	}
	return fmt.Sprintf("esi %s: %v", e.Kind, e.Err)
}
func (e *Error) Unwrap() error { return e.Err }

// Response describes transport and cache information for a successful request.
type Response struct {
	StatusCode          int
	FromCache           bool
	ETag                string
	ExpiresAt           time.Time
	Pages               int
	ErrorLimitRemain    int
	ErrorLimitReset     time.Duration
	ErrorLimitRemainSet bool
	ErrorLimitResetSet  bool
	RetryAfter          time.Duration
	RetryAt             time.Time
	RetryAfterSet       bool
}

// Client performs bounded ESI HTTP requests. UserAgent is mandatory.
type Client struct {
	BaseURL      string
	UserAgent    string
	Transport    http.RoundTripper
	HTTP         *http.Client // Optional; Transport takes precedence when both are set.
	Cache        Cache
	MaxBodyBytes int64
	Now          func() time.Time
	Limiter      *Limiter
	limiterOnce  sync.Once
}

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Client) baseURL() (*url.URL, error) {
	base := c.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, errors.New("invalid ESI base URL")
	}
	return u, nil
}

func (c *Client) httpClient() *http.Client {
	if c.Transport != nil {
		return &http.Client{Transport: c.Transport, Timeout: 30 * time.Second}
	}
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (c *Client) maxBody() int64 {
	if c.MaxBodyBytes > 0 {
		return c.MaxBodyBytes
	}
	return DefaultMaxBodyBytes
}

func (c *Client) limiter() *Limiter {
	c.limiterOnce.Do(func() {
		if c.Limiter == nil {
			c.Limiter = NewLimiter(DefaultLimiterMaxWait)
		}
	})
	return c.Limiter
}

func (c *Client) buildURL(path string, query url.Values) (string, error) {
	u, err := c.baseURL()
	if err != nil {
		return "", err
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/" + strings.TrimLeft(path, "/")
	q := u.Query()
	for k, values := range query {
		for _, value := range values {
			q.Add(k, value)
		}
	}
	if q.Get("datasource") == "" {
		q.Set("datasource", "tranquility")
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (c *Client) postJSON(ctx context.Context, path string, query url.Values, requestBody any, target any) (Response, error) {
	if strings.TrimSpace(c.UserAgent) == "" {
		return Response{}, errors.New("ESI User-Agent is required")
	}
	rawURL, err := c.buildURL(path, query)
	if err != nil {
		return Response{}, err
	}
	body, err := json.Marshal(requestBody)
	if err != nil {
		return Response{}, &Error{Kind: ErrDecode, Err: err}
	}
	// Universe names accepts at most 1000 integer IDs; 16 KiB is ample and
	// prevents this generic transport from becoming an unbounded request sink.
	if len(body) > 16<<10 {
		return Response{}, &Error{Kind: ErrTooLarge, Err: errors.New("request body exceeds 16384 bytes")}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(string(body)))
	if err != nil {
		return Response{}, &Error{Kind: ErrNetwork, Err: err}
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if err := c.limiter().Wait(ctx); err != nil {
		return Response{}, err
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		kind := ErrNetwork
		if ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) {
			kind = ErrTimeout
		}
		return Response{}, &Error{Kind: kind, Err: err}
	}
	defer resp.Body.Close()
	meta := responseMetadata(resp, c.now())
	c.limiter().Observe(meta)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return meta, httpError(resp, meta, c.now())
	}
	responseBody, err := readBounded(resp.Body, c.maxBody())
	if err != nil {
		return meta, err
	}
	if err := json.Unmarshal(responseBody, target); err != nil {
		return meta, &Error{Kind: ErrDecode, Err: err}
	}
	return meta, nil
}

func (c *Client) getJSON(ctx context.Context, path string, query url.Values, token string, target any) (Response, error) {
	if strings.TrimSpace(c.UserAgent) == "" {
		return Response{}, errors.New("ESI User-Agent is required")
	}
	rawURL, err := c.buildURL(path, query)
	if err != nil {
		return Response{}, err
	}

	var cached CacheEntry
	var cacheHit bool
	if c.Cache != nil {
		cached, cacheHit, err = c.Cache.Get(ctx, rawURL)
		if err != nil {
			return Response{}, fmt.Errorf("ESI cache get: %w", err)
		}
		if cacheHit && cached.ETag == "" && !cached.ExpiresAt.IsZero() && cached.ExpiresAt.After(c.now()) {
			if err := json.Unmarshal(cached.Body, target); err != nil {
				return Response{}, &Error{Kind: ErrDecode, Err: err}
			}
			return Response{StatusCode: http.StatusOK, FromCache: true, ExpiresAt: cached.ExpiresAt}, nil
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Response{}, &Error{Kind: ErrNetwork, Err: err}
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if cacheHit && cached.ETag != "" {
		req.Header.Set("If-None-Match", cached.ETag)
	}
	if err := c.limiter().Wait(ctx); err != nil {
		return Response{}, err
	}

	resp, err := c.httpClient().Do(req)
	if err != nil {
		kind := ErrNetwork
		if ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) {
			kind = ErrTimeout
		}
		return Response{}, &Error{Kind: kind, Err: err}
	}
	defer resp.Body.Close()
	meta := responseMetadata(resp, c.now())
	if resp.StatusCode == http.StatusNotModified {
		if !cacheHit {
			return meta, &Error{Kind: ErrHTTP, Status: resp.StatusCode, Err: errors.New("304 without cached representation")}
		}
		if err := json.Unmarshal(cached.Body, target); err != nil {
			return meta, &Error{Kind: ErrDecode, Err: err}
		}
		meta.FromCache = true
		if meta.ExpiresAt.IsZero() {
			meta.ExpiresAt = cached.ExpiresAt
		}
		return meta, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return meta, httpError(resp, meta, c.now())
	}
	body, err := readBounded(resp.Body, c.maxBody())
	if err != nil {
		return meta, err
	}
	if err := json.Unmarshal(body, target); err != nil {
		return meta, &Error{Kind: ErrDecode, Err: err}
	}
	if c.Cache != nil {
		entry := CacheEntry{Body: append([]byte(nil), body...), ETag: resp.Header.Get("ETag"), ExpiresAt: meta.ExpiresAt}
		if err := c.Cache.Put(ctx, rawURL, entry); err != nil {
			return meta, fmt.Errorf("ESI cache put: %w", err)
		}
	}
	return meta, nil
}

func readBounded(r io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, &Error{Kind: ErrNetwork, Err: err}
	}
	if int64(len(body)) > limit {
		return nil, &Error{Kind: ErrTooLarge, Err: fmt.Errorf("response exceeds %d bytes", limit)}
	}
	return body, nil
}

func responseMetadata(resp *http.Response, now time.Time) Response {
	remain, remainOK := parseIntHeader(resp.Header.Get("X-Esi-Error-Limit-Remain"))
	reset, resetOK := parseIntHeader(resp.Header.Get("X-Esi-Error-Limit-Reset"))
	pages, _ := strconv.Atoi(resp.Header.Get("X-Pages"))
	if pages < 1 {
		pages = 1
	}
	m := Response{StatusCode: resp.StatusCode, ETag: resp.Header.Get("ETag"), Pages: pages, ErrorLimitRemain: remain, ErrorLimitReset: time.Duration(reset) * time.Second, ErrorLimitRemainSet: remainOK, ErrorLimitResetSet: resetOK}
	m.RetryAfter, m.RetryAt, m.RetryAfterSet = parseRetryAfter(resp.Header.Get("Retry-After"), now)
	m.ExpiresAt = expiry(resp.Header, now)
	return m
}

func httpError(resp *http.Response, meta Response, now time.Time) error {
	e := &Error{Kind: ErrHTTP, Status: resp.StatusCode, ErrorLimitRemain: meta.ErrorLimitRemain, ErrorLimitReset: meta.ErrorLimitReset}
	if _, ok := parseIntHeader(resp.Header.Get("X-Esi-Error-Limit-Remain")); ok {
		e.ErrorLimitRemainSet = true
	}
	if _, ok := parseIntHeader(resp.Header.Get("X-Esi-Error-Limit-Reset")); ok {
		e.ErrorLimitResetSet = true
	}
	e.RetryAfter = meta.RetryAfter
	e.RetryAt = meta.RetryAt
	return e
}

func parseRetryAfter(value string, now time.Time) (time.Duration, time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, time.Time{}, false
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		d := time.Duration(seconds) * time.Second
		return d, now.Add(d), true
	}
	if at, err := http.ParseTime(value); err == nil {
		d := time.Duration(0)
		if at.After(now) {
			d = at.Sub(now)
		}
		return d, at, true
	}
	return 0, time.Time{}, false
}

func parseIntHeader(value string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	return n, err == nil
}

func expiry(h http.Header, now time.Time) time.Time {
	for _, directive := range strings.Split(h.Get("Cache-Control"), ",") {
		parts := strings.SplitN(strings.TrimSpace(directive), "=", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "max-age") {
			if seconds, err := strconv.Atoi(strings.Trim(parts[1], `"`)); err == nil && seconds >= 0 {
				return now.Add(time.Duration(seconds) * time.Second)
			}
		}
	}
	if value := h.Get("Expires"); value != "" {
		if t, err := http.ParseTime(value); err == nil {
			return t
		}
	}
	return time.Time{}
}
