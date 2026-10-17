package passphrase

import (
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/base64"
	"fmt"
	"math/big"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

const (
	MinLength = 10
	numWords  = 4

	// OWASP Password Storage Cheat Sheet, argon2id option m=19 MiB, t=2, p=1.
	memoryKiB = 19 * 1024
	timeCost  = 2
	threads   = 1
	saltLen   = 16
	keyLen    = 32
)

// EFF short wordlist 1: https://www.eff.org/files/2016/09/08/eff_short_wordlist_1.txt
//
//go:embed words.txt
var wordsTxt string

var words = strings.Fields(wordsTxt)

var ErrTooShort = fmt.Errorf("passphrase must be at least %d characters", MinLength)

func Generate() (string, error) {
	n := big.NewInt(int64(len(words)))
	parts := make([]string, numWords)
	for i := range parts {
		k, err := rand.Int(rand.Reader, n)
		if err != nil {
			return "", fmt.Errorf("generating passphrase: %w", err)
		}
		parts[i] = words[k.Int64()]
	}
	return strings.Join(parts, "-"), nil
}

func Validate(p string) error {
	if utf8.RuneCountInString(p) < MinLength {
		return ErrTooShort
	}
	return nil
}

var b64 = base64.RawStdEncoding

// Each hash holds memoryKiB of RAM; the bound keeps a burst of logins from exhausting a small host.
var slots = make(chan struct{}, 4)

func idKey(p string, salt []byte, t, m uint32, par uint8, n uint32) []byte {
	slots <- struct{}{}
	defer func() { <-slots }()
	return argon2.IDKey([]byte(p), salt, t, m, par, n)
}

func Hash(p string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generating salt: %w", err)
	}
	key := idKey(p, salt, timeCost, memoryKiB, threads, keyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, memoryKiB, timeCost, threads, b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

func Verify(p, encoded string) bool {
	m, t, par, salt, key, ok := parse(encoded)
	return ok && subtle.ConstantTimeCompare(idKey(p, salt, t, m, par, uint32(len(key))), key) == 1
}

func parse(encoded string) (m, t uint32, p uint8, salt, key []byte, ok bool) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return
	}
	var v int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &v); err != nil || v != argon2.Version {
		return
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil || m == 0 || t == 0 || p == 0 {
		return
	}
	salt, err := b64.DecodeString(parts[4])
	if err != nil {
		return
	}
	key, err = b64.DecodeString(parts[5])
	return m, t, p, salt, key, err == nil && len(key) > 0
}
