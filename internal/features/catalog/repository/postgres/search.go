package repository

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/caslumes/solado_certo_backend/internal/features/catalog"
)

const textSearchConfig = "portuguese_unaccent"

const minProfileWordLength = 4

var wordPattern = regexp.MustCompile(`[\p{L}\p{N}]+`)

func searchQuery(filters catalog.AdvertisementFilters) string {
	if filters.Query == nil {
		return ""
	}
	words := wordPattern.FindAllString(*filters.Query, -1)
	for i, word := range words {
		words[i] = word + ":*"
	}
	return strings.Join(words, " & ")
}

func anyWordQuery(text string) string {
	var words []string
	for _, word := range wordPattern.FindAllString(text, -1) {
		if utf8.RuneCountInString(word) >= minProfileWordLength {
			words = append(words, word)
		}
	}
	return strings.Join(words, " | ")
}
