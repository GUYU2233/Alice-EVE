package app

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"eve-assistant/desktop-app/internal/protocol"
	"eve-assistant/desktop-app/internal/storage"
)

var chatLinePattern = regexp.MustCompile(`^\[\s*(\d{4}\.\d{2}\.\d{2} \d{2}:\d{2}:\d{2})\s*]\s*([^>]+?)\s*>\s*(.*)$`)
var gameLinePattern = regexp.MustCompile(`^\[\s*(\d{4}\.\d{2}\.\d{2} \d{2}:\d{2}:\d{2})\s*]\s*\(([^)]+)\)\s*(.*)$`)
var htmlTagPattern = regexp.MustCompile(`<[^>]*>`)

type ChatLogStatus struct {
	Directory   string    `json:"directory"`
	Running     bool      `json:"running"`
	Files       int       `json:"files"`
	Messages    int64     `json:"messages"`
	Findings    int64     `json:"findings"`
	LastMessage time.Time `json:"lastMessage,omitempty"`
	LastError   string    `json:"lastError,omitempty"`
}

type ChatIntelEvent struct {
	ID        string                  `json:"id"`
	Channel   string                  `json:"channel"`
	Listener  string                  `json:"listener,omitempty"`
	Author    string                  `json:"author"`
	Timestamp time.Time               `json:"timestamp"`
	Findings  []protocol.IntelFinding `json:"findings"`
}

type chatFileState struct {
	Offset            int64
	Remainder         []byte
	Channel, Listener string
}

type ChatLogWatcher struct {
	mu        sync.Mutex
	store     storage.Store
	directory string
	status    ChatLogStatus
	files     map[string]*chatFileState
	recent    []ChatIntelEvent
	seen      map[string]time.Time
	cancel    context.CancelFunc
}

func NewChatLogWatcher(store storage.Store) *ChatLogWatcher {
	return &ChatLogWatcher{store: store, files: map[string]*chatFileState{}, seen: map[string]time.Time{}}
}

func DiscoverEVEChatLogs() string {
	candidates := []string{}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, "Documents", "EVE", "logs", "Chatlogs"), filepath.Join(home, "OneDrive", "Documents", "EVE", "logs", "Chatlogs"))
	}
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return p
		}
	}
	if len(candidates) > 0 {
		return candidates[0]
	}
	return ""
}

func (w *ChatLogWatcher) SetDirectory(path string) error {
	path = filepath.Clean(strings.TrimSpace(path))
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return errors.New("EVE Chatlogs directory is unavailable")
	}
	w.Stop()
	w.mu.Lock()
	w.directory = path
	w.status.Directory = path
	w.status.LastError = ""
	w.files = map[string]*chatFileState{}
	w.mu.Unlock()
	return nil
}

func (w *ChatLogWatcher) Start(ctx context.Context) error {
	w.mu.Lock()
	if w.directory == "" {
		w.directory = DiscoverEVEChatLogs()
		w.status.Directory = w.directory
	}
	dir := w.directory
	if w.cancel != nil {
		w.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	w.status.Running = true
	w.mu.Unlock()
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		w.Stop()
		return errors.New("EVE Chatlogs directory was not found")
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

func (w *ChatLogWatcher) Stop() {
	w.mu.Lock()
	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
	w.status.Running = false
	w.mu.Unlock()
}
func (w *ChatLogWatcher) Status() ChatLogStatus { w.mu.Lock(); defer w.mu.Unlock(); return w.status }
func (w *ChatLogWatcher) Recent() []ChatIntelEvent {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]ChatIntelEvent, len(w.recent))
	copy(out, w.recent)
	return out
}
func (w *ChatLogWatcher) Find(id string) (ChatIntelEvent, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, event := range w.recent {
		if event.ID == id {
			return event, true
		}
	}
	return ChatIntelEvent{}, false
}

func (w *ChatLogWatcher) scan(initial bool) error {
	w.mu.Lock()
	dir := w.directory
	w.mu.Unlock()
	entries, err := os.ReadDir(dir)
	if err != nil {
		w.setError(err)
		return err
	}
	type candidate struct {
		path string
		mod  time.Time
	}
	var files []candidate
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".txt") {
			continue
		}
		info, err := e.Info()
		if err == nil {
			files = append(files, candidate{filepath.Join(dir, e.Name()), info.ModTime()})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.After(files[j].mod) })
	if len(files) > 64 {
		files = files[:64]
	}
	for _, f := range files {
		if err := w.readFile(f.path, initial); err != nil {
			w.setError(err)
		}
	}
	w.mu.Lock()
	w.status.Files = len(files)
	w.mu.Unlock()
	return nil
}

