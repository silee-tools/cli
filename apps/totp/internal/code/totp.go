// Package code computes RFC 6238 TOTP codes from base32 secrets.
package code

import (
	"fmt"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// Generate returns the current 6-digit TOTP code for the given base32 secret.
// Whitespace and dashes in the secret are stripped, and the result is
// upper-cased before decoding (matching the original zsh implementation).
func Generate(secret string) (string, error) {
	return GenerateAt(secret, time.Now())
}

// GenerateAt computes the code at a specific time. Exposed for testing with
// RFC 6238 standard vectors.
func GenerateAt(secret string, t time.Time) (string, error) {
	cleaned := normalize(secret)
	if cleaned == "" {
		return "", fmt.Errorf("empty secret")
	}
	code, err := totp.GenerateCodeCustom(cleaned, t, totp.ValidateOpts{
		Period:    30,
		Skew:      0,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	if err != nil {
		return "", fmt.Errorf("generate code: %w", err)
	}
	return code, nil
}

func normalize(secret string) string {
	s := strings.TrimSpace(secret)
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "-", "")
	return strings.ToUpper(s)
}
