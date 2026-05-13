package security

import (
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

const passwordIterations = 310000

func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	sum, err := pbkdf2.Key(sha256.New, password, salt, passwordIterations, 32)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", passwordIterations, base64.RawStdEncoding.EncodeToString(salt), hex.EncodeToString(sum)), nil
}

func VerifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 {
		return false
	}

	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations <= 0 {
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

	var actual []byte
	switch parts[0] {
	case "pbkdf2-sha256":
		actual, err = pbkdf2.Key(sha256.New, password, salt, iterations, len(expected))
		if err != nil {
			return false
		}
	case "sha256":
		actual = deriveLegacyKey(password, salt, iterations)
	default:
		return false
	}
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

func RandomString(length int) (string, error) {
	const alphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	bytes := make([]byte, length)
	random := make([]byte, length)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	for i := range bytes {
		bytes[i] = alphabet[int(random[i])%len(alphabet)]
	}
	return string(bytes), nil
}

func deriveLegacyKey(password string, salt []byte, iterations int) []byte {
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
