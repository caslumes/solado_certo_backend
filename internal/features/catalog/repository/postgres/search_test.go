package repository

import (
	"testing"

	"github.com/caslumes/solado_certo_backend/internal/features/catalog"
	"github.com/caslumes/solado_certo_backend/internal/helpers"
)

func TestSearchQuery(t *testing.T) {
	tests := map[string]struct {
		query *string
		want  string
	}{
		"absent":                      {nil, ""},
		"one word":                    {helpers.StringPtr("palm"), "palm:*"},
		"words are all required":      {helpers.StringPtr("  palmilha   silicone "), "palmilha:* & silicone:*"},
		"accents are kept for the db": {helpers.StringPtr("pé diabético"), "pé:* & diabético:*"},
		"operators are dropped":       {helpers.StringPtr("palmilha & !(gel) | 'x':*"), "palmilha:* & gel:* & x:*"},
		"no words":                    {helpers.StringPtr("!!! ---"), ""},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := searchQuery(catalog.AdvertisementFilters{Query: tt.query}); got != tt.want {
				t.Fatalf("searchQuery = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAnyWordQuery(t *testing.T) {
	tests := map[string]string{
		"Fascite plantar":        "Fascite | plantar",
		"dor no pé":              "",
		"tenho joanete, dói!":    "tenho | joanete",
		"pé diabético & (x | y)": "diabético",
	}
	for text, want := range tests {
		t.Run(text, func(t *testing.T) {
			if got := anyWordQuery(text); got != want {
				t.Fatalf("anyWordQuery = %q, want %q", got, want)
			}
		})
	}
}
