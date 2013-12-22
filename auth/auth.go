package auth

import (
	"crypto/sha512"
	"fmt"
)

// SaltAndHash creates a SHA512 hash of a byte string salted with an
// internal array of random bytes.
func SaltAndHash(b []byte) string {
	h := sha512.New()
	h.Write(salt)
	h.Write(b)
	return fmt.Sprintf("%x", h.Sum(nil))
}

// SaltAndHash creates a SHA512 hash of a string salted with an
// internal array of random bytes.
func SaltAndHashString(s string) string {
	return SaltAndHash([]byte(s))
}
