package helpers

import (
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/caslumes/solado_certo_backend/internal/app"
)

const (
	maxEmailLength    = 255
	maxNameLength     = 120
	minPasswordLength = 8
	maxPasswordLength = 72
)

func NormalizeEmail(email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || len(email) > maxEmailLength {
		return "", app.ErrInvalidInput
	}

	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email {
		return "", app.ErrInvalidInput
	}

	return email, nil
}

func NormalizePhone(phone string) (string, error) {
	phone = strings.TrimSpace(phone)
	international := strings.HasPrefix(phone, "+")
	if international {
		if !strings.HasPrefix(phone, "+55") {
			return "", app.ErrInvalidInput
		}
		phone = strings.TrimPrefix(phone, "+55")
	}

	var digits strings.Builder
	for _, r := range phone {
		switch {
		case r >= '0' && r <= '9':
			digits.WriteRune(r)
		case strings.ContainsRune(" ()-.", r):
		default:
			return "", app.ErrInvalidInput
		}
	}

	normalized := digits.String()
	if !international && len(normalized) > 11 {
		normalized = strings.TrimPrefix(normalized, "55")
	}

	if len(normalized) != 10 && len(normalized) != 11 {
		return "", app.ErrInvalidInput
	}

	return normalized, nil
}

func NormalizeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxNameLength {
		return "", app.ErrInvalidInput
	}

	return name, nil
}

func ValidatePassword(password string) error {
	if len(password) < minPasswordLength || len(password) > maxPasswordLength {
		return app.ErrInvalidInput
	}

	return nil
}
