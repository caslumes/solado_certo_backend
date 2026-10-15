package helpers

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type Cents int64

var ErrInvalidAmount = errors.New("invalid amount")

var amountPattern = regexp.MustCompile(`^\d{1,13}(\.\d{1,2})?$`)

// ParseCents reads a decimal such as "89.90", "89.9" or "89", as numeric(10,2) is printed by PostgreSQL.
func ParseCents(s string) (Cents, error) {
	if !amountPattern.MatchString(s) {
		return 0, fmt.Errorf("%w: %q", ErrInvalidAmount, s)
	}
	whole, fraction, _ := strings.Cut(s, ".")
	fraction = (fraction + "00")[:2]
	n, err := strconv.ParseInt(whole+fraction, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %q", ErrInvalidAmount, s)
	}
	return Cents(n), nil
}

func MustParseCents(s string) Cents {
	c, err := ParseCents(s)
	if err != nil {
		panic(err)
	}
	return c
}

func (c Cents) Times(quantity int) Cents {
	return c * Cents(quantity)
}

func (c Cents) String() string {
	sign := ""
	if c < 0 {
		sign = "-"
		c = -c
	}
	return fmt.Sprintf("%s%d.%02d", sign, c/100, c%100)
}
