package app

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
	"unicode/utf16"
)

func utf16LE(text string) []byte {
	units := utf16.Encode([]rune(text))
	out := make([]byte, 2+len(units)*2)
	out[0], out[1] = 0xff, 0xfe
	for i, u := range units {
		binary.LittleEndian.PutUint16(out[2+i*2:], u)
	}
	return out
}

func TestChatLogWatcherTailsUTF16AndDeduplicates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Intel_20260916_010000.txt")
	header := "---------------------------------------------------------------\r\n  Channel ID: 1\r\n  Channel Name: Intel\r\n  Listener: Alice\r\n---------------------------------------------------------------\r\n"
	if err := os.WriteFile(path, utf16LE(header), 0600); err != nil {
		t.Fatal(err)
	}
	w := NewChatLogWatcher(nil)
	if err := w.SetDirectory(dir); err != nil {
		t.Fatal(err)
	}
	if err := w.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer w.Stop()
	line := "[ 2026.09.16 01:00:01 ] Scout > hostile Drake in Jita\r\n"
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	raw := utf16LE(line)[2:]
	if _, err = f.Write(raw); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if len(w.Recent()) == 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	events := w.Recent()
	if len(events) != 1 {
		t.Fatalf("events=%v status=%+v", events, w.Status())
	}
	if events[0].Channel != "Intel" || events[0].Author != "Scout" || len(events[0].Findings) < 2 {
		t.Fatalf("event=%+v", events[0])
	}
	if err := w.scan(false); err != nil {
		t.Fatal(err)
	}
	if len(w.Recent()) != 1 {
		t.Fatal("duplicate event emitted")
	}
}

func TestDecodeCompleteChatBytesKeepsUTF16PartialLine(t *testing.T) {
	data := utf16LE("one\r\ntwo")
	text, n := decodeCompleteChatBytes(data)
	if text != "\ufeffone\r\n" || n >= len(data) {
		t.Fatalf("text=%q consumed=%d total=%d", text, n, len(data))
	}
}
