package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"eve-assistant/desktop-app/internal/ingest"
	"eve-assistant/desktop-app/internal/storage"
)

type localCursorAdapter struct {
	store      storage.Store
	sourceKind string
	mu         sync.Mutex
	meta       map[string]storage.LogCursor
	pending    map[string]storage.NormalizedLocalEvent
}

func newLocalCursorAdapter(store storage.Store, kind string) *localCursorAdapter {
	return &localCursorAdapter{store: store, sourceKind: kind, meta: map[string]storage.LogCursor{}, pending: map[string]storage.NormalizedLocalEvent{}}
}
func (c *localCursorAdapter) Load(ctx context.Context, key string) (int64, bool, error) {
	cursor, found, err := storage.LoadLogCursor(ctx, c.store, c.sourceKind, key)
	if err != nil {
		return 0, false, err
	}
	c.mu.Lock()
	observed := c.meta[key]
	if found && observed.FileIdentity != "" && cursor.FileIdentity != observed.FileIdentity {
		observed.Generation = cursor.Generation + 1
		observed.CommittedOffset = 0
		c.meta[key] = observed
		c.mu.Unlock()
		return 0, false, nil
	}
	if found {
		cursor.Size, cursor.ModifiedMS = observed.Size, observed.ModifiedMS
		c.meta[key] = cursor
		c.mu.Unlock()
		return cursor.CommittedOffset, true, nil
	}
	c.mu.Unlock()
	return 0, false, nil
}
func (c *localCursorAdapter) Commit(ctx context.Context, key string, offset int64) error {
	c.mu.Lock()
	cursor := c.meta[key]
	event, hasEvent := c.pending[key]
	delete(c.pending, key)
	c.mu.Unlock()
	cursor.SourceKind = c.sourceKind
	cursor.Path = key
	cursor.CommittedOffset = offset
	cursor.UpdatedMS = time.Now().UnixMilli()
	events := []storage.NormalizedLocalEvent{}
	if hasEvent {
		events = append(events, event)
	}
	if err := storage.CommitNormalizedEvents(ctx, c.store, events, cursor); err != nil {
		return err
	}
	c.mu.Lock()
	c.meta[key] = cursor
	c.mu.Unlock()
	return nil
}
func (c *localCursorAdapter) setFile(key, identity, encoding string, size, modified int64) {
	c.mu.Lock()
	cur := c.meta[key]
	cur.SourceKind = c.sourceKind
	cur.Path = key
	cur.FileIdentity = identity
	cur.Encoding = encoding
	cur.Size = size
	cur.ModifiedMS = modified
	cur.UpdatedMS = time.Now().UnixMilli()
	c.meta[key] = cur
	c.mu.Unlock()
}
func (c *localCursorAdapter) setPending(key string, event storage.NormalizedLocalEvent) {
	c.mu.Lock()
	c.pending[key] = event
	c.mu.Unlock()
}

type LocalIngestion struct {
	store                  storage.Store
	chat, game             *ingest.Tailer
	chatCursor, gameCursor *localCursorAdapter
	mu                     sync.Mutex
	chatMeta               map[string]chatHeader
	recentChat             []ChatIntelEvent
	recentGame             []GameLogEvent
	status                 map[string]any
}
type chatHeader struct{ Channel, Listener string }

