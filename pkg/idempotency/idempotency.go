package idempotency

import (
	"errors"
	"strings"

	"github.com/google/uuid"
)

// HTTP headers of the idempotency contract
const (
	HeaderKey      = "Idempotency-Key"
	HeaderReplayed = "Idempotent-Replayed"
)

// canonicalKeyLength is the length of a UUID in its canonical 8-4-4-4-12 form
const canonicalKeyLength = 36

// ErrInvalidKey is returned when the key is not a UUID in canonical form
var ErrInvalidKey = errors.New("invalid idempotency key: must be a UUID in canonical form")

// ParseKey validates an idempotency key and returns it normalized to lowercase.
// Only the canonical 36 character form is accepted (any case). uuid.Parse also accepts
// forms without hyphens, with braces or with the urn:uuid: prefix, so the format is checked first.
func ParseKey(raw string) (string, error) {
	if len(raw) != canonicalKeyLength {
		return "", ErrInvalidKey
	}

	for i, c := range raw {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return "", ErrInvalidKey
			}
		default:
			if !isHexDigit(c) {
				return "", ErrInvalidKey
			}
		}
	}

	if _, err := uuid.Parse(raw); err != nil {
		return "", ErrInvalidKey
	}

	return strings.ToLower(raw), nil
}

func isHexDigit(c rune) bool {
	return ('0' <= c && c <= '9') || ('a' <= c && c <= 'f') || ('A' <= c && c <= 'F')
}
