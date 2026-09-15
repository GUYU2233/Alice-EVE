// Package updates contains update manifest validation and release selection rules.
package updates

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type SignatureMetadata struct {
	Algorithm string `json:"algorithm"`
	KeyID     string `json:"keyId"`
	Value     string `json:"value"`
}

type Manifest struct {
	ReleaseID           string            `json:"releaseId"`
	App                 string            `json:"app"`
	Version             string            `json:"version"`
	Platform            string            `json:"platform"`
	Arch                string            `json:"arch"`
	Channel             string            `json:"channel"`
	MinSupportedVersion string            `json:"minSupportedVersion,omitempty"`
	Mandatory           bool              `json:"mandatory"`
	ArtifactURL         string            `json:"artifactUrl"`
	SHA256              string            `json:"sha256"`
	Signature           SignatureMetadata `json:"signature"`
	PublishedAt         time.Time         `json:"publishedAt"`
	WithdrawnAt         *time.Time        `json:"withdrawnAt,omitempty"`
	Metadata            map[string]string `json:"metadata,omitempty"`
}

type Query struct {
	App, Platform, Arch, Channel, CurrentVersion string
	AllowRollback                                bool
}

type Selection struct {
	Manifest  Manifest
	Available bool
	Rollback  bool
}

type Ack struct {
	ReleaseID string
	AccountID string
	DeviceID  string
	Status    string
	ErrorCode string
	At        time.Time
}

type Repository interface {
	List(context.Context, string, string, string, string) ([]Manifest, error)
	RecordAck(context.Context, Ack) error
}

type Service struct {
	repo Repository
	now  func() time.Time
}

func NewService(repo Repository) *Service { return &Service{repo: repo, now: time.Now} }

var (
	ErrInvalidManifest = errors.New("invalid update manifest")
	ErrNoUpdate        = errors.New("no applicable update")
)

func ValidateManifest(m Manifest) error {
	if strings.TrimSpace(m.ReleaseID) == "" || strings.TrimSpace(m.App) == "" || strings.TrimSpace(m.Version) == "" || strings.TrimSpace(m.Platform) == "" || strings.TrimSpace(m.Arch) == "" || strings.TrimSpace(m.Channel) == "" {
		return fmt.Errorf("%w: required field missing", ErrInvalidManifest)
	}
	if _, err := ParseVersion(m.Version); err != nil {
		return fmt.Errorf("%w: version: %v", ErrInvalidManifest, err)
	}
	if m.MinSupportedVersion != "" {
		if _, err := ParseVersion(m.MinSupportedVersion); err != nil {
			return fmt.Errorf("%w: min supported version: %v", ErrInvalidManifest, err)
		}
	}
	if !strings.HasPrefix(strings.ToLower(m.ArtifactURL), "https://") {
		return fmt.Errorf("%w: artifact URL must use https", ErrInvalidManifest)
	}
	if len(m.SHA256) != 64 {
		return fmt.Errorf("%w: sha256 must be 64 hex characters", ErrInvalidManifest)
	}
	if _, err := hex.DecodeString(m.SHA256); err != nil {
		return fmt.Errorf("%w: sha256: %v", ErrInvalidManifest, err)
	}
	if strings.TrimSpace(m.Signature.Algorithm) == "" || strings.TrimSpace(m.Signature.KeyID) == "" || strings.TrimSpace(m.Signature.Value) == "" {
		return fmt.Errorf("%w: signature metadata incomplete", ErrInvalidManifest)
	}
	if m.PublishedAt.IsZero() {
		return fmt.Errorf("%w: publishedAt required", ErrInvalidManifest)
	}
	return nil
}

