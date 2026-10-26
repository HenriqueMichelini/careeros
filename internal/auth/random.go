package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
)

func randomToken(size int) (string, error) {
	if size < 16 || size > 64 {
		return "", fmt.Errorf("invalid random token size")
	}
	buffer := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, buffer); err != nil {
		return "", fmt.Errorf("%w: random source", ErrUnavailable)
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func hashSecret(secret string) [32]byte {
	return sha256.Sum256([]byte(secret))
}
