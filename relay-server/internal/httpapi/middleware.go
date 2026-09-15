// Package httpapi contains security-focused HTTP building blocks. The package is
// intentionally independent of the relay server and can be composed around any
// net/http handler.
package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	RequestIDHeader = "X-Request-ID"
	MaxBodyBytes    = 1 << 20 // 1 MiB; callers should choose an endpoint-specific limit.
)

// Middleware is the common shape used by all middleware in this package.
type Middleware func(http.Handler) http.Handler

type contextKey uint8

const (
	requestIDKey contextKey = iota
	subjectKey
)

var fallbackRequestID uint64

// RequestID adds a request ID to the request context and response. A caller
// supplied ID is accepted only when it is short and header-safe; otherwise a
// cryptographically random ID is generated. IDs must never contain newlines.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := validRequestID(r.Header.Get(RequestIDHeader))
		if id == "" {
			id = newRequestID()
		}
		r = r.WithContext(context.WithValue(r.Context(), requestIDKey, id))
		w.Header().Set(RequestIDHeader, id)
		next.ServeHTTP(w, r)
	})
}

func validRequestID(id string) string {
	if len(id) == 0 || len(id) > 64 {
		return ""
	}
	for _, c := range id {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' {
			continue
		}
		return ""
	}
	return id
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err == nil {
		return hex.EncodeToString(b[:])
	}
	// This is only an entropy-failure fallback. It remains unique within the
	// process and does not expose request data.
	n := atomic.AddUint64(&fallbackRequestID, 1)
	return strconv.FormatInt(time.Now().UnixNano(), 16) + strconv.FormatUint(n, 16)
}

// RequestIDFromContext returns the middleware-assigned request ID, if any.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// RequestIDFromRequest returns the context ID and, for convenience, falls back
// to the request header. The fallback is useful to handlers in tests, but the
// middleware remains the authority for IDs sent in responses.
func RequestIDFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	if id := RequestIDFromContext(r.Context()); id != "" {
		return id
	}
	return validRequestID(r.Header.Get(RequestIDHeader))
}

