package evegrant

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func testKey(b byte) string {
	return base64.RawStdEncoding.EncodeToString([]byte(strings.Repeat(string([]byte{b}), 32)))
}

func TestKeyringEncryptionRoundTripAndRotation(t *testing.T) {
	old, err := ParseKeyring("old=" + testKey(1))
	if err != nil {
		t.Fatal(err)
	}
	repo := NewMemoryRepository()
	service, _ := NewService(repo, old)
	expires := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	if err := service.Save(context.Background(), Grant{AccountID: "account", ProviderSubject: "character", RefreshToken: "super-secret", ExpiresAt: expires, Scope: "a b"}); err != nil {
		t.Fatal(err)
	}
	record, _ := repo.FindByAccount(context.Background(), "account")
	if strings.Contains(string(record.Ciphertext), "super-secret") {
		t.Fatal("plaintext persisted")
	}
	rotated, _ := ParseKeyring("new=" + testKey(2) + ",old=" + testKey(1))
	service, _ = NewService(repo, rotated)
	got, err := service.Load(context.Background(), "account")
	if err != nil {
		t.Fatal(err)
	}
	if got.RefreshToken != "super-secret" || got.Scope != "a b" || !got.ExpiresAt.Equal(expires) {
		t.Fatalf("got %#v", got)
	}
}

func TestKeyringRejectsInvalidAndGrantDoesNotSerializeTokens(t *testing.T) {
	for _, value := range []string{"", "missing", "k=AAAA", "k=" + testKey(1) + ",k=" + testKey(2)} {
		if _, err := ParseKeyring(value); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
	encoded, err := json.Marshal(Grant{RefreshToken: "must-not-leak"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "must-not-leak") {
		t.Fatal("token serialized")
	}
}
