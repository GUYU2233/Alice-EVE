package app

import (
	"context"
	"testing"
	"time"
)

func TestMemoryCursorMonotonic(t *testing.T) {
	c := newMemoryCursor()
	if err := c.SaveCursor(context.Background(), "x", 4); err != nil {
		t.Fatal(err)
	}
	if err := c.SaveCursor(context.Background(), "x", 2); err != nil {
		t.Fatal(err)
	}
	v, err := c.LoadCursor(context.Background(), "x")
	if err != nil || v != 4 {
		t.Fatalf("cursor=%d err=%v", v, err)
	}
}
func TestRealtimeStatusInitial(t *testing.T) {
	c := NewRealtimeClient(NewRelayClient(), nil)
	if c.Status().State != "stopped" {
		t.Fatal(c.Status())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = ctx
	c.Stop()
	select {
	case <-time.After(20 * time.Millisecond):
	default:
	}
}
