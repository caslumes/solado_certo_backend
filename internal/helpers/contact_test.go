package helpers

import (
	"strings"
	"testing"
)

func TestNormalizeEmail(t *testing.T) {
	valid := map[string]string{
		"ana@example.com":       "ana@example.com",
		"  Ana@Example.COM ":    "ana@example.com",
		"ana.maria+x@email.com": "ana.maria+x@email.com",
	}
	for input, want := range valid {
		got, err := NormalizeEmail(input)
		if err != nil || got != want {
			t.Errorf("NormalizeEmail(%q) = %q, %v; want %q", input, got, err, want)
		}
	}

	invalid := []string{"", "   ", "ana", "ana@", "Ana <ana@example.com>", strings.Repeat("a", 250) + "@x.com"}
	for _, input := range invalid {
		if got, err := NormalizeEmail(input); err == nil {
			t.Errorf("NormalizeEmail(%q) = %q, want error", input, got)
		}
	}
}

func TestNormalizePhone(t *testing.T) {
	valid := map[string]string{
		"19912345678":         "19912345678",
		"(19) 91234-5678":     "19912345678",
		"(19) 3123-4567":      "1931234567",
		"+55 (19) 91234-5678": "19912345678",
		"5519912345678":       "19912345678",
	}
	for input, want := range valid {
		got, err := NormalizePhone(input)
		if err != nil || got != want {
			t.Errorf("NormalizePhone(%q) = %q, %v; want %q", input, got, err, want)
		}
	}

	invalid := []string{"", "123", "912345678", "199123456789012", "19 9123a5678", "+1 555 123 4567", "19 9+1234-5678", "١٩٩١٢٣٤٥٦٧٨"}
	for _, input := range invalid {
		if got, err := NormalizePhone(input); err == nil {
			t.Errorf("NormalizePhone(%q) = %q, want error", input, got)
		}
	}
}

func TestNormalizeName(t *testing.T) {
	if got, err := NormalizeName("  Ana Maria "); err != nil || got != "Ana Maria" {
		t.Errorf("NormalizeName = %q, %v", got, err)
	}
	if _, err := NormalizeName(strings.Repeat("á", 120)); err != nil {
		t.Errorf("120-character name rejected: %v", err)
	}
	for _, input := range []string{"", "   ", strings.Repeat("a", 121)} {
		if _, err := NormalizeName(input); err == nil {
			t.Errorf("NormalizeName(%q) accepted", input)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	for _, input := range []string{"12345678", strings.Repeat("a", 72)} {
		if err := ValidatePassword(input); err != nil {
			t.Errorf("ValidatePassword(len %d) = %v", len(input), err)
		}
	}
	for _, input := range []string{"", "1234567", strings.Repeat("a", 73)} {
		if err := ValidatePassword(input); err == nil {
			t.Errorf("ValidatePassword(len %d) accepted", len(input))
		}
	}
}
