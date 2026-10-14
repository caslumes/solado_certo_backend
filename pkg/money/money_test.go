package money

import (
	"errors"
	"testing"
)

func TestParse(t *testing.T) {
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
		got, err := Parse(tt.in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tt.in, err)
		}
		if got != tt.want {
			t.Errorf("Parse(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestParseRejectsInvalid(t *testing.T) {
	for _, in := range []string{"", "-1.00", "1.234", "1,50", ".50", "abc", "1."} {
		if _, err := Parse(in); !errors.Is(err, ErrInvalidAmount) {
			t.Errorf("Parse(%q) error = %v, want ErrInvalidAmount", in, err)
		}
	}
}

func TestString(t *testing.T) {
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

func TestTimes(t *testing.T) {
	if got := MustParse("19.90").Times(3).String(); got != "59.70" {
		t.Errorf("19.90 × 3 = %s, want 59.70", got)
	}
}
