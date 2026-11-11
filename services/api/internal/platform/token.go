package platform

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// NewToken returns a URL-safe random token (32 bytes entropy).
func NewToken() (raw string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", fmt.Errorf("rand: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
