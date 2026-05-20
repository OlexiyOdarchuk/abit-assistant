package edbo

import (
	"errors"
	"strings"
	"testing"
)

// TestDecryptName_KnownSample is wired to fire once a captured (b64, n,
// prsid, year) quadruple from a live vstup.edbo.gov.ua page is plugged in.
// The placeholder sample carried over from main.go does not have its real
// (n, prsid) — they're guesses — so unpadding fails and we skip until a
// genuine capture is added.
func TestDecryptName_KnownSample(t *testing.T) {
	t.Skip("needs a real (b64, n, prsid, year) tuple from a live edbo page; see TODO in package docs")

	got, err := DecryptName("MnZncVNmOGwva0UxZGFOK1VMTHpHdz09", 1, 14, "2025")
	if err != nil {
		t.Fatalf("DecryptName: %v", err)
	}
	if got == "" || strings.ContainsAny(got, "\x00\x01\x02\x03") {
		t.Fatalf("DecryptName returned suspect output: %q", got)
	}
	t.Logf("decrypted = %q", got)
}

func TestDecryptName_InvalidBase64(t *testing.T) {
	_, err := DecryptName("@@@not-base64@@@", 1, 14, "2025")
	if err == nil {
		t.Fatal("expected error on invalid base64")
	}
}

func TestDecryptName_WrongParamsYieldsBadPadding(t *testing.T) {
	// Using the same sample but wrong prsid/n should fail unpadding —
	// the AES block decrypts to garbage, which almost certainly isn't a
	// valid PKCS#7 suffix.
	_, err := DecryptName("MnZncVNmOGwva0UxZGFOK1VMTHpHdz09", 999, 999, "2025")
	if err == nil {
		t.Fatal("expected error with wrong (n, prsid)")
	}
	if !errors.Is(err, ErrInvalidPadding) && !errors.Is(err, ErrInvalidBlockSize) {
		t.Fatalf("expected padding/block error, got %v", err)
	}
}
