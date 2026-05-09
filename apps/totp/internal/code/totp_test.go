package code

import (
	"testing"
	"time"
)

// RFC 6238 Appendix B test vectors (SHA-1, 30s period).
// Secret = ASCII "12345678901234567890" → base32 GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ.
// The RFC publishes 8-digit values; we truncate to 6 digits (right-most).
const rfcSecret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

func TestGenerateAt_RFC6238Vectors(t *testing.T) {
	cases := []struct {
		name     string
		unix     int64
		expected string // 6-digit truncation of RFC 8-digit code
	}{
		{"T=59", 59, "287082"},                 // RFC: 94287082
		{"T=1111111109", 1111111109, "081804"}, // RFC: 07081804
		{"T=1111111111", 1111111111, "050471"}, // RFC: 14050471
		{"T=1234567890", 1234567890, "005924"}, // RFC: 89005924
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := GenerateAt(rfcSecret, time.Unix(tc.unix, 0))
			if err != nil {
				t.Fatalf("GenerateAt: %v", err)
			}
			if got != tc.expected {
				t.Errorf("got %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestGenerate_NormalizesInput(t *testing.T) {
	// Same secret, but with whitespace, dashes and lowercase.
	dirty := " gezd-gnbv-gy3t-qojq-gezd-gnbv-gy3t-qojq "
	got, err := GenerateAt(dirty, time.Unix(59, 0))
	if err != nil {
		t.Fatalf("GenerateAt: %v", err)
	}
	if got != "287082" {
		t.Errorf("got %q, want 287082", got)
	}
}

func TestGenerate_EmptySecret(t *testing.T) {
	if _, err := Generate("   "); err == nil {
		t.Fatal("expected error for empty secret, got nil")
	}
}
