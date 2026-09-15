package auth

import "testing"

func TestPKCE(t *testing.T) {
	c, err := NewPKCE()
	if err != nil {
		t.Fatal(err)
	}
	if c.Verifier == "" || c.Challenge == "" {
		t.Fatal("empty pkce values")
	}
	if err := VerifyPKCE(c.Verifier, c.Challenge); err != nil {
		t.Fatal(err)
	}
	if err := VerifyPKCE(c.Verifier+"x", c.Challenge); err == nil {
		t.Fatal("tampered verifier accepted")
	}
}
func TestPKCEVerifierRejectsMalformedInput(t *testing.T) {
	c, err := NewPKCE()
	if err != nil { t.Fatal(err) }
	for _, verifier := range []string{"short", "a" + string(make([]byte, 128)), "invalid!"} {
		if VerifyPKCE(verifier, c.Challenge) == nil { t.Fatalf("accepted malformed verifier") }
	}
}

func TestPKCERequired(t *testing.T) {
	if VerifyPKCE("", "x") == nil {
		t.Fatal("empty verifier accepted")
	}
}