func (w *ChatLogWatcher) readFile(path string, initial bool) error {
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
		state = &chatFileState{}
		if w.store != nil {
			rows, queryErr := w.store.Query(context.Background(), `SELECT offset FROM chat_log_cursor WHERE path=?`, path)
			if queryErr == nil {
				if rows.Next() {
					_ = rows.Scan(&state.Offset)
				}
				_ = rows.Close()
			}
		}
		if initial && state.Offset == 0 {
			state.Offset = info.Size()
			if headerBytes, readErr := os.ReadFile(path); readErr == nil {
				if len(headerBytes) > 128<<10 {
					headerBytes = headerBytes[:128<<10]
				}
				headerText, _ := decodeChatBytes(headerBytes)
				_, state.Channel, state.Listener = parseChatText(headerText, "", "")
			}
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
	data, err := io.ReadAll(io.LimitReader(file, 2<<20))
	if err != nil {
		return err
	}
	combined := append(rem, data...)
	text, consumed := decodeCompleteChatBytes(combined)
	messages, channel, listener := parseChatText(text, state.Channel, state.Listener)
	w.mu.Lock()
	state.Offset = offset + int64(len(data))
	state.Remainder = append([]byte(nil), combined[consumed:]...)
	if channel != "" {
		state.Channel = channel
	}
	if listener != "" {
		state.Listener = listener
	}
	channel, stateChannel := state.Channel, state.Listener
	newOffset := state.Offset
	w.mu.Unlock()
	if w.store != nil {
		_, _ = w.store.Exec(context.Background(), `INSERT INTO chat_log_cursor(path,offset,size,modified_ms,updated_ms) VALUES(?,?,?,?,?) ON CONFLICT(path) DO UPDATE SET offset=excluded.offset,size=excluded.size,modified_ms=excluded.modified_ms,updated_ms=excluded.updated_ms`, path, newOffset, info.Size(), info.ModTime().UnixMilli(), time.Now().UnixMilli())
	}
	for _, m := range messages {
		w.handleMessage(path, channel, stateChannel, m)
	}
	return nil
}

type parsedChatMessage struct {
	Timestamp    time.Time
	Author, Body string
}

func parseChatText(text, channel, listener string) ([]parsedChatMessage, string, string) {
	var out []parsedChatMessage
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "\ufeff"))
		if strings.Contains(line, "Channel Name") {
			channel = strings.TrimSpace(strings.TrimPrefix(strings.SplitN(line, ":", 2)[1], " "))
		}
		if strings.Contains(line, "Listener") && strings.Contains(line, ":") {
			listener = strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
		}
		match := chatLinePattern.FindStringSubmatch(line)
		if len(match) == 4 {
			ts, err := time.ParseInLocation("2006.01.02 15:04:05", match[1], time.UTC)
			if err == nil {
				out = append(out, parsedChatMessage{ts, strings.TrimSpace(match[2]), match[3]})
			}
		}
	}
	return out, channel, listener
}
func decodeChatBytes(data []byte) (string, bool) {
	if len(data) < 2 {
		return "", false
	}
	utf16le := bytes.HasPrefix(data, []byte{0xff, 0xfe}) || bytes.Count(data, []byte{0}) > len(data)/4
	if !utf16le {
		return string(data), false
	}
	n := len(data) - len(data)%2
	units := make([]uint16, n/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(data[i*2:])
	}
	return string(utf16.Decode(units)), true
}

func decodeCompleteChatBytes(data []byte) (string, int) {
	if len(data) < 2 {
		return "", 0
	}
	utf16le := bytes.HasPrefix(data, []byte{0xff, 0xfe}) || bytes.Count(data, []byte{0}) > len(data)/4
	if utf16le {
		end := 0
		for i := 0; i+1 < len(data); i += 2 {
			if data[i] == '\n' && data[i+1] == 0 {
				end = i + 2
			}
		}
		if end == 0 {
			return "", 0
		}
		units := make([]uint16, end/2)
		for i := range units {
			units[i] = binary.LittleEndian.Uint16(data[i*2:])
		}
		return string(utf16.Decode(units)), end
	}
	idx := bytes.LastIndexByte(data, '\n')
	if idx < 0 {
		return "", 0
	}
	return string(data[:idx+1]), idx + 1
}
func (w *ChatLogWatcher) handleMessage(path, channel, listener string, m parsedChatMessage) {
	findings := ParseIntel(m.Body, "eve-chat:"+channel, m.Timestamp)
	if len(findings) == 0 {
		return
	}
	sum := sha256.Sum256([]byte(filepath.Base(path) + "\x00" + m.Timestamp.Format(time.RFC3339) + "\x00" + m.Author + "\x00" + m.Body))
	id := hex.EncodeToString(sum[:])
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, ok := w.seen[id]; ok {
		return
	}
	w.seen[id] = time.Now()
	w.status.Messages++
	w.status.Findings += int64(len(findings))
	w.status.LastMessage = m.Timestamp
	w.recent = append([]ChatIntelEvent{{ID: id, Channel: channel, Listener: listener, Author: m.Author, Timestamp: m.Timestamp, Findings: findings}}, w.recent...)
	if len(w.recent) > 200 {
		w.recent = w.recent[:200]
	}
}
func (w *ChatLogWatcher) setError(err error) {
	w.mu.Lock()
	w.status.LastError = err.Error()
	w.mu.Unlock()
}
