package thunderauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestCanonicalHMAC(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	canonical := "THUNDER-AUTH-V1\nPOST\n/api/v1/auth/token\n1790000000\nYWJjZGVmZ2hpamtsbW5vcA\n" + hex.EncodeToString(make([]byte, 32))
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(canonical))
	if got := hex.EncodeToString(mac.Sum(nil)); len(got) != 64 {
		t.Fatalf("unexpected signature length: %d", len(got))
	}
}
