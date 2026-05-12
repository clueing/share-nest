package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

const passwordIterations = 120000

func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	sum := deriveKey(password, salt, passwordIterations)
	return fmt.Sprintf("sha256$%d$%s$%s", passwordIterations, base64.RawStdEncoding.EncodeToString(salt), hex.EncodeToString(sum)), nil
}

func VerifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "sha256" {
		return false
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}

	expected, err := hex.DecodeString(parts[3])
	if err != nil {
		return false
	}

	actual := deriveKey(password, salt, passwordIterations)
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

func SignToken(secret, payload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func EqualString(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func RandomString(length int) string {
	const alphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	bytes := make([]byte, length)
	random := make([]byte, length)
	if _, err := rand.Read(random); err != nil {
		return "sharecode00"
	}
	for i := range bytes {
		bytes[i] = alphabet[int(random[i])%len(alphabet)]
	}
	return string(bytes)
}

func deriveKey(password string, salt []byte, iterations int) []byte {
	buf := make([]byte, 0, len(salt)+len(password))
	buf = append(buf, salt...)
	buf = append(buf, password...)

	sum := sha256.Sum256(buf)
	result := sum[:]
	for i := 1; i < iterations; i++ {
		next := sha256.Sum256(append(result, salt...))
		result = next[:]
	}

	final := make([]byte, len(result))
	copy(final, result)
	return final
}