func NewLocalIngestion(store storage.Store) (*LocalIngestion, error) {
	l := &LocalIngestion{store: store, chatMeta: map[string]chatHeader{}, status: map[string]any{}}
	chatDir := DiscoverEVEChatLogs()
	gameDir := DiscoverEVEGameLogs()
	l.chatCursor = newLocalCursorAdapter(store, "eve_chat")
	l.gameCursor = newLocalCursorAdapter(store, "eve_game")
	if chatDir != "" {
		_ = storage.UpsertLogSource(context.Background(), store, storage.LogSource{SourceKind: "eve_chat", RootPath: chatDir, UpdatedMS: time.Now().UnixMilli()})
		tail, err := ingest.New(ingest.Config{Directory: chatDir, PollInterval: time.Second, Selector: l.chatSelector, Handler: l.handleChat, Cursor: l.chatCursor, Initial: ingest.ResumeIfPresentElseEnd, MaxReadBytes: 2 << 20, MaxRecordsScan: 5000})
		if err != nil {
			return nil, err
		}
		l.chat = tail
	}
	if gameDir != "" {
		_ = storage.UpsertLogSource(context.Background(), store, storage.LogSource{SourceKind: "eve_game", RootPath: gameDir, UpdatedMS: time.Now().UnixMilli()})
		tail, err := ingest.New(ingest.Config{Directory: gameDir, PollInterval: time.Second, Selector: l.gameSelector, Handler: l.handleGame, Cursor: l.gameCursor, Initial: ingest.ResumeIfPresentElseEnd, MaxReadBytes: 4 << 20, MaxRecordsScan: 10000})
		if err != nil {
			return nil, err
		}
		l.game = tail
	}
	return l, nil
}
func (l *LocalIngestion) Start(ctx context.Context) {
	if l.chat != nil {
		_ = l.chat.Start(ctx)
	}
	if l.game != nil {
		_ = l.game.Start(ctx)
	}
}
func (l *LocalIngestion) Stop() {
	if l.chat != nil {
		l.chat.Stop()
	}
	if l.game != nil {
		l.game.Stop()
	}
}
func fileIdentity(path string, _ fs.FileInfo) string {
	// EVE filenames include the immutable session timestamp and character ID.
	// Content, size, and mtime all change while the client appends.
	sum := sha256.Sum256([]byte(filepath.Clean(path)))
	return hex.EncodeToString(sum[:])
}
func (l *LocalIngestion) chatSelector(path string, entry fs.DirEntry) (ingest.File, bool) {
	meta, ok := matchChatLogFilename(path)
	if !ok || time.Since(meta.Session) > 7*24*time.Hour {
		return ingest.File{}, false
	}
	info, err := entry.Info()
	if err != nil {
		return ingest.File{}, false
	}
	identity := fileIdentity(path, info)
	l.chatCursor.setFile(path, identity, "utf16le", info.Size(), info.ModTime().UnixMilli())
	_ = storage.UpsertLogFile(context.Background(), l.store, storage.LogFile{SourceKind: "eve_chat", Path: path, FileIdentity: identity, Size: info.Size(), ModifiedMS: info.ModTime().UnixMilli(), UpdatedMS: time.Now().UnixMilli()})
	l.ensureChatHeader(path)
	return ingest.File{Key: path, Encoding: ingest.UTF16LE}, true
}
func (l *LocalIngestion) gameSelector(path string, entry fs.DirEntry) (ingest.File, bool) {
	meta, ok := matchGameLogFilename(path)
	if !ok || time.Since(meta.Session) > 7*24*time.Hour {
		return ingest.File{}, false
	}
	info, err := entry.Info()
	if err != nil {
		return ingest.File{}, false
	}
	identity := fileIdentity(path, info)
	l.gameCursor.setFile(path, identity, "utf8", info.Size(), info.ModTime().UnixMilli())
	_ = storage.UpsertLogFile(context.Background(), l.store, storage.LogFile{SourceKind: "eve_game", Path: path, FileIdentity: identity, Size: info.Size(), ModifiedMS: info.ModTime().UnixMilli(), UpdatedMS: time.Now().UnixMilli()})
	return ingest.File{Key: path, Encoding: ingest.UTF8}, true
}
func (l *LocalIngestion) ensureChatHeader(path string) {
	l.mu.Lock()
	_, ok := l.chatMeta[path]
	l.mu.Unlock()
	if ok {
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	if len(b) > 128<<10 {
		b = b[:128<<10]
	}
	text, _ := decodeChatBytes(b)
	_, channel, listener := parseChatText(text, "", "")
	l.mu.Lock()
	l.chatMeta[path] = chatHeader{channel, listener}
	l.mu.Unlock()
}
func stableRecordID(kind, path, identity string, start, end int64) string {
	sum := sha256.Sum256([]byte(kind + "\x00" + path + "\x00" + identity + "\x00" + strconv.FormatInt(start, 10) + "\x00" + strconv.FormatInt(end, 10)))
	return hex.EncodeToString(sum[:])
}
func (l *LocalIngestion) handleChat(ctx context.Context, r ingest.Record, commit ingest.CommitFunc) error {
	match := chatLinePattern.FindStringSubmatch(strings.TrimPrefix(r.Text, "\ufeff"))
	if len(match) != 4 {
		return commit()
	}
	ts, err := time.ParseInLocation("2006.01.02 15:04:05", match[1], time.UTC)
	if err != nil {
		return commit()
	}
	author, body := strings.TrimSpace(match[2]), match[3]
	findings := ParseIntel(body, "eve-chat", ts)
	l.chatCursor.mu.Lock()
	cur := l.chatCursor.meta[r.File.Key]
	l.chatCursor.mu.Unlock()
	payload, _ := json.Marshal(map[string]any{"author": author, "findings": findings})
	id := stableRecordID("eve_chat", r.Path, cur.FileIdentity, r.Start, r.End)
	l.chatCursor.setPending(r.File.Key, storage.NormalizedLocalEvent{ID: id, SourceKind: "eve_chat", SourcePath: r.Path, FileIdentity: cur.FileIdentity, RecordStart: r.Start, RecordEnd: r.End, EventType: "chat.message", ObservedMS: ts.UnixMilli(), PayloadJSON: string(payload), CreatedMS: time.Now().UnixMilli()})
	if err := commit(); err != nil {
		return err
	}
	if len(findings) > 0 {
		l.mu.Lock()
		meta := l.chatMeta[r.Path]
		l.recentChat = append([]ChatIntelEvent{{ID: id, Channel: meta.Channel, Listener: meta.Listener, Author: author, Timestamp: ts, Findings: findings}}, l.recentChat...)
		if len(l.recentChat) > 200 {
			l.recentChat = l.recentChat[:200]
		}
		l.mu.Unlock()
	}
	return nil
}
func (l *LocalIngestion) handleGame(ctx context.Context, r ingest.Record, commit ingest.CommitFunc) error {
	event, ok := parseGameLogLine(strings.TrimSpace(r.Text))
	if !ok {
		return commit()
	}
	l.gameCursor.mu.Lock()
	cur := l.gameCursor.meta[r.File.Key]
	l.gameCursor.mu.Unlock()
	event.ID = stableRecordID("eve_game", r.Path, cur.FileIdentity, r.Start, r.End)
	payload, _ := json.Marshal(event)
	l.gameCursor.setPending(r.File.Key, storage.NormalizedLocalEvent{ID: event.ID, SourceKind: "eve_game", SourcePath: r.Path, FileIdentity: cur.FileIdentity, RecordStart: r.Start, RecordEnd: r.End, EventType: "game." + event.Action, ObservedMS: event.Timestamp.UnixMilli(), PayloadJSON: string(payload), CreatedMS: time.Now().UnixMilli()})
	if err := commit(); err != nil {
		return err
	}
	l.mu.Lock()
	l.recentGame = append([]GameLogEvent{event}, l.recentGame...)
	if len(l.recentGame) > 200 {
		l.recentGame = l.recentGame[:200]
	}
	l.mu.Unlock()
	return nil
}
func (l *LocalIngestion) ChatStatus() ChatLogStatus {
	l.mu.Lock()
	defer l.mu.Unlock()
	status := ChatLogStatus{Directory: DiscoverEVEChatLogs(), Running: l.chat != nil, Findings: int64(len(l.recentChat))}
	if len(l.recentChat) > 0 {
		status.LastMessage = l.recentChat[0].Timestamp
	}
	return status
}
func (l *LocalIngestion) GameStatus() GameLogStatus {
	l.mu.Lock()
	defer l.mu.Unlock()
	status := GameLogStatus{Directory: DiscoverEVEGameLogs(), Running: l.game != nil, Events: int64(len(l.recentGame))}
	if len(l.recentGame) > 0 {
		status.LastEvent = l.recentGame[0].Timestamp
	}
	return status
}
func (l *LocalIngestion) RecentChat() []ChatIntelEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := append([]ChatIntelEvent(nil), l.recentChat...)
	return out
}
func (l *LocalIngestion) RecentGame() []GameLogEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := append([]GameLogEvent(nil), l.recentGame...)
	return out
}
func (l *LocalIngestion) StoredEvents(kind string, limit int) ([]storage.NormalizedLocalEvent, error) {
	return storage.ListNormalizedLocalEvents(context.Background(), l.store, kind, limit)
}

var _ = filepath.Clean
var _ = sort.Slice
