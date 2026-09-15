package eve

import (
	"bytes"
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

var (
	ErrInvalidESIPath      = errors.New("ESI path is not allowed")
	ErrESIResponseTooLarge = errors.New("ESI response exceeds the configured limit")
	ErrESIRetryExhausted   = errors.New("ESI retry limit exceeded")
)

type ESIClient struct {
	BaseURL   string
	Client    *http.Client
	UserAgent string
	CacheTTL  time.Duration
	// MaxRetries is the number of retries after the initial request. Only
	// throttled responses (420/429) are retried.
	MaxRetries       int
	MaxRetryDelay    time.Duration
	RetryBaseDelay   time.Duration
	MaxResponseBytes int64
	mu               sync.RWMutex
	cache            map[string]cachedESI
}

type cachedESI struct {
	response ESIResponse
	cachedAt time.Time
	noStore  bool
}
type ESIResponse struct {
	Status       int             `json:"status"`
	Body         json.RawMessage `json:"body"`
	ExpiresAt    time.Time       `json:"expiresAt"`
	FromCache    bool            `json:"fromCache"`
	ETag         string          `json:"etag,omitempty"`
	CacheControl string          `json:"cacheControl,omitempty"`
	RetryAfter   time.Duration   `json:"retryAfter,omitempty"`
	Attempts     int             `json:"attempts,omitempty"`
}

func NewESIClient() *ESIClient {
	return &ESIClient{BaseURL: "https://esi.evetech.net", Client: &http.Client{Timeout: 12 * time.Second}, UserAgent: "Alice-EVE/Desktop", CacheTTL: 5 * time.Minute, MaxRetries: 3, MaxRetryDelay: 30 * time.Second, RetryBaseDelay: 250 * time.Millisecond, MaxResponseBytes: 8 << 20, cache: make(map[string]cachedESI)}
}
func (c *ESIClient) validatePath(path string) (string, error) {
	if path == "" || !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.Contains(path, "..") || strings.ContainsAny(path, "\\?#") {
		return "", ErrInvalidESIPath
	}
	base, err := url.Parse(c.BaseURL)
	if err != nil || base.Scheme != "https" || base.Host == "" {
		return "", ErrInvalidESIPath
	}
	u, err := url.Parse(base.String() + path)
	if err != nil || u.Scheme != "https" || u.Host != base.Host {
		return "", ErrInvalidESIPath
	}
	return u.String(), nil
}

func parseCacheControl(value string) (maxAge time.Duration, noCache, noStore bool) {
	for _, token := range strings.Split(value, ",") {
		parts := strings.SplitN(strings.TrimSpace(token), "=", 2)
		name := strings.ToLower(strings.TrimSpace(parts[0]))
		switch name {
		case "no-cache":
			noCache = true
		case "no-store":
			noStore = true
		case "max-age":
			if len(parts) == 2 {
				if seconds, err := strconv.ParseInt(strings.Trim(strings.TrimSpace(parts[1]), "\""), 10, 64); err == nil && seconds >= 0 {
					maxAge = time.Duration(seconds) * time.Second
				}
			}
		}
	}
	return
}
func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil && at.After(now) {
		return at.Sub(now)
	}
	return 0
}
func (c *ESIClient) retryDelay(resp *http.Response, attempt int) time.Duration {
	d := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
	if d == 0 {
		base := c.RetryBaseDelay
		if base <= 0 {
			base = 250 * time.Millisecond
		}
		d = base * time.Duration(1<<minInt(attempt, 6))
	}
	max := c.MaxRetryDelay
	if max <= 0 {
		max = 30 * time.Second
	}
	if d > max {
		d = max
	}
	return d
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (c *ESIClient) Get(ctx context.Context, path string) (ESIResponse, error) {
	full, err := c.validatePath(path)
	if err != nil {
		return ESIResponse{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ttl := c.CacheTTL
	if ttl == 0 {
		ttl = 5 * time.Minute
	}
	disableFreshCache := ttl < 0
	c.mu.RLock()
	hit, hasHit := c.cache[path]
	c.mu.RUnlock()
	if hasHit && !disableFreshCache && !hit.noStore && time.Since(hit.cachedAt) < ttl && (hit.response.ExpiresAt.IsZero() || time.Now().UTC().Before(hit.response.ExpiresAt)) {
		hit.response.FromCache = true
		hit.response.Attempts = 0
		return hit.response, nil
	}
	cl := c.Client
	if cl == nil {
		cl = &http.Client{Timeout: 12 * time.Second}
	}
	maxRetries := c.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}
	for attempt := 0; ; attempt++ {
		req, e := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
		if e != nil {
			return ESIResponse{}, e
		}
		req.Header.Set("Accept", "application/json")
		ua := c.UserAgent
		if ua == "" {
			ua = "Alice-EVE/Desktop"
		}
		req.Header.Set("User-Agent", ua)
		if hasHit && hit.response.ETag != "" {
			req.Header.Set("If-None-Match", hit.response.ETag)
		}
		resp, e := cl.Do(req)
		if e != nil {
			return ESIResponse{Attempts: attempt + 1}, e
		}
		if resp.StatusCode == http.StatusNotModified && hasHit {
			_, noCache, noStore := parseCacheControl(resp.Header.Get("Cache-Control"))
			result := hit.response
			result.Status = http.StatusOK
			result.FromCache = true
			result.Attempts = attempt + 1
			if tag := resp.Header.Get("ETag"); tag != "" {
				result.ETag = tag
			}
			if cc := resp.Header.Get("Cache-Control"); cc != "" {
				result.CacheControl = cc
			}
			result.ExpiresAt = expiry(resp.Header, ttl, noCache)
			hit.response = result
			hit.cachedAt = time.Now()
			hit.noStore = noStore
			resp.Body.Close()
			if noStore {
				c.mu.Lock()
				delete(c.cache, path)
				c.mu.Unlock()
			} else {
				c.mu.Lock()
				c.cache[path] = hit
				c.mu.Unlock()
			}
			return result, nil
		}
		if (resp.StatusCode == 420 || resp.StatusCode == http.StatusTooManyRequests) && attempt < maxRetries {
			delay := c.retryDelay(resp, attempt)
			resp.Body.Close()
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ESIResponse{Status: resp.StatusCode, RetryAfter: delay, Attempts: attempt + 1}, ctx.Err()
			case <-timer.C:
			}
			continue
		}
		body, readErr := readLimited(resp.Body, c.MaxResponseBytes)
		resp.Body.Close()
		if readErr != nil {
			return ESIResponse{Status: resp.StatusCode, Attempts: attempt + 1}, readErr
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			if resp.StatusCode == 420 || resp.StatusCode == http.StatusTooManyRequests {
				return ESIResponse{Status: resp.StatusCode, RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"), time.Now()), Attempts: attempt + 1}, fmt.Errorf("%w: HTTP %d", ErrESIRetryExhausted, resp.StatusCode)
			}
			return ESIResponse{Status: resp.StatusCode, Attempts: attempt + 1}, fmt.Errorf("ESI HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		cc := resp.Header.Get("Cache-Control")
		_, noCache, noStore := parseCacheControl(cc)
		result := ESIResponse{Status: resp.StatusCode, Body: body, ExpiresAt: expiry(resp.Header, ttl, noCache), ETag: resp.Header.Get("ETag"), CacheControl: cc, Attempts: attempt + 1}
		if !noStore {
			c.mu.Lock()
			if c.cache == nil {
				c.cache = make(map[string]cachedESI)
			}
			c.cache[path] = cachedESI{response: result, cachedAt: time.Now(), noStore: noStore}
			c.mu.Unlock()
		} else {
			c.mu.Lock()
			delete(c.cache, path)
			c.mu.Unlock()
		}
		return result, nil
	}
}
func readLimited(r io.Reader, max int64) ([]byte, error) {
	if max <= 0 {
		max = 8 << 20
	}
	b, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, ErrESIResponseTooLarge
	}
	return b, nil
}
func expiry(h http.Header, fallback time.Duration, noCache bool) time.Time {
	now := time.Now().UTC()
	if noCache {
		return now
	}
	if cc := h.Get("Cache-Control"); cc != "" {
		if age, _, _ := parseCacheControl(cc); age > 0 {
			return now.Add(age)
		}
	}
	if value := h.Get("Expires"); value != "" {
		if t, e := http.ParseTime(value); e == nil {
			return t
		}
	}
	return now.Add(fallback)
}
func (c *ESIClient) GetJSON(ctx context.Context, path string, out any) (ESIResponse, error) {
	r, e := c.Get(ctx, path)
	if e != nil {
		return r, e
	}
	if out != nil && len(bytes.TrimSpace(r.Body)) > 0 {
		if e = json.Unmarshal(r.Body, out); e != nil {
			return r, e
		}
	}
	return r, nil
}
func CharacterPath(id int64) string { return "/latest/characters/" + strconv.FormatInt(id, 10) + "/" }
