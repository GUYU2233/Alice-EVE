// Package settings contains the persistence-independent settings domain service.
package settings

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	ScopeAccount Scope = "account"
	ScopeDevice  Scope = "device"
	ScopeProfile Scope = "profile"
)

type Scope string

type Key struct {
	AccountID string
	Scope     Scope
	ScopeID   string
}

func (k Key) valid() error {
	if strings.TrimSpace(k.AccountID) == "" {
		return errors.New("account id is required")
	}
	if k.Scope != ScopeAccount && k.Scope != ScopeDevice && k.Scope != ScopeProfile {
		return ErrInvalidScope
	}
	if k.Scope == ScopeAccount && k.ScopeID != "" {
		return errors.New("account scope cannot have scope id")
	}
	if k.Scope != ScopeAccount && strings.TrimSpace(k.ScopeID) == "" {
		return errors.New("scope id is required")
	}
	return nil
}

type Document struct {
	Key       Key
	Version   int64
	ETag      string
	Values    map[string]any
	UpdatedBy string
	UpdatedAt time.Time
}

type UpdateRequest struct {
	Key         Key
	Values      map[string]any
	BaseVersion int64
	IfMatch     string
	UpdatedBy   string
}

type Policy struct {
	// AllowedKeys is nil/empty for the conservative built-in key policy.
	AllowedKeys    map[string]struct{}
	MaxBytes       int
	MaxDepth       int
	MaxKeys        int
	MaxArrayLength int
}

func NewPolicy(keys ...string) Policy {
	allowed := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		// A blank key is never a meaningful setting name. Ignore it rather than
		// accidentally allowing a whitespace-only JSON property.
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		allowed[key] = struct{}{}
	}
	return Policy{AllowedKeys: allowed}
}

func DefaultPolicy() Policy {
	// Keep this list deliberately small. New settings must be explicitly reviewed
	// and added rather than silently becoming remotely writable.
	return Policy{
		AllowedKeys: NewPolicy(
			"theme", "locale", "timezone", "notifications", "appearance",
			"shortcuts", "sync", "updateChannel", "telemetry",
		).AllowedKeys,
		MaxBytes:       256 * 1024,
		MaxDepth:       8,
		MaxKeys:        128,
		MaxArrayLength: 128,
	}
}

var (
	ErrInvalidScope     = errors.New("invalid settings scope")
	ErrInvalidDocument  = errors.New("invalid settings document")
	ErrSettingsConflict = errors.New("settings version is stale")
	ErrNotFound         = errors.New("settings document not found")
)

type ConflictError struct {
	Current   Document
	MergeHint string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("%v: current version %d", ErrSettingsConflict, e.Current.Version)
}
func (e *ConflictError) Unwrap() error { return ErrSettingsConflict }

type Repository interface {
	Get(context.Context, Key) (Document, error)
	CompareAndSwap(context.Context, Key, int64, string, map[string]any, string, time.Time) (Document, error)
	Reset(context.Context, Key, int64, string, string, time.Time) (Document, error)
}

type Service struct {
	repo   Repository
	policy Policy
	now    func() time.Time
}

func NewService(repo Repository, policy Policy) *Service {
	p := policy
	d := DefaultPolicy()
	if len(p.AllowedKeys) == 0 {
		p.AllowedKeys = d.AllowedKeys
	}
	if p.MaxBytes <= 0 {
		p.MaxBytes = d.MaxBytes
	}
	if p.MaxDepth <= 0 {
		p.MaxDepth = d.MaxDepth
	}
	if p.MaxKeys <= 0 {
		p.MaxKeys = d.MaxKeys
	}
	if p.MaxArrayLength <= 0 {
		p.MaxArrayLength = d.MaxArrayLength
	}
	return &Service{repo: repo, policy: p, now: time.Now}
}
func (s *Service) Get(ctx context.Context, key Key) (Document, error) {
	if err := key.valid(); err != nil {
		return Document{}, err
	}
	return s.repo.Get(ctx, key)
}
func (s *Service) Update(ctx context.Context, req UpdateRequest) (Document, error) {
	if err := req.Key.valid(); err != nil {
		return Document{}, err
	}
	if req.BaseVersion < 1 {
		return Document{}, fmt.Errorf("%w: base version must be positive", ErrInvalidDocument)
	}
	if err := validateDocument(req.Values, s.policy); err != nil {
		return Document{}, err
	}
	current, err := s.repo.Get(ctx, req.Key)
	if err != nil {
		return Document{}, err
	}
	if req.BaseVersion != current.Version || !etagMatches(req.IfMatch, current.ETag) {
		return Document{}, &ConflictError{Current: current, MergeHint: "refresh and merge changed keys before retrying"}
	}
	return s.repo.CompareAndSwap(ctx, req.Key, req.BaseVersion, current.ETag, req.Values, req.UpdatedBy, s.now())
}
func (s *Service) Reset(ctx context.Context, key Key, baseVersion int64, ifMatch, updatedBy string) (Document, error) {
	if err := key.valid(); err != nil {
		return Document{}, err
	}
	if baseVersion < 1 {
		return Document{}, ErrInvalidDocument
	}
	current, err := s.repo.Get(ctx, key)
	if err != nil {
		return Document{}, err
	}
	if baseVersion != current.Version || !etagMatches(ifMatch, current.ETag) {
		return Document{}, &ConflictError{Current: current, MergeHint: "refresh before resetting"}
	}
	return s.repo.Reset(ctx, key, baseVersion, current.ETag, updatedBy, s.now())
}
func etagMatches(got, want string) bool {
	got = strings.TrimSpace(got)
	got = strings.Trim(got, "\"")
	return got != "" && got == want
}

