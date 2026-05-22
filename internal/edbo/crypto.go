// Package edbo provides utilities for working with the vstup.edbo.gov.ua
// platform — specifically, decryption of obfuscated applicant fields.
//
// The portal serves full names (and other PII) as AES-CBC ciphertext.
// The exact key derivation is taken verbatim from the page's
// Handlebars helper at vstup2025.edbo.gov.ua/js/functions.js:
//
//	Handlebars.registerHelper('dec', function(a, b) {
//	    const $sk = b;
//	    const $si = '2025';
//	    const k = CryptoJS.SHA256($sk).toString(CryptoJS.enc.Hex).substring(0, 32);
//	    let   i = CryptoJS.SHA256($si).toString(CryptoJS.enc.Hex).substring(0, 16);
//	    const e = CryptoJS.enc.Base64.parse(a).toString(CryptoJS.enc.Utf8);
//	    const d = CryptoJS.AES.decrypt(e, Utf8.parse(k), { iv: Utf8.parse(i) }).toString(Utf8);
//	    return d;
//	});
//
// Salt (b) is whatever the template passes, usually built from another
// helper:
//
//	Handlebars.registerHelper('multiply', function(a, b) {
//	    return 'v' + (Number(a) * Number(b));
//	});
//
// So a typical decrypt call is `{{dec encryptedName (multiply x y)}}`,
// producing salt = "v" + str(x*y). Concrete x/y picks are template-
// specific — we leave them to the caller and accept a raw salt string.
//
// The emitted blob is base64 twice over: outer is what the template
// prints, inner is the AES output. Some payloads only have a single
// layer — the decoder transparently handles either.
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
	// caused by a wrong salt/year combination.
	ErrInvalidPadding = errors.New("edbo: invalid PKCS#7 padding")
)

// Decrypt decodes the AES-CBC blob `encrypted` (base64-encoded, possibly
// twice) using the salt and year as in the upstream template.
//
//	key = hex(sha256(salt))[:32]      // 32 ASCII bytes
//	iv  = hex(sha256(year))[:16]      // 16 ASCII bytes
//	plain = AES-CBC-Decrypt(base64*base64(encrypted), key, iv)
//
// salt is whatever the template's `multiply` helper (or equivalent)
// builds — caller's responsibility. For the common `multiply(a, b)`
// case use SaltMultiply(a, b).
func Decrypt(encrypted, salt, year string) (string, error) {
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

// SaltMultiply replicates the upstream `multiply(a, b)` Handlebars
// helper: it returns "v" + str(a*b), which is the canonical salt
// format for most encrypted fields in EDBO templates.
func SaltMultiply(a, b int) string {
	return "v" + strconv.Itoa(a*b)
}

// DecryptName is a convenience wrapper: builds the salt with
// SaltMultiply(prsid, n) and calls Decrypt. Matches the typical
// `{{dec encName (multiply prsid n)}}` call shape in templates.
func DecryptName(encrypted string, prsid, n int, year string) (string, error) {
	return Decrypt(encrypted, SaltMultiply(prsid, n), year)
}

// Encrypt is the inverse of Decrypt — produces the doubly-base64'd
// blob exactly like the upstream template would. We don't use this in
// the bot; it exists so the unit tests can prove round-trip symmetry
// without needing a live encrypted payload from EDBO.
func Encrypt(plaintext, salt, year string) (string, error) {
	keyHash := sha256.Sum256([]byte(salt))
	key := []byte(hex.EncodeToString(keyHash[:])[:32])

	ivHash := sha256.Sum256([]byte(year))
	iv := []byte(hex.EncodeToString(ivHash[:])[:16])

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	padded := pkcs7Pad([]byte(plaintext), aes.BlockSize)
	ct := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ct, padded)

	inner := base64.StdEncoding.EncodeToString(ct)
	outer := base64.StdEncoding.EncodeToString([]byte(inner))
	return outer, nil
}

func pkcs7Pad(b []byte, blockSize int) []byte {
	padLen := blockSize - len(b)%blockSize
	out := make([]byte, len(b)+padLen)
	copy(out, b)
	for i := len(b); i < len(out); i++ {
		out[i] = byte(padLen)
	}
	return out
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
