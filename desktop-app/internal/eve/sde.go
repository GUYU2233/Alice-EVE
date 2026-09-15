package eve

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrInvalidSDEPath      = errors.New("SDE directory must be an existing local directory")
	ErrSDEResourceLimit    = errors.New("SDE resource limit exceeded")
	ErrSDEChecksumMismatch = errors.New("SDE checksum mismatch")
)

const sdeIndexVersion = 1

type SDEEntry struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Kind   string `json:"kind,omitempty"`
	Source string `json:"source,omitempty"`
}
type SDEStatus struct {
	Directory       string    `json:"directory"`
	IndexedAt       time.Time `json:"indexedAt"`
	Files           int       `json:"files"`
	Entries         int       `json:"entries"`
	Ready           bool      `json:"ready"`
	Version         string    `json:"version,omitempty"`
	Source          string    `json:"source,omitempty"`
	Checksum        string    `json:"checksum,omitempty"`
	EntriesChecksum string    `json:"entriesChecksum,omitempty"`
	Signature       string    `json:"signature,omitempty"`
	IndexVersion    int       `json:"indexVersion"`
}

type SDEIndex struct {
	mu             sync.RWMutex
	root, cacheDir string
	entries        []SDEEntry
	status         SDEStatus
	// Resource limits protect the desktop process from unexpectedly large SDEs.
	MaxFileBytes                                 int64
	MaxTotalBytes                                int64
	MaxEntries                                   int
	Version, Source, Signature, metadataChecksum string
}

