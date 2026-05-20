// Package edbo provides utilities for working with the vstup.edbo.gov.ua
// platform — specifically, decryption of obfuscated applicant fields.
//
// The 2026 portal serves full names as AES-CBC ciphertext computed from
// the applicant's position (n) and admission status (prsid). The key and
// IV are derived as in the page's Handlebars helper:
//
//	salt = string((7500 - prsid) * n)
//	key  = hex(sha256(salt))[:32]  // 32 ASCII chars
//	iv   = hex(sha256(year))[:16]  // 16 ASCII chars
//
// The emitted blob is base64 *twice*: outer is what the template prints,
// inner is the actual AES output. Some payloads only have a single layer.
package edbo

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var (
	// ErrInvalidBlockSize means the ciphertext length is not a multiple
	// of the AES block size — most likely the input wasn't valid output
	// from the template.
	ErrInvalidBlockSize = errors.New("edbo: ciphertext not a multiple of AES block size")

	// ErrInvalidPadding means PKCS#7 padding stripping failed. Most often
	// caused by a wrong (n, prsid, year) triple.
	ErrInvalidPadding = errors.New("edbo: invalid PKCS#7 padding")
)

// DecryptName recovers an applicant's full name from the AES-CBC blob
// produced by vstup.edbo.gov.ua templates.
//
// Arguments:
//
//	encrypted — the base64 string emitted by the template;
//	n         — 1-based position in the list of applicants for the offer;
//	prsid     — admission status code (e.g. 14 = "Допущено");
//	year      — admission year as a string ("2025", "2026", ...).
func DecryptName(encrypted string, n, prsid int, year string) (string, error) {
	salt := strconv.Itoa((7500 - prsid) * n)

	keyHash := sha256.Sum256([]byte(salt))
	key := []byte(hex.EncodeToString(keyHash[:])[:32])

	ivHash := sha256.Sum256([]byte(year))
	iv := []byte(hex.EncodeToString(ivHash[:])[:16])

	outer, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return "", fmt.Errorf("edbo: outer base64: %w", err)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(string(outer))
	if err != nil {
		// Single-layer payload — use the outer bytes as ciphertext.
		ciphertext = outer
	}
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return "", ErrInvalidBlockSize
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("edbo: aes cipher: %w", err)
	}

	plain := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, ciphertext)

	plain, err = pkcs7Unpad(plain, aes.BlockSize)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(plain)), nil
}

func pkcs7Unpad(b []byte, blockSize int) ([]byte, error) {
	if len(b) == 0 || len(b)%blockSize != 0 {
		return nil, ErrInvalidPadding
	}
	pad := int(b[len(b)-1])
	if pad == 0 || pad > blockSize {
		return nil, ErrInvalidPadding
	}
	for _, c := range b[len(b)-pad:] {
		if int(c) != pad {
			return nil, ErrInvalidPadding
		}
	}
	return b[:len(b)-pad], nil
}