func (s *Service) Manifest(ctx context.Context, q Query) (Selection, error) {
	if strings.TrimSpace(q.App) == "" || strings.TrimSpace(q.Platform) == "" || strings.TrimSpace(q.Arch) == "" || strings.TrimSpace(q.Channel) == "" {
		return Selection{}, ErrNoUpdate
	}
	if q.CurrentVersion != "" {
		if _, err := ParseVersion(q.CurrentVersion); err != nil {
			return Selection{}, fmt.Errorf("%w: current version: %v", ErrNoUpdate, err)
		}
	}
	list, err := s.repo.List(ctx, q.App, q.Platform, q.Arch, q.Channel)
	if err != nil {
		return Selection{}, err
	}
	candidates := make([]Manifest, 0, len(list))
	for _, m := range list {
		if ValidateManifest(m) != nil || m.WithdrawnAt != nil {
			continue
		}
		if q.CurrentVersion != "" && m.MinSupportedVersion != "" {
			min, _ := ParseVersion(m.MinSupportedVersion)
			current, _ := ParseVersion(q.CurrentVersion)
			if current.Less(min) && !m.Mandatory {
				continue
			}
		}
		candidates = append(candidates, m)
	}
	if len(candidates) == 0 {
		return Selection{}, ErrNoUpdate
	}
	sort.Slice(candidates, func(i, j int) bool {
		a, _ := ParseVersion(candidates[i].Version)
		b, _ := ParseVersion(candidates[j].Version)
		return a.Less(b)
	})
	current, _ := ParseVersion(q.CurrentVersion)
	selected := candidates[len(candidates)-1]
	rollback := false
	if q.CurrentVersion != "" {
		if v, _ := ParseVersion(selected.Version); v.Compare(current) <= 0 {
			if !q.AllowRollback {
				return Selection{}, ErrNoUpdate
			}
			// Select the highest release below the installed version for rollback.
			idx := -1
			for i, m := range candidates {
				v, _ := ParseVersion(m.Version)
				if v.Compare(current) < 0 {
					idx = i
				}
			}
			if idx < 0 {
				return Selection{}, ErrNoUpdate
			}
			selected = candidates[idx]
			rollback = true
		} else if min, ok := ParseVersion(selected.MinSupportedVersion); ok == nil && current.Less(min) && !selected.Mandatory {
			return Selection{}, ErrNoUpdate
		}
	}
	return Selection{Manifest: selected, Available: true, Rollback: rollback}, nil
}
func (s *Service) Ack(ctx context.Context, ack Ack) error {
	if strings.TrimSpace(ack.ReleaseID) == "" || strings.TrimSpace(ack.AccountID) == "" || strings.TrimSpace(ack.DeviceID) == "" || strings.TrimSpace(ack.Status) == "" {
		return errors.New("release, account, device and status are required")
	}
	if ack.At.IsZero() {
		ack.At = s.now()
	}
	return s.repo.RecordAck(ctx, ack)
}

type Version struct {
	Major, Minor, Patch int
	Pre                 string
}

func ParseVersion(raw string) (Version, error) {
	raw = strings.TrimSpace(strings.TrimPrefix(raw, "v"))
	parts := strings.SplitN(raw, "-", 2)
	nums := strings.Split(parts[0], ".")
	if len(nums) != 3 {
		return Version{}, errors.New("expected major.minor.patch")
	}
	vals := [3]int{}
	for i := range nums {
		n, e := strconv.Atoi(nums[i])
		if e != nil || n < 0 {
			return Version{}, errors.New("invalid numeric version")
		}
		vals[i] = n
	}
	v := Version{Major: vals[0], Minor: vals[1], Patch: vals[2]}
	if len(parts) == 2 {
		v.Pre = parts[1]
	}
	return v, nil
}
func (v Version) Compare(o Version) int {
	if v.Major != o.Major {
		if v.Major < o.Major {
			return -1
		}
		return 1
	}
	if v.Minor != o.Minor {
		if v.Minor < o.Minor {
			return -1
		}
		return 1
	}
	if v.Patch != o.Patch {
		if v.Patch < o.Patch {
			return -1
		}
		return 1
	}
	if v.Pre == o.Pre {
		return 0
	}
	if v.Pre == "" {
		return 1
	}
	if o.Pre == "" {
		return -1
	}
	if v.Pre < o.Pre {
		return -1
	}
	return 1
}
func (v Version) Less(o Version) bool { return v.Compare(o) < 0 }

type MemoryRepository struct {
	mu        sync.Mutex
	manifests []Manifest
	acks      []Ack
}

func NewMemoryRepository() *MemoryRepository { return &MemoryRepository{} }
func (r *MemoryRepository) Add(m Manifest) error {
	if err := ValidateManifest(m); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, x := range r.manifests {
		if x.ReleaseID == m.ReleaseID {
			return errors.New("release already exists")
		}
		if x.App == m.App && x.Version == m.Version && x.Platform == m.Platform && x.Arch == m.Arch && x.Channel == m.Channel {
			return errors.New("release version already exists")
		}
	}
	r.manifests = append(r.manifests, m)
	return nil
}
func (r *MemoryRepository) List(_ context.Context, app, platform, arch, channel string) ([]Manifest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []Manifest{}
	for _, m := range r.manifests {
		if m.App == app && m.Platform == platform && m.Arch == arch && m.Channel == channel {
			out = append(out, m)
		}
	}
	return out, nil
}
func (r *MemoryRepository) RecordAck(_ context.Context, a Ack) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.acks = append(r.acks, a)
	return nil
}
func (r *MemoryRepository) Acks() []Ack {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Ack(nil), r.acks...)
}
