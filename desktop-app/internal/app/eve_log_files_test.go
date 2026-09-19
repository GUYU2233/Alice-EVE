package app

import "testing"

func TestMatchChatLogFilename(t *testing.T) {
	got, ok := matchChatLogFilename(`C:\logs\本地_20260916_043250_2115507479.txt`)
	if !ok || got.Channel != "本地" || got.CharacterID != 2115507479 || got.Session.IsZero() {
		t.Fatalf("got=%+v ok=%v", got, ok)
	}
	for _, name := range []string{"notes.txt", "本地_20261340_043250_1.txt", "本地_20260916_043250_0.txt"} {
		if _, ok := matchChatLogFilename(name); ok {
			t.Fatalf("matched %q", name)
		}
	}
}
func TestMatchGameLogFilename(t *testing.T) {
	got, ok := matchGameLogFilename(`C:\logs\20260916_043250_2115507479.txt`)
	if !ok || got.CharacterID != 2115507479 || got.Session.IsZero() {
		t.Fatalf("got=%+v ok=%v", got, ok)
	}
	if _, ok := matchGameLogFilename("20260916_bad_1.txt"); ok {
		t.Fatal("invalid matched")
	}
}
