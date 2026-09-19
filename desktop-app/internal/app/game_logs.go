package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"eve-assistant/desktop-app/internal/protocol"
	"eve-assistant/desktop-app/internal/storage"
)

type GameLogEvent struct {
	ID, Type, Action, Target, Summary string
	Timestamp                         time.Time
	Severity                          string
}
type GameLogStatus struct {
	Directory string    `json:"directory"`
	Running   bool      `json:"running"`
	Files     int       `json:"files"`
	Events    int64     `json:"events"`
	LastEvent time.Time `json:"lastEvent,omitempty"`
	LastError string    `json:"lastError,omitempty"`
}
type gameFileState struct {
	Offset    int64
	Remainder []byte
}
type GameLogWatcher struct {
	mu        sync.Mutex
	store     storage.Store
	directory string
	status    GameLogStatus
	files     map[string]*gameFileState
	recent    []GameLogEvent
	seen      map[string]struct{}
	cancel    context.CancelFunc
}

func NewGameLogWatcher(store storage.Store) *GameLogWatcher {
	return &GameLogWatcher{store: store, files: map[string]*gameFileState{}, seen: map[string]struct{}{}}
}
func DiscoverEVEGameLogs() string {
	chat := DiscoverEVEChatLogs()
	if chat == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(chat), "Gamelogs")
}
func (w *GameLogWatcher) Start(ctx context.Context) error {
	w.mu.Lock()
	if w.directory == "" {
		w.directory = DiscoverEVEGameLogs()
		w.status.Directory = w.directory
	}
	if w.cancel != nil {
		w.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	w.status.Running = true
	dir := w.directory
	w.mu.Unlock()
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		w.Stop()
		return errors.New("EVE Gamelogs directory was not found")
	}
	if err := w.scan(true); err != nil {
		w.Stop()
		return err
	}
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = w.scan(false)
			}
		}
	}()
	return nil
}
func (w *GameLogWatcher) Stop() {
	w.mu.Lock()
	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
	w.status.Running = false
	w.mu.Unlock()
}
func (w *GameLogWatcher) Status() GameLogStatus { w.mu.Lock(); defer w.mu.Unlock(); return w.status }
func (w *GameLogWatcher) Recent() []GameLogEvent {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]GameLogEvent, len(w.recent))
	copy(out, w.recent)
	return out
}
func (w *GameLogWatcher) scan(initial bool) error {
	w.mu.Lock()
	dir := w.directory
	w.mu.Unlock()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	type f struct {
		path string
		mod  time.Time
	}
	var files []f
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".txt") {
			continue
		}
		if i, er := e.Info(); er == nil {
			files = append(files, f{filepath.Join(dir, e.Name()), i.ModTime()})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.After(files[j].mod) })
	if len(files) > 16 {
		files = files[:16]
	}
	for _, f := range files {
		_ = w.readFile(f.path, initial)
	}
	w.mu.Lock()
	w.status.Files = len(files)
	w.mu.Unlock()
	return nil
}
func (w *GameLogWatcher) readFile(path string, initial bool) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	w.mu.Lock()
	state := w.files[path]
	if state == nil {
		state = &gameFileState{}
		if w.store != nil {
			rows, e := w.store.Query(context.Background(), `SELECT offset FROM chat_log_cursor WHERE path=?`, path)
			if e == nil {
				if rows.Next() {
					_ = rows.Scan(&state.Offset)
				}
				_ = rows.Close()
			}
		}
		if initial && state.Offset == 0 {
			state.Offset = info.Size()
		}
		w.files[path] = state
	}
	if info.Size() < state.Offset {
		state.Offset = 0
		state.Remainder = nil
	}
	offset := state.Offset
	rem := append([]byte(nil), state.Remainder...)
	w.mu.Unlock()
	if info.Size() == offset {
		return nil
	}
	if _, err = file.Seek(offset, io.SeekStart); err != nil {
		return err
	}
	data, err := io.ReadAll(io.LimitReader(file, 4<<20))
	if err != nil {
		return err
	}
	combined := append(rem, data...)
	idx := strings.LastIndex(string(combined), "\n")
	if idx < 0 {
		return nil
	}
	text := string(combined[:idx+1])
	remainder := append([]byte(nil), combined[idx+1:]...)
	newOffset := offset + int64(len(data))
	w.mu.Lock()
	state.Offset = newOffset
	state.Remainder = remainder
	w.mu.Unlock()
	if w.store != nil {
		_, _ = w.store.Exec(context.Background(), `INSERT INTO chat_log_cursor(path,offset,size,modified_ms,updated_ms) VALUES(?,?,?,?,?) ON CONFLICT(path) DO UPDATE SET offset=excluded.offset,size=excluded.size,modified_ms=excluded.modified_ms,updated_ms=excluded.updated_ms`, path, newOffset, info.Size(), info.ModTime().UnixMilli(), time.Now().UnixMilli())
	}
	for _, line := range strings.Split(text, "\n") {
		if event, ok := parseGameLogLine(strings.TrimSpace(line)); ok {
			w.add(path, event)
		}
	}
	return nil
}
func parseGameLogLine(line string) (GameLogEvent, bool) {
	match := gameLinePattern.FindStringSubmatch(strings.TrimPrefix(line, "\ufeff"))
	if len(match) != 4 {
		return GameLogEvent{}, false
	}
	ts, err := time.ParseInLocation("2006.01.02 15:04:05", match[1], time.UTC)
	if err != nil {
		return GameLogEvent{}, false
	}
	typ := strings.ToLower(match[2])
	plain := strings.TrimSpace(htmlTagPattern.ReplaceAllString(match[3], ""))
	action, target, severity := "", "", "info"
	if typ == "combat" {
		severity = "warning"
		lower := strings.ToLower(plain)
		switch {
		case strings.Contains(lower, "warp scramble") || strings.Contains(plain, "跃迁扰频") || strings.Contains(plain, "跃迁干扰"):
			action = "warp_scrambled"
			target = plain
		case strings.Contains(lower, " from "):
			action = "under_attack"
			target = strings.TrimSpace(strings.SplitN(plain, " from ", 2)[1])
		case strings.Contains(lower, " misses you"):
			action = "under_attack"
			target = strings.TrimSpace(strings.SplitN(plain, " misses you", 2)[0])
		case strings.Contains(lower, " to "):
			action = "attacking"
			target = strings.TrimSpace(strings.SplitN(plain, " to ", 2)[1])
		default:
			action = "combat"
			target = plain
		}
	} else if typ == "notify" && (strings.Contains(plain, "反跳") || strings.Contains(plain, "扰频") || strings.Contains(strings.ToLower(plain), "warp scramble")) {
		action = "warp_scrambled"
		target = plain
		severity = "danger"
	} else {
		return GameLogEvent{}, false
	}
	target = strings.TrimSpace(strings.SplitN(target, " - ", 2)[0])
	if len([]rune(target)) > 120 {
		target = string([]rune(target)[:120])
	}
	summary := map[string]string{"under_attack": "受到攻击", "attacking": "正在攻击", "warp_scrambled": "遭到跃迁干扰", "combat": "战斗事件"}[action]
	return GameLogEvent{Type: typ, Action: action, Target: target, Summary: summary, Timestamp: ts, Severity: severity}, true
}
func (w *GameLogWatcher) add(path string, event GameLogEvent) {
	sum := sha256.Sum256([]byte(filepath.Base(path) + event.Timestamp.Format(time.RFC3339Nano) + event.Action + event.Target))
	event.ID = hex.EncodeToString(sum[:])
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, ok := w.seen[event.ID]; ok {
		return
	}
	w.seen[event.ID] = struct{}{}
	w.status.Events++
	w.status.LastEvent = event.Timestamp
	w.recent = append([]GameLogEvent{event}, w.recent...)
	if len(w.recent) > 200 {
		w.recent = w.recent[:200]
	}
}
func (e GameLogEvent) Finding() protocol.IntelFinding {
	confidence := .96
	if e.Action == "combat" {
		confidence = .8
	}
	return protocol.IntelFinding{Kind: "combat", Value: e.Target, Stance: "hostile", Source: "eve-gamelog", Summary: e.Summary, ObservedAt: e.Timestamp, ExpiresAt: e.Timestamp.Add(15 * time.Minute), Confidence: confidence}
}

var _ = strconv.Itoa
