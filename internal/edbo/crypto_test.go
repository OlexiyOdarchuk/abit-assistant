package edbo

import (
	"errors"
	"strings"
	"testing"
)

// TestRoundTrip proves the algorithm is self-consistent: encrypting a
// plaintext and decrypting the result with the same (salt, year) gives
// back exactly the input. Without a live EDBO blob to compare against
// this is the most we can verify locally — it does NOT prove that we
// match production byte-for-byte, only that the AES/SHA256/base64
// chain in Decrypt is internally consistent.
func TestRoundTrip(t *testing.T) {
	cases := []struct {
		name      string
		plaintext string
		salt      string
		year      string
	}{
		{"ascii", "Іваненко І.О.", "v14", "2025"},
		{"long cyrillic", "Шевченко-Петренко Тарас Григорович", "v123", "2025"},
		{"unicode mix", "О’Брайєн О. О.", "v9876", "2026"},
		{"single rune", "А", "v1", "2025"},
		{"empty year override", "abc", "v0", "2024"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			blob, err := Encrypt(tc.plaintext, tc.salt, tc.year)
			if err != nil {
				t.Fatalf("Encrypt: %v", err)
			}
			got, err := Decrypt(blob, tc.salt, tc.year)
			if err != nil {
				t.Fatalf("Decrypt: %v", err)
			}
			if got != tc.plaintext {
				t.Errorf("round-trip mismatch:\n got:  %q\n want: %q", got, tc.plaintext)
			}
		})
	}
}

// TestSaltMultiply pins the canonical "v" + a*b format that matches
// vstup2025.edbo.gov.ua/js/functions.js — multiply(a, b) returns
// 'v' + Number(a) * Number(b).
func TestSaltMultiply(t *testing.T) {
	cases := []struct {
		a, b int
		want string
	}{
		{14, 5, "v70"},
		{1, 1, "v1"},
		{0, 0, "v0"},
		{100, 100, "v10000"},
	}
	for _, tc := range cases {
		got := SaltMultiply(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("SaltMultiply(%d, %d) = %q, want %q",
				tc.a, tc.b, got, tc.want)
		}
	}
}

// TestDecryptName_WrongSaltFails is a sanity check: a blob produced
// with one salt cannot be decoded with another — the padding check
// must reject (or AES garbage clearly doesn't match plaintext).
func TestDecryptName_WrongSaltFails(t *testing.T) {
	blob, err := Encrypt("Test Test", "v100", "2025")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decrypt(blob, "v999", "2025")
	if err == nil && got == "Test Test" {
		t.Fatal("decryption with wrong salt should not yield the plaintext")
	}
	// Either we get a clean padding error, or we get garbage — both fine.
	if err != nil && !errors.Is(err, ErrInvalidPadding) && !errors.Is(err, ErrInvalidBlockSize) {
		t.Logf("got %v (acceptable: padding or garbage)", err)
	}
}

func TestDecrypt_InvalidBase64(t *testing.T) {
	if _, err := Decrypt("@@@not-base64@@@", "v1", "2025"); err == nil {
		t.Fatal("expected error on garbage base64 input")
	}
}

// TestSaltName pins the live offer-template salt formula
// "v" + (7500-prsid)*n, captured from vstup2025.edbo.gov.ua/offer/1507081/
// in June 2026.
func TestSaltName(t *testing.T) {
	cases := []struct {
		prsid, n int
		want     string
	}{
		{14, 1, "v7486"},  // (7500-14)*1
		{14, 2, "v14972"}, // (7500-14)*2
		{14, 10, "v74860"},
	}
	for _, tc := range cases {
		if got := SaltName(tc.prsid, tc.n); got != tc.want {
			t.Errorf("SaltName(%d, %d) = %q, want %q", tc.prsid, tc.n, got, tc.want)
		}
	}
}

// TestDecryptName_KnownSample pins the formula against a REAL blob
// captured from the live offer page. These are the `p` (priority)
// fields of the first applicants of offer 1507081 (КНУ ім. Шевченка),
// status id 14, year 2025. With the correct (7500-prsid)*n salt each
// decrypts to a clean "<priority> (Б)" value; the old prsid*n salt
// yields padding garbage. This is the regression guard for the
// 9b59... formula fix.
func TestDecryptName_KnownSample(t *testing.T) {
	cases := []struct {
		blob     string
		prsid, n int
		want     string
	}{
		{"dEZ3eS94ZjVVVWlab2lHd0o4ZHFJZz09", 14, 1, "5 (Б)"},
	}
	for _, tc := range cases {
		got, err := DecryptName(tc.blob, tc.prsid, tc.n, "2025")
		if err != nil {
			t.Fatalf("DecryptName(%q, %d, %d): %v", tc.blob, tc.prsid, tc.n, err)
		}
		if strings.TrimSpace(got) != tc.want {
			t.Errorf("DecryptName(%q, %d, %d) = %q, want %q",
				tc.blob, tc.prsid, tc.n, got, tc.want)
		}
	}
}
