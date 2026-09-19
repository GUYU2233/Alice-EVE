// Package ingest provides a parser-independent, polling file tailer.
package ingest

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unicode/utf16"
)

// Encoding is the on-disk encoding of a selected file.
type Encoding uint8

const (
	UTF8 Encoding = iota
	UTF16LE
)

// File identifies a selected log. Key is used to persist its cursor; when
// empty, the cleaned absolute path is used.
type File struct {
	Key      string
	Encoding Encoding
}

// Selector decides which regular directory entries are tailed.
type Selector func(path string, entry fs.DirEntry) (File, bool)

// Cursor persists the exclusive byte offset through which records have been
// successfully handled. Load distinguishes a missing cursor from offset zero.
type Cursor interface {
	Load(ctx context.Context, key string) (offset int64, present bool, err error)
	Commit(ctx context.Context, key string, offset int64) error
}

// Record is one complete newline-terminated record. Start and End are stable
// byte offsets in the source file; End is exclusive and includes the newline.
// Text excludes the newline and an immediately preceding carriage return.
type Record struct {
	File  File
	Path  string
	Text  string
	Start int64
	End   int64
}

// CommitFunc persists this record's End offset. A handler must call it after
// it has durably accepted the record. If it does not, the record is offered
// again on a later scan.
type CommitFunc func() error

// Handler receives complete records in file/offset order.
type Handler func(ctx context.Context, record Record, commit CommitFunc) error

// InitialPosition controls a file with no stored cursor.
type InitialPosition uint8

const (
	// ResumeIfPresentElseEnd resumes a stored cursor and otherwise starts at
	// the file's current end, avoiding ingestion of historical records.
	ResumeIfPresentElseEnd InitialPosition = iota
)

// Config configures a Tailer.
type Config struct {
	Directory      string
	PollInterval   time.Duration
	Selector       Selector
	Handler        Handler
	Cursor         Cursor
	Initial        InitialPosition
	MaxReadBytes   int
	MaxRecordsScan int
}

// Tailer polls one directory without overlapping scans.
type Tailer struct {
	cfg Config

	lifeMu sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
	scanMu sync.Mutex
}

// New validates config and constructs a stopped tailer.
func New(cfg Config) (*Tailer, error) {
	if cfg.Directory == "" || cfg.Selector == nil || cfg.Handler == nil || cfg.Cursor == nil {
		return nil, errors.New("ingest: directory, selector, handler, and cursor are required")
	}
	if cfg.Initial != ResumeIfPresentElseEnd {
		return nil, errors.New("ingest: unsupported initial position")
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = time.Second
	}
	if cfg.MaxReadBytes <= 0 {
		cfg.MaxReadBytes = 256 << 10
	}
	if cfg.MaxRecordsScan <= 0 {
		cfg.MaxRecordsScan = 1000
	}
	return &Tailer{cfg: cfg}, nil
}

// Start is idempotent. It performs an initial scan before starting the polling
// goroutine, so missing cursors are established at current file ends.
func (t *Tailer) Start(ctx context.Context) error {
	t.lifeMu.Lock()
	if t.cancel != nil {
		t.lifeMu.Unlock()
		return nil
	}
	info, err := os.Stat(t.cfg.Directory)
	if err != nil || !info.IsDir() {
		t.lifeMu.Unlock()
		if err == nil {
			err = errors.New("not a directory")
		}
		return fmt.Errorf("ingest: directory unavailable: %w", err)
	}
	// Keep lifeMu through the synchronous first scan so concurrent Start calls
	// cannot both initialize the same tailer.
	if err := t.Scan(ctx); err != nil {
		t.lifeMu.Unlock()
		return err
	}
	runCtx, cancel := context.WithCancel(ctx)
	t.cancel = cancel
	t.done = make(chan struct{})
	done := t.done
	t.lifeMu.Unlock()
	go t.loop(runCtx, done)
	return nil
}

func (t *Tailer) loop(ctx context.Context, done chan struct{}) {
	defer close(done)
	ticker := time.NewTicker(t.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = t.Scan(ctx)
		}
	}
}

// Stop is idempotent and waits for the polling goroutine (and any active
// handler called by it) to finish.
func (t *Tailer) Stop() {
	t.lifeMu.Lock()
	cancel, done := t.cancel, t.done
	if cancel == nil {
		t.lifeMu.Unlock()
		return
	}
	cancel()
	t.lifeMu.Unlock()
	<-done
	t.lifeMu.Lock()
	if t.done == done {
		t.cancel, t.done = nil, nil
	}
	t.lifeMu.Unlock()
}

