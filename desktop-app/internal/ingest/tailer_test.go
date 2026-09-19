package ingest

import (
	"context"
	"encoding/binary"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
	"unicode/utf16"
)

type memoryCursor struct {
	mu sync.Mutex
	m  map[string]int64
}

func (c *memoryCursor) Load(_ context.Context, key string) (int64, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.m[key]
	return v, ok, nil
}
func (c *memoryCursor) Commit(_ context.Context, key string, offset int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[key] = offset
	return nil
}
func (c *memoryCursor) get(key string) int64 { c.mu.Lock(); defer c.mu.Unlock(); return c.m[key] }

func selectEncoding(enc Encoding) Selector {
	return func(_ string, e fs.DirEntry) (File, bool) {
		return File{Key: e.Name(), Encoding: enc}, filepath.Ext(e.Name()) == ".log"
	}
}

func newTestTailer(t *testing.T, dir string, enc Encoding, cursor Cursor, handler Handler) *Tailer {
	t.Helper()
	tailer, err := New(Config{Directory: dir, Selector: selectEncoding(enc), Handler: handler, Cursor: cursor, MaxReadBytes: 1024, MaxRecordsScan: 100})
	if err != nil {
		t.Fatal(err)
	}
	return tailer
}

func TestUTF8PartialLineRereadAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.log")
	if err := os.WriteFile(path, []byte("complete\npart"), 0600); err != nil {
		t.Fatal(err)
	}
	cursor := &memoryCursor{m: map[string]int64{"a.log": 0}}
	var got []Record
	h := func(_ context.Context, r Record, commit CommitFunc) error { got = append(got, r); return commit() }
	one := newTestTailer(t, dir, UTF8, cursor, h)
	if err := one.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Text != "complete" || got[0].Start != 0 || got[0].End != 9 {
		t.Fatalf("first records: %#v", got)
	}
	if cursor.get("a.log") != 9 {
		t.Fatalf("cursor=%d", cursor.get("a.log"))
	}

	if err := os.WriteFile(path, []byte("complete\npartial\n"), 0600); err != nil {
		t.Fatal(err)
	}
	two := newTestTailer(t, dir, UTF8, cursor, h) // process restart: no remainder retained
	if err := two.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1].Text != "partial" || got[1].Start != 9 || got[1].End != 17 {
		t.Fatalf("records: %#v", got)
	}
}

func utf16LE(s string) []byte {
	u := utf16.Encode([]rune(s))
	out := make([]byte, len(u)*2)
	for i, v := range u {
		binary.LittleEndian.PutUint16(out[i*2:], v)
	}
	return out
}

func TestUTF16LEPartialLineRereadAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "u.log")
	partial := utf16LE("alpha\n世")
	if err := os.WriteFile(path, partial, 0600); err != nil {
		t.Fatal(err)
	}
	cursor := &memoryCursor{m: map[string]int64{"u.log": 0}}
	var got []Record
	h := func(_ context.Context, r Record, commit CommitFunc) error { got = append(got, r); return commit() }
	if err := newTestTailer(t, dir, UTF16LE, cursor, h).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Text != "alpha" || got[0].End != 12 {
		t.Fatalf("first records: %#v", got)
	}
	if err := os.WriteFile(path, append(partial, utf16LE("界\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	if err := newTestTailer(t, dir, UTF16LE, cursor, h).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1].Text != "世界" || got[1].Start != 12 || got[1].End != 18 {
		t.Fatalf("records: %#v", got)
	}
}

func TestHandlerFailureDoesNotAdvanceCursor(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.log")
	if err := os.WriteFile(path, []byte("one\ntwo\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cursor := &memoryCursor{m: map[string]int64{"a.log": 0}}
	wantErr := errors.New("reject")
	calls := 0
	tailer := newTestTailer(t, dir, UTF8, cursor, func(_ context.Context, r Record, commit CommitFunc) error {
		calls++
		if calls == 1 {
			return wantErr
		}
		return commit()
	})
	if err := tailer.Scan(context.Background()); !errors.Is(err, wantErr) {
		t.Fatalf("error=%v", err)
	}
	if cursor.get("a.log") != 0 {
		t.Fatalf("cursor advanced to %d", cursor.get("a.log"))
	}
	if err := tailer.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if cursor.get("a.log") != 8 || calls != 3 {
		t.Fatalf("cursor=%d calls=%d", cursor.get("a.log"), calls)
	}
}

func TestTruncationResetsCursorAndReadsNewRecords(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.log")
	if err := os.WriteFile(path, []byte("new\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cursor := &memoryCursor{m: map[string]int64{"a.log": 99}}
	var got Record
	tailer := newTestTailer(t, dir, UTF8, cursor, func(_ context.Context, r Record, commit CommitFunc) error { got = r; return commit() })
	if err := tailer.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got.Text != "new" || got.Start != 0 || got.End != 4 || cursor.get("a.log") != 4 {
		t.Fatalf("record=%#v cursor=%d", got, cursor.get("a.log"))
	}
}

func TestInitialPositionStartsAtEnd(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.log")
	if err := os.WriteFile(path, []byte("old\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cursor := &memoryCursor{m: map[string]int64{}}
	calls := 0
	tailer := newTestTailer(t, dir, UTF8, cursor, func(_ context.Context, r Record, commit CommitFunc) error { calls++; return commit() })
	if err := tailer.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 0 || cursor.get("a.log") != 4 {
		t.Fatalf("calls=%d cursor=%d", calls, cursor.get("a.log"))
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString("new\n")
	_ = f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if err := tailer.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestStartIdempotentStopWaitsAndScansDoNotOverlap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.log")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	cursor := &memoryCursor{m: map[string]int64{"a.log": 0}}
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	h := func(_ context.Context, r Record, commit CommitFunc) error {
		once.Do(func() { close(entered) })
		<-release
		return commit()
	}
	tailer, err := New(Config{Directory: dir, PollInterval: time.Millisecond, Selector: selectEncoding(UTF8), Handler: h, Cursor: cursor})
	if err != nil {
		t.Fatal(err)
	}
	if err := tailer.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := tailer.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x\n"), 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("handler not entered")
	}
	stopped := make(chan struct{})
	go func() { tailer.Stop(); close(stopped) }()
	select {
	case <-stopped:
		t.Fatal("Stop returned before handler")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Stop did not return")
	}
	tailer.Stop()
}
