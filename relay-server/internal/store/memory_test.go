package store

import (
	"context"
	"testing"
)

func TestMemoryStoreTenantScopedMessageIdentityAndAck(t *testing.T) {
	s := NewMemory()
	ctx := context.Background()
	if err := s.PutMessage(ctx, Message{ID: "same", AccountID: "acct-a", OwnerDeviceID: "dev-a", Body: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutMessage(ctx, Message{ID: "same", AccountID: "acct-b", OwnerDeviceID: "dev-b", Body: "b"}); err != nil {
		t.Fatal(err)
	}
	for _, item := range [][3]string{{"acct-a", "dev-a", "a"}, {"acct-b", "dev-b", "b"}} {
		account, id, want := item[0], item[1], item[2]
		rows, err := s.ListMessages(ctx, account, id, 0, 10)
		if err != nil || len(rows) != 1 || rows[0].Body != want {
			t.Fatalf("account=%s rows=%#v err=%v", account, rows, err)
		}
	}
	if ok, err := s.HasMessage(ctx, "acct-a", "same"); err != nil || !ok {
		t.Fatalf("acct-a has=%v err=%v", ok, err)
	}
	if ok, err := s.HasMessage(ctx, "acct-b", "same"); err != nil || !ok {
		t.Fatalf("acct-b has=%v err=%v", ok, err)
	}
	if ok, err := s.AckMessage(ctx, "acct-a", "same", "dev-b"); err != nil || ok {
		t.Fatalf("cross-account ack=%v err=%v", ok, err)
	}
	if ok, err := s.AckMessage(ctx, "acct-a", "same", "dev-a"); err != nil || !ok {
		t.Fatalf("owner ack=%v err=%v", ok, err)
	}
	rows, err := s.ListMessages(ctx, "acct-b", "dev-b", 0, 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("acct-b affected by acct-a ack rows=%#v err=%v", rows, err)
	}
}

func TestMemoryStoreDeviceTenantIsolation(t *testing.T) {
	s := NewMemory()
	if err := s.AddDevice(Device{ID: "dev-a", AccountID: "acct-a", Type: "mobile"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddDevice(Device{ID: "dev-b", AccountID: "acct-b", Type: "mobile"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.GetDeviceForAccount("acct-a", "dev-b"); ok {
		t.Fatal("cross-account device lookup succeeded")
	}
	if !s.RevokeDeviceForAccount("acct-a", "dev-b") {
		// false is expected: the foreign device must not be revocable.
	} else {
		t.Fatal("cross-account device revoke succeeded")
	}
	if _, ok := s.GetDeviceForAccount("acct-b", "dev-b"); !ok {
		t.Fatal("foreign device was changed by rejected revoke")
	}
}