// Scan runs one serialized directory scan. Calls may safely race with polling;
// they never overlap.
func (t *Tailer) Scan(ctx context.Context) error {
	t.scanMu.Lock()
	defer t.scanMu.Unlock()
	entries, err := os.ReadDir(t.cfg.Directory)
	if err != nil {
		return err
	}
	remainingRecords := t.cfg.MaxRecordsScan
	remainingBytes := t.cfg.MaxReadBytes
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if remainingRecords == 0 || remainingBytes == 0 {
			break
		}
		if entry.IsDir() || entry.Type()&fs.ModeType != 0 {
			continue
		}
		path := filepath.Join(t.cfg.Directory, entry.Name())
		file, ok := t.cfg.Selector(path, entry)
		if !ok {
			continue
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		path = filepath.Clean(abs)
		if file.Key == "" {
			file.Key = path
		}
		n, bytesRead, err := t.scanFile(ctx, path, file, remainingRecords, remainingBytes)
		remainingRecords -= n
		remainingBytes -= bytesRead
		if err != nil {
			return err
		}
	}
	return nil
}

func (t *Tailer) scanFile(ctx context.Context, path string, selected File, maxRecords, maxBytes int) (int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return 0, 0, err
	}
	offset, present, err := t.cfg.Cursor.Load(ctx, selected.Key)
	if err != nil {
		return 0, 0, err
	}
	if !present {
		if err := t.cfg.Cursor.Commit(ctx, selected.Key, st.Size()); err != nil {
			return 0, 0, err
		}
		return 0, 0, nil
	}
	if offset < 0 {
		offset = 0
	}
	if offset > st.Size() { // truncation/rotation at the same path
		offset = 0
		if err := t.cfg.Cursor.Commit(ctx, selected.Key, 0); err != nil {
			return 0, 0, err
		}
	}
	available := st.Size() - offset
	if available <= 0 {
		return 0, 0, nil
	}
	want := int64(maxBytes)
	if available < want {
		want = available
	}
	buf := make([]byte, int(want))
	n, readErr := f.ReadAt(buf, offset)
	if readErr != nil && readErr != io.EOF {
		return 0, n, readErr
	}
	buf = buf[:n]
	records := 0
	pos := 0
	for records < maxRecords {
		end := newlineEnd(buf, pos, selected.Encoding, offset)
		if end < 0 {
			break
		}
		textBytes := buf[pos:end]
		newlineWidth := 1
		if selected.Encoding == UTF16LE {
			newlineWidth = 2
		}
		textEnd := len(textBytes) - newlineWidth
		if textEnd < 0 {
			textEnd = 0
		}
		line := textBytes[:textEnd]
		if selected.Encoding == UTF16LE {
			if len(line) >= 2 && line[len(line)-2] == '\r' && line[len(line)-1] == 0 {
				line = line[:len(line)-2]
			}
		} else if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		startOffset, endOffset := offset+int64(pos), offset+int64(end)
		record := Record{File: selected, Path: path, Text: decode(line, selected.Encoding), Start: startOffset, End: endOffset}
		committed := false
		commit := func() error {
			if committed {
				return nil
			}
			if err := t.cfg.Cursor.Commit(ctx, selected.Key, endOffset); err != nil {
				return err
			}
			committed = true
			return nil
		}
		if err := t.cfg.Handler(ctx, record, commit); err != nil {
			return records, n, err
		}
		records++
		if !committed {
			break
		}
		pos = end
	}
	return records, n, nil
}

func newlineEnd(b []byte, start int, enc Encoding, absoluteOffset int64) int {
	if enc == UTF16LE {
		// A cursor committed by this package is code-unit aligned. If an external
		// cursor is odd, align scanning to the next code-unit boundary.
		i := start
		if (absoluteOffset+int64(i))%2 != 0 {
			i++
		}
		for ; i+1 < len(b); i += 2 {
			if b[i] == '\n' && b[i+1] == 0 {
				return i + 2
			}
		}
		return -1
	}
	for i := start; i < len(b); i++ {
		if b[i] == '\n' {
			return i + 1
		}
	}
	return -1
}

func decode(b []byte, enc Encoding) string {
	if enc == UTF8 {
		if len(b) >= 3 && b[0] == 0xef && b[1] == 0xbb && b[2] == 0xbf {
			b = b[3:]
		}
		return string(b)
	}
	if len(b)%2 != 0 {
		b = b[:len(b)-1]
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(b[i*2:])
	}
	if len(u) > 0 && u[0] == 0xfeff {
		u = u[1:]
	}
	return string(utf16.Decode(u))
}
