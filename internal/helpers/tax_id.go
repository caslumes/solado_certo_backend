package helpers

import (
	"strings"

	"github.com/caslumes/solado_certo_backend/internal/app"
)

func NormalizeCPF(cpf string) (string, error) {
	var digits strings.Builder
	for _, r := range strings.TrimSpace(cpf) {
		switch {
		case r >= '0' && r <= '9':
			digits.WriteRune(r)
		case r == '.' || r == '-':
		default:
			return "", app.ErrInvalidInput
		}
	}

	normalized := digits.String()
	if len(normalized) != 11 || strings.Count(normalized, normalized[:1]) == 11 {
		return "", app.ErrInvalidInput
	}
	if cpfCheckDigit(normalized[:9]) != normalized[9] || cpfCheckDigit(normalized[:10]) != normalized[10] {
		return "", app.ErrInvalidInput
	}
	return normalized, nil
}

func cpfCheckDigit(digits string) byte {
	sum := 0
	weight := len(digits) + 1
	for i := range len(digits) {
		sum += int(digits[i]-'0') * (weight - i)
	}
	rest := sum % 11
	if rest < 2 {
		return '0'
	}
	return byte('0' + 11 - rest)
}
