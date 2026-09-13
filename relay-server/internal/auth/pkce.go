// Package auth provides small, testable authentication primitives.
package auth

import (
 "crypto/rand"
 "crypto/sha256"
 "encoding/base64"
 "errors"
)

// Challenge contains a verifier and its RFC 7636 S256 challenge.
type Challenge struct { Verifier string; Challenge string }

// NewPKCE generates a cryptographically random verifier (32 bytes) and challenge.
func NewPKCE() (Challenge, error) {
 b := make([]byte, 32)
 if _, err := rand.Read(b); err != nil { return Challenge{}, err }
 v := base64.RawURLEncoding.EncodeToString(b)
 h := sha256.Sum256([]byte(v))
 return Challenge{Verifier:v, Challenge:base64.RawURLEncoding.EncodeToString(h[:])}, nil
}

// VerifyPKCE checks a verifier against an S256 challenge using constant-time comparison.
func VerifyPKCE(verifier, challenge string) error {
 if verifier == "" || challenge == "" { return errors.New("verifier and challenge are required") }
 h := sha256.Sum256([]byte(verifier))
 got := base64.RawURLEncoding.EncodeToString(h[:])
 if len(got) != len(challenge) { return errors.New("pkce challenge mismatch") }
 var diff byte
 for i := range got { diff |= got[i] ^ challenge[i] }
 if diff != 0 { return errors.New("pkce challenge mismatch") }
 return nil
}