func ETag(version int64, values map[string]any) string {
	b, _ := json.Marshal(values)
	sum := sha256.Sum256(b)
	return fmt.Sprintf("%d-%s", version, hex.EncodeToString(sum[:]))
}

func validateDocument(doc map[string]any, p Policy) error {
	if doc == nil {
		return fmt.Errorf("%w: document must be an object", ErrInvalidDocument)
	}
	b, err := json.Marshal(doc)
	if err != nil || len(b) > p.MaxBytes {
		return fmt.Errorf("%w: document exceeds size limit", ErrInvalidDocument)
	}
	keys, err := validateValue(doc, 1, p)
	if err != nil {
		return err
	}
	if keys > p.MaxKeys {
		return fmt.Errorf("%w: too many keys", ErrInvalidDocument)
	}
	return nil
}
func validateValue(v any, depth int, p Policy) (int, error) {
	if depth > p.MaxDepth {
		return 0, fmt.Errorf("%w: nesting too deep", ErrInvalidDocument)
	}
	switch x := v.(type) {
	case map[string]any:
		n := 0
		for k, child := range x {
			n++
			if forbiddenKey(k) {
				return n, fmt.Errorf("%w: key %q is not allowed", ErrInvalidDocument, k)
			}
			if len(p.AllowedKeys) > 0 {
				root := k
				if depth == 1 {
					if _, ok := p.AllowedKeys[root]; !ok {
						return n, fmt.Errorf("%w: key %q is not whitelisted", ErrInvalidDocument, k)
					}
				}
			}
			childN, err := validateValue(child, depth+1, p)
			n += childN
			if err != nil {
				return n, err
			}
		}
		return n, nil
	case []any:
		if len(x) > p.MaxArrayLength {
			return 0, fmt.Errorf("%w: array exceeds length limit", ErrInvalidDocument)
		}
		n := 0
		for _, child := range x {
			childN, err := validateValue(child, depth+1, p)
			n += childN
			if err != nil {
				return n, err
			}
		}
		return n, nil
	case string:
		if forbiddenString(x) {
			return 0, fmt.Errorf("%w: sensitive value is not allowed", ErrInvalidDocument)
		}
	}
	return 0, nil
}
func forbiddenKey(k string) bool {
	k = strings.ToLower(strings.TrimSpace(k))
	for _, part := range []string{"token", "secret", "private", "password", "credential", "authorization", "cookie"} {
		if strings.Contains(k, part) {
			return true
		}
	}
	return false
}
func forbiddenString(v string) bool {
	l := strings.ToLower(strings.TrimSpace(v))
	if strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://") {
		return true
	}
	if u, err := url.Parse(l); err == nil && u.Scheme != "" && (u.Scheme == "file" || u.Scheme == "javascript" || u.Scheme == "data") {
		return true
	}
	return false
}

type MemoryRepository struct {
	mu   sync.Mutex
	docs map[Key]Document
}

func NewMemoryRepository() *MemoryRepository { return &MemoryRepository{docs: make(map[Key]Document)} }
func cloneValues(in map[string]any) map[string]any {
	b, _ := json.Marshal(in)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return out
}
func (r *MemoryRepository) Get(_ context.Context, key Key) (Document, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.docs[key]
	if !ok {
		d = Document{Key: key, Version: 1, Values: map[string]any{}, ETag: ETag(1, map[string]any{})}
		r.docs[key] = d
	}
	d.Values = cloneValues(d.Values)
	return d, nil
}
func (r *MemoryRepository) CompareAndSwap(_ context.Context, key Key, version int64, tag string, values map[string]any, by string, at time.Time) (Document, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cur, ok := r.docs[key]
	if !ok {
		cur = Document{Key: key, Version: 1, Values: map[string]any{}, ETag: ETag(1, map[string]any{})}
	}
	if cur.Version != version || cur.ETag != tag {
		return Document{}, &ConflictError{Current: cur, MergeHint: "refresh and merge changed keys before retrying"}
	}
	cur.Version++
	cur.Values = cloneValues(values)
	cur.ETag = ETag(cur.Version, cur.Values)
	cur.UpdatedBy = by
	cur.UpdatedAt = at
	r.docs[key] = cur
	cur.Values = cloneValues(cur.Values)
	return cur, nil
}
func (r *MemoryRepository) Reset(_ context.Context, key Key, version int64, tag, by string, at time.Time) (Document, error) {
	return r.CompareAndSwap(context.Background(), key, version, tag, map[string]any{}, by, at)
}
