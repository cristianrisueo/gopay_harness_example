package idempotency

import (
	"errors"
	"testing"
)

func TestParseKey(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "canonical lowercase", raw: "6f1c2a9e-3b4d-4e5f-8a7b-9c0d1e2f3a4b", want: "6f1c2a9e-3b4d-4e5f-8a7b-9c0d1e2f3a4b"},
		{name: "canonical uppercase is normalized", raw: "6F1C2A9E-3B4D-4E5F-8A7B-9C0D1E2F3A4B", want: "6f1c2a9e-3b4d-4e5f-8a7b-9c0d1e2f3a4b"},
		{name: "mixed case is normalized", raw: "6f1C2a9E-3b4D-4e5F-8a7B-9c0D1e2F3a4B", want: "6f1c2a9e-3b4d-4e5f-8a7b-9c0d1e2f3a4b"},
		{name: "empty", raw: "", wantErr: true},
		{name: "without hyphens", raw: "6f1c2a9e3b4d4e5f8a7b9c0d1e2f3a4b", wantErr: true},
		{name: "with braces", raw: "{6f1c2a9e-3b4d-4e5f-8a7b-9c0d1e2f3a4b}", wantErr: true},
		{name: "with urn prefix", raw: "urn:uuid:6f1c2a9e-3b4d-4e5f-8a7b-9c0d1e2f3a4b", wantErr: true},
		{name: "too short", raw: "6f1c2a9e-3b4d-4e5f-8a7b-9c0d1e2f3a4", wantErr: true},
		{name: "too long", raw: "6f1c2a9e-3b4d-4e5f-8a7b-9c0d1e2f3a4b0", wantErr: true},
		{name: "non hex characters", raw: "6f1c2a9e-3b4d-4e5f-8a7b-9c0d1e2f3a4z", wantErr: true},
		{name: "hyphens in wrong positions", raw: "6f1c2a9e3-b4d-4e5f-8a7b-9c0d1e2f3a4b", wantErr: true},
		{name: "surrounding spaces", raw: " 6f1c2a9e-3b4d-4e5f-8a7b-9c0d1e2f3a4b", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseKey(tt.raw)

			if tt.wantErr {
				if !errors.Is(err, ErrInvalidKey) {
					t.Fatalf("ParseKey(%q) error = %v, want ErrInvalidKey", tt.raw, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("ParseKey(%q) unexpected error: %v", tt.raw, err)
			}

			if got != tt.want {
				t.Fatalf("ParseKey(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