func NewSDEIndex(cacheDir string) *SDEIndex {
	return &SDEIndex{cacheDir: cacheDir, MaxFileBytes: 64 << 20, MaxTotalBytes: 512 << 20, MaxEntries: 1000000}
}
func (s *SDEIndex) SetMetadata(version, source, checksum, signature string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status.Version, s.status.Source, s.status.Checksum, s.status.Signature = version, source, checksum, signature
	s.Version, s.Source, s.metadataChecksum, s.Signature = version, source, checksum, signature
}
func (s *SDEIndex) Status() SDEStatus { s.mu.RLock(); defer s.mu.RUnlock(); return s.status }
func validateDir(path string) (string, error) {
	if strings.TrimSpace(path) == "" || strings.ContainsRune(path, 0) {
		return "", ErrInvalidSDEPath
	}
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", ErrInvalidSDEPath
	}
	st, err := os.Stat(abs)
	if err != nil || !st.IsDir() {
		return "", ErrInvalidSDEPath
	}
	return abs, nil
}
func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
func (s *SDEIndex) SetDirectory(path string) error {
	root, err := validateDir(path)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.root = root
	s.status = SDEStatus{Directory: root}
	s.entries = nil
	s.mu.Unlock()
	return nil
}
func (s *SDEIndex) Directory() string { s.mu.RLock(); defer s.mu.RUnlock(); return s.root }
func (s *SDEIndex) Reindex(ctx context.Context) (SDEStatus, error) {
	s.mu.RLock()
	root := s.root
	s.mu.RUnlock()
	if root == "" {
		return SDEStatus{}, ErrInvalidSDEPath
	}
	entries := make([]SDEEntry, 0, 1024)
	files, totalBytes := 0, int64(0)
	maxFile, maxTotal, maxEntries := s.MaxFileBytes, s.MaxTotalBytes, s.MaxEntries
	if maxFile <= 0 {
		maxFile = 64 << 20
	}
	if maxTotal <= 0 {
		maxTotal = 512 << 20
	}
	if maxEntries <= 0 {
		maxEntries = 1000000
	}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".json" && ext != ".jsonl" && ext != ".csv" {
			return nil
		}
		real, er := filepath.EvalSymlinks(path)
		if er != nil || !within(root, real) {
			return nil
		}
		info, er := d.Info()
		if er != nil {
			return er
		}
		if info.Size() > maxFile || totalBytes > maxTotal-info.Size() {
			return ErrSDEResourceLimit
		}
		totalBytes += info.Size()
		files++
		got, er := parseFile(path, root)
		if er != nil {
			return er
		}
		if len(got) > maxEntries-len(entries) {
			return ErrSDEResourceLimit
		}
		entries = append(entries, got...)
		return nil
	})
	if err != nil {
		return SDEStatus{}, err
	}
	sort.Slice(entries, func(i, j int) bool { return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name) })
	now := time.Now().UTC()
	s.mu.RLock()
	version, source, signature, declaredChecksum := s.Version, s.Source, s.Signature, s.metadataChecksum
	s.mu.RUnlock()
	entriesChecksum := checksumEntries(entries)
	checksum := declaredChecksum
	st := SDEStatus{Directory: root, IndexedAt: now, Files: files, Entries: len(entries), Ready: true, Version: version, Source: source, Checksum: checksum, EntriesChecksum: entriesChecksum, Signature: signature, IndexVersion: sdeIndexVersion}
	s.mu.Lock()
	s.entries = entries
	s.status = st
	s.mu.Unlock()
	if err := s.persist(); err != nil {
		return st, err
	}
	return st, nil
}
func parseFile(path, root string) ([]SDEEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	rel, _ := filepath.Rel(root, path)
	switch strings.ToLower(filepath.Ext(path)) {
	case ".csv":
		return parseCSV(f, rel)
	case ".jsonl":
		return parseJSONL(f, rel)
	default:
		b, er := io.ReadAll(io.LimitReader(f, 64<<20))
		if er != nil {
			return nil, er
		}
		return parseJSON(b, rel)
	}
}
func parseCSV(r io.Reader, src string) ([]SDEEntry, error) {
	cr := csv.NewReader(r)
	hdr, err := cr.Read()
	if err != nil {
		return nil, err
	}
	for i := range hdr {
		hdr[i] = strings.ToLower(strings.TrimSpace(hdr[i]))
	}
	idix, nameix := -1, -1
	for i, h := range hdr {
		if h == "id" || strings.HasSuffix(h, "id") || h == "typeid" {
			if idix < 0 {
				idix = i
			}
		}
		if h == "name" || strings.HasSuffix(h, "name") {
			if nameix < 0 {
				nameix = i
			}
		}
	}
	if nameix < 0 {
		return nil, nil
	}
	out := []SDEEntry{}
	for {
		row, e := cr.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, e
		}
		if len(row) <= nameix {
			continue
		}
		name := strings.TrimSpace(row[nameix])
		if name == "" {
			continue
		}
		id := ""
		if idix >= 0 && len(row) > idix {
			id = strings.TrimSpace(row[idix])
		}
		out = append(out, SDEEntry{ID: id, Name: name, Source: src})
	}
	return out, nil
}
func parseJSONL(r io.Reader, src string) ([]SDEEntry, error) {
	sc := bufio.NewScanner(io.LimitReader(r, 64<<20))
	sc.Buffer(make([]byte, 4096), 2<<20)
	out := []SDEEntry{}
	for sc.Scan() {
		x, er := parseJSON(sc.Bytes(), src)
		if er != nil {
			continue
		}
		out = append(out, x...)
	}
	return out, sc.Err()
}
func parseJSON(b []byte, src string) ([]SDEEntry, error) {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	out := []SDEEntry{}
	walkJSON(v, "", src, &out)
	return out, nil
}
func localized(v any) string {
	if m, ok := v.(map[string]any); ok {
		for _, k := range []string{"en", "en-us", "english"} {
			if x, ok := m[k].(string); ok {
				return x
			}
		}
		for _, x := range m {
			if z, ok := x.(string); ok && z != "" {
				return z
			}
		}
	}
	if x, ok := v.(string); ok {
		return x
	}
	return ""
}
func walkJSON(v any, key, src string, out *[]SDEEntry) {
	switch x := v.(type) {
	case []any:
		for _, z := range x {
			walkJSON(z, "", src, out)
		}
	case map[string]any:
		name := ""
		for _, k := range []string{"name", "Name", "typeName", "solarSystemName"} {
			if z, ok := x[k]; ok {
				name = localized(z)
				if name != "" {
					break
				}
			}
		}
		id := ""
		for _, k := range []string{"id", "ID", "typeID", "typeId", "itemID", "solarSystemID"} {
			if z, ok := x[k]; ok {
				id = fmt.Sprint(z)
				break
			}
		}
		if name == "" && key != "" && !strings.ContainsAny(key, "{}[]") {
			name = localized(v)
		}
		if name != "" {
			if id == "" {
				id = key
			}
			*out = append(*out, SDEEntry{ID: id, Name: name, Source: src})
		}
		for k, z := range x {
			walkJSON(z, k, src, out)
		}
	default:
		if key != "" {
			if z := localized(v); z != "" {
				*out = append(*out, SDEEntry{ID: key, Name: z, Source: src})
			}
		}
	}
}
func (s *SDEIndex) Query(query string, limit int) []SDEEntry {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]SDEEntry, 0, limit)
	seen := map[string]bool{}
	for _, e := range s.entries {
		if strings.Contains(strings.ToLower(e.Name), q) || strings.EqualFold(e.ID, q) {
			k := e.ID + "|" + e.Name
			if !seen[k] {
				seen[k] = true
				out = append(out, e)
			}
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}
func checksumEntries(entries []SDEEntry) string {
	h := sha256.New()
	for _, e := range entries {
		_, _ = io.WriteString(h, e.ID+"\x00"+e.Name+"\x00"+e.Kind+"\x00"+e.Source+"\n")
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (s *SDEIndex) persist() error {
	s.mu.RLock()
	root, cache := s.root, s.cacheDir
	entries := s.entries
	s.mu.RUnlock()
	if cache == "" {
		return nil
	}
	if err := os.MkdirAll(cache, 0700); err != nil {
		return err
	}
	key := sha256.Sum256([]byte(root))
	p := filepath.Join(cache, "sde-"+hex.EncodeToString(key[:])+".json")
	tmp := p + ".tmp"
	b, err := json.Marshal(struct {
		IndexVersion int        `json:"indexVersion"`
		Status       SDEStatus  `json:"status"`
		Entries      []SDEEntry `json:"entries"`
	}{sdeIndexVersion, s.status, entries})
	if err != nil {
		return err
	}
	if err = os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
func (s *SDEIndex) LoadCached(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var v struct {
		IndexVersion int        `json:"indexVersion"`
		Status       SDEStatus  `json:"status"`
		Entries      []SDEEntry `json:"entries"`
	}
	if err = json.Unmarshal(b, &v); err != nil {
		return err
	}
	if v.IndexVersion != 0 && v.IndexVersion != sdeIndexVersion {
		return fmt.Errorf("unsupported SDE index version %d", v.IndexVersion)
	}
	root, e := validateDir(v.Status.Directory)
	if e != nil {
		return e
	}
	if len(v.Entries) > 1000000 {
		return ErrSDEResourceLimit
	}
	if v.Status.EntriesChecksum != "" && v.Status.EntriesChecksum != checksumEntries(v.Entries) {
		return ErrSDEChecksumMismatch
	}
	v.Status.IndexVersion = sdeIndexVersion
	s.mu.Lock()
	s.root = root
	s.status = v.Status
	s.entries = v.Entries
	s.Version, s.Source, s.metadataChecksum, s.Signature = v.Status.Version, v.Status.Source, v.Status.Checksum, v.Status.Signature
	s.mu.Unlock()
	return nil
}
func (s *SDEIndex) CachePath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cacheDir == "" || s.root == "" {
		return ""
	}
	key := sha256.Sum256([]byte(s.root))
	return filepath.Join(s.cacheDir, "sde-"+hex.EncodeToString(key[:])+".json")
}
