package helpers

import (
	"errors"
	"testing"
)

func TestParseCents(t *testing.T) {
	tests := []struct {
		in   string
		want Cents
	}{
		{"89.90", 8990},
		{"89.9", 8990},
		{"89", 8900},
		{"0.05", 5},
		{"0", 0},
		{"12345678.99", 1234567899},
	}
	for _, tt := range tests {
		got, err := ParseCents(tt.in)
		if err != nil {
			t.Fatalf("ParseCents(%q): %v", tt.in, err)
		}
		if got != tt.want {
			t.Errorf("ParseCents(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestParseCentsRejectsInvalid(t *testing.T) {
	for _, in := range []string{"", "-1.00", "1.234", "1,50", ".50", "abc", "1."} {
		if _, err := ParseCents(in); !errors.Is(err, ErrInvalidAmount) {
			t.Errorf("ParseCents(%q) error = %v, want ErrInvalidAmount", in, err)
		}
	}
}

func TestCentsString(t *testing.T) {
	tests := []struct {
		in   Cents
		want string
	}{
		{8990, "89.90"},
		{5, "0.05"},
		{0, "0.00"},
		{-450, "-4.50"},
	}
	for _, tt := range tests {
		if got := tt.in.String(); got != tt.want {
			t.Errorf("%d.String() = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCentsTimes(t *testing.T) {
	if got := MustParseCents("19.90").Times(3).String(); got != "59.70" {
		t.Errorf("19.90 × 3 = %s, want 59.70", got)
	}
}
