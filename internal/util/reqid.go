package util

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// NewReqID generates a request ID.
func NewReqID(prefix string) string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		if prefix == "" {
			prefix = "req"
		}
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}

	if prefix == "" {
		prefix = "req"
	}
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(b))
}