// APIError is the stable, machine-readable error payload returned by WriteError.
type APIError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"requestId,omitempty"`
}

type ErrorResponse struct {
	Error APIError `json:"error"`
}

// WriteError writes one consistent JSON error shape. Messages passed by
// callers should be safe, generic text; implementation details belong in logs.
func WriteError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	if status < 400 || status > 599 {
		status = http.StatusInternalServerError
	}
	if code == "" {
		code = "internal_error"
	}
	if message == "" {
		message = http.StatusText(status)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{Error: APIError{
		Code: code, Message: message, RequestID: RequestIDFromRequest(r),
	}})
}

// CORSOptions is an explicit CORS policy. Origins are exact origins, not
// prefixes. An empty allowlist denies every cross-origin request.
type CORSOptions struct {
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	ExposeHeaders    []string
	AllowCredentials bool
	MaxAge           int
}

// CORS applies an allowlist CORS policy. Requests without Origin are treated
// as same-origin/non-browser traffic and pass through unchanged. A denied
// Origin, including a denied preflight, receives a generic 403 response.
func CORS(options CORSOptions) Middleware {
	origins := make(map[string]struct{}, len(options.AllowedOrigins))
	allowAll := false
	for _, raw := range options.AllowedOrigins {
		if strings.TrimSpace(raw) == "*" {
			allowAll = true
			continue
		}
		if origin := normalizeOrigin(raw); origin != "" {
			origins[origin] = struct{}{}
		}
	}
	methods := make([]string, 0, len(options.AllowedMethods))
	for _, method := range options.AllowedMethods {
		if method = strings.ToUpper(strings.TrimSpace(method)); method != "" {
			methods = append(methods, method)
		}
	}
	if len(methods) == 0 {
		methods = []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	}
	methodSet := make(map[string]struct{}, len(methods))
	for _, method := range methods {
		methodSet[method] = struct{}{}
	}
	allowedHeaders := make([]string, 0, len(options.AllowedHeaders))
	headerSet := make(map[string]struct{}, len(options.AllowedHeaders))
	for _, header := range options.AllowedHeaders {
		if header = strings.TrimSpace(header); header != "" {
			allowedHeaders = append(allowedHeaders, header)
			headerSet[strings.ToLower(header)] = struct{}{}
		}
	}
	exposeHeaders := make([]string, 0, len(options.ExposeHeaders))
	for _, header := range options.ExposeHeaders {
		if header = strings.TrimSpace(header); header != "" {
			exposeHeaders = append(exposeHeaders, header)
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rawOrigin := strings.TrimSpace(r.Header.Get("Origin"))
			if rawOrigin == "" {
				next.ServeHTTP(w, r)
				return
			}
			origin := normalizeOrigin(rawOrigin)
			_, allowed := origins[origin]
			if allowAll && !options.AllowCredentials && origin != "" {
				allowed = true
			}
			if !allowed {
				WriteError(w, r, http.StatusForbidden, "origin_denied", "origin is not allowed")
				return
			}
			w.Header().Add("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Origin", rawOrigin)
			if options.AllowCredentials {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			if len(exposeHeaders) > 0 {
				w.Header().Set("Access-Control-Expose-Headers", strings.Join(exposeHeaders, ", "))
			}
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				requestedMethod := strings.ToUpper(strings.TrimSpace(r.Header.Get("Access-Control-Request-Method")))
				if _, ok := methodSet[requestedMethod]; !ok {
					WriteError(w, r, http.StatusForbidden, "method_denied", "method is not allowed")
					return
				}
				requestedHeaders := splitHeaderList(r.Header.Get("Access-Control-Request-Headers"))
				for _, header := range requestedHeaders {
					if _, ok := headerSet[header]; !ok {
						WriteError(w, r, http.StatusForbidden, "header_denied", "header is not allowed")
						return
					}
				}
				w.Header().Set("Access-Control-Allow-Methods", strings.Join(methods, ", "))
				if len(allowedHeaders) > 0 {
					w.Header().Set("Access-Control-Allow-Headers", strings.Join(allowedHeaders, ", "))
				}
				if options.MaxAge > 0 {
					w.Header().Set("Access-Control-Max-Age", strconv.Itoa(options.MaxAge))
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func normalizeOrigin(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "null" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return ""
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host)
}

func splitHeaderList(value string) []string {
	var out []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.ToLower(strings.TrimSpace(item)); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// RateLimiter is a concurrency-safe sliding-window limiter. A separate
// instance can be used for IP and subject limits, and middleware can be
// chained to enforce both dimensions.
type RateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	now     func() time.Time
	entries map[string][]time.Time
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	if limit < 1 {
		limit = 1
	}
	if window <= 0 {
		window = time.Minute
	}
	return &RateLimiter{limit: limit, window: window, now: time.Now, entries: make(map[string][]time.Time)}
}

func (l *RateLimiter) Allow(key string) bool {
	if l == nil {
		return true
	}
	if key == "" {
		key = "anonymous"
	}
	now := l.now()
	cutoff := now.Add(-l.window)
	l.mu.Lock()
	defer l.mu.Unlock()
	old := l.entries[key]
	first := 0
	for first < len(old) && old[first].After(cutoff) == false {
		first++
	}
	old = old[first:]
	if len(old) >= l.limit {
		l.entries[key] = old
		return false
	}
	l.entries[key] = append(old, now)
	return true
}

// RateLimit applies a limiter keyed by the supplied function. The key should
// not contain bearer tokens or other secrets.
func RateLimit(limiter *RateLimiter, key func(*http.Request) string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			k := "anonymous"
			if key != nil {
				k = key(r)
			}
			if limiter != nil && !limiter.Allow(k) {
				w.Header().Set("Retry-After", strconv.FormatInt(int64((limiter.window+time.Second-1)/time.Second), 10))
				WriteError(w, r, http.StatusTooManyRequests, "rate_limited", "too many requests")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func ClientIP(r *http.Request) string {
	if r == nil {
		return "unknown"
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	if strings.TrimSpace(r.RemoteAddr) != "" {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return "unknown"
}

func RateLimitByIP(limiter *RateLimiter) Middleware {
	return RateLimit(limiter, ClientIP)
}

// WithSubject sets an authenticated principal for subject-keyed middleware.
// It should be called by authentication middleware after successful auth.
func WithSubject(ctx context.Context, subject string) context.Context {
	return context.WithValue(ctx, subjectKey, strings.TrimSpace(subject))
}

func Subject(r *http.Request) string {
	if r == nil {
		return ""
	}
	subject, _ := r.Context().Value(subjectKey).(string)
	return subject
}

func RateLimitBySubject(limiter *RateLimiter) Middleware {
	return RateLimit(limiter, Subject)
}

// BodyLimit caps both declared and streamed request bodies. Handlers should
// treat *http.MaxBytesError as a 413 and use WriteError for a uniform response.
func BodyLimit(maxBytes int64) Middleware {
	if maxBytes <= 0 {
		maxBytes = MaxBodyBytes
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > maxBytes {
				WriteError(w, r, http.StatusRequestEntityTooLarge, "body_too_large", "request body is too large")
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}

// LimitBody is an alias kept for callers that prefer verb-first naming.
func LimitBody(maxBytes int64) Middleware { return BodyLimit(maxBytes) }

var sensitiveHeaders = map[string]struct{}{
	"authorization": {}, "proxy-authorization": {}, "cookie": {}, "set-cookie": {},
	"x-api-key": {}, "x-auth-token": {}, "x-csrf-token": {},
}

// RedactHeaders copies headers and replaces credentials/cookies with a fixed
// marker. The input map is never mutated.
func RedactHeaders(headers http.Header) map[string][]string {
	out := make(map[string][]string, len(headers))
	for name, values := range headers {
		copied := append([]string(nil), values...)
		if _, sensitive := sensitiveHeaders[strings.ToLower(name)]; sensitive {
			for i := range copied {
				copied[i] = "[REDACTED]"
			}
		}
		out[name] = copied
	}
	return out
}

var sensitiveQueryKeys = map[string]struct{}{
	"token": {}, "access_token": {}, "refresh_token": {}, "code": {}, "state": {},
	"secret": {}, "password": {}, "key": {}, "api_key": {}, "apikey": {},
}

// RedactURL returns a safe URL string for logs, masking known credential query
// parameters while preserving path and non-sensitive diagnostics.
func RedactURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	copyURL := *u
	// URL userinfo is credential-bearing and must never reach logs.
	copyURL.User = nil
	query := copyURL.Query()
	for key := range query {
		if _, sensitive := sensitiveQueryKeys[strings.ToLower(key)]; sensitive {
			query[key] = []string{"[REDACTED]"}
		}
	}
	copyURL.RawQuery = query.Encode()
	return copyURL.String()
}

// RequestLog is a body-free, redacted representation suitable for structured
// request logging.
type RequestLog struct {
	Method     string              `json:"method"`
	URL        string              `json:"url"`
	RemoteAddr string              `json:"remoteAddr,omitempty"`
	RequestID  string              `json:"requestId,omitempty"`
	Headers    map[string][]string `json:"headers,omitempty"`
}

func RedactRequest(r *http.Request) RequestLog {
	if r == nil {
		return RequestLog{}
	}
	return RequestLog{Method: r.Method, URL: RedactURL(r.URL), RemoteAddr: ClientIP(r), RequestID: RequestIDFromRequest(r), Headers: RedactHeaders(r.Header)}
}
