package app

import (
	"context"
	"database/sql"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
	"unicode/utf16"

	"eve-assistant/desktop-app/internal/ingest"
	"eve-assistant/desktop-app/internal/storage"
	_ "modernc.org/sqlite"
)

func ingestionStore(t *testing.T) storage.Store {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "ingest.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	s := storage.NewSQLiteStore(db)
	if err = storage.EnsureReliableSchema(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
func appendUTF16(t *testing.T, path, text string) {
	t.Helper()
	units := utf16.Encode([]rune(text))
	b := make([]byte, len(units)*2)
	for i, u := range units {
		binary.LittleEndian.PutUint16(b[i*2:], u)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.Write(b); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
}
func waitUntil(t *testing.T, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("condition timeout")
}
func newTestTailer(dir string, selector ingest.Selector, handler ingest.Handler, cursor ingest.Cursor) (*ingest.Tailer, error) {
	return ingest.New(ingest.Config{Directory: dir, PollInterval: 25 * time.Millisecond, Selector: selector, Handler: handler, Cursor: cursor, Initial: ingest.ResumeIfPresentElseEnd, MaxReadBytes: 1 << 20, MaxRecordsScan: 1000})
}

func TestLocalIngestionPersistsChatAndGameAcrossRestart(t *testing.T) {
	root := t.TempDir()
	chatDir := filepath.Join(root, "Chatlogs")
	gameDir := filepath.Join(root, "Gamelogs")
	_ = os.MkdirAll(chatDir, 0700)
	_ = os.MkdirAll(gameDir, 0700)
	chatPath := filepath.Join(chatDir, "Intel_20260916_043250_2115507479.txt")
	gamePath := filepath.Join(gameDir, "20260916_043250_2115507479.txt")
	header := "---------------------------------------------------------------\r\n Channel Name: Intel\r\n Listener: Alice\r\n---------------------------------------------------------------\r\n"
	raw := []byte{0xff, 0xfe}
	units := utf16.Encode([]rune(header))
	for _, u := range units {
		raw = append(raw, byte(u), byte(u>>8))
	}
	if err := os.WriteFile(chatPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gamePath, []byte("header\n"), 0600); err != nil {
		t.Fatal(err)
	}
	store := ingestionStore(t)
	l := &LocalIngestion{store: store, chatMeta: map[string]chatHeader{}, status: map[string]any{}, chatCursor: newLocalCursorAdapter(store, "eve_chat"), gameCursor: newLocalCursorAdapter(store, "eve_game")}
	var err error
	l.chat, err = newTestTailer(chatDir, l.chatSelector, l.handleChat, l.chatCursor)
	if err != nil {
		t.Fatal(err)
	}
	l.game, err = newTestTailer(gameDir, l.gameSelector, l.handleGame, l.gameCursor)
	if err != nil {
		t.Fatal(err)
	}
	l.Start(t.Context())
	defer l.Stop()
	appendUTF16(t, chatPath, "[ 2026.09.16 04:52:30 ] Scout > hostile Drake in Jita\r\n")
	f, _ := os.OpenFile(gamePath, os.O_APPEND|os.O_WRONLY, 0600)
	_, _ = f.WriteString("[ 2026.09.16 04:52:31 ] (combat) <b>125</b> from <b>Hostile Pilot</b> - Hits\n")
	_ = f.Close()
	waitUntil(t, func() bool {
		a, _ := storage.ListNormalizedLocalEvents(context.Background(), store, "eve_chat", 10)
		b, _ := storage.ListNormalizedLocalEvents(context.Background(), store, "eve_game", 10)
		return len(a) == 1 && len(b) == 1
	})
	l.Stop()
	chatBefore, _ := storage.ListNormalizedLocalEvents(context.Background(), store, "eve_chat", 10)
	gameBefore, _ := storage.ListNormalizedLocalEvents(context.Background(), store, "eve_game", 10)
	if len(chatBefore) != 1 || len(gameBefore) != 1 {
		t.Fatal("missing events")
	}
}
