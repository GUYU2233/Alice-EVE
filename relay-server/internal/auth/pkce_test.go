package auth

import "testing"

func TestPKCE(t *testing.T) {
 c, err := NewPKCE(); if err != nil { t.Fatal(err) }
 if c.Verifier == "" || c.Challenge == "" { t.Fatal("empty pkce values") }
 if err := VerifyPKCE(c.Verifier, c.Challenge); err != nil { t.Fatal(err) }
 if err := VerifyPKCE(c.Verifier+"x", c.Challenge); err == nil { t.Fatal("tampered verifier accepted") }
}
func TestPKCERequired(t *testing.T) { if VerifyPKCE("", "x") == nil { t.Fatal("empty verifier accepted") } }
