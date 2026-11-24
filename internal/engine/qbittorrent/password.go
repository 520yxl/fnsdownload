package qbittorrent

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/base64"
)

// qBittorrent's PBKDF2 format: SHA-512, 100000 iterations, 16-byte salt.
func passwordHash(password string, salt []byte) string {
	h := hmac.New(sha512.New, []byte(password))
	h.Write(salt)
	h.Write([]byte{0, 0, 0, 1})
	u := h.Sum(nil)
	key := append([]byte(nil), u...)
	for i := 1; i < 100000; i++ {
		h.Reset()
		h.Write(u)
		u = h.Sum(nil)
		for j := range key {
			key[j] ^= u[j]
		}
	}
	return base64.StdEncoding.EncodeToString(salt) + ":" + base64.StdEncoding.EncodeToString(key)
}
