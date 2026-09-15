package accounts

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
)

func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", errors.New("secure random source unavailable")
	}
	// UUID string form keeps IDs accepted by the PostgreSQL UUID columns while
	// avoiding a runtime dependency solely for identifier generation.
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(b[:])
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32], nil
}
