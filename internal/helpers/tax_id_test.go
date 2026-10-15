package helpers

import "testing"

func TestNormalizeCPF(t *testing.T) {
	valid := map[string]string{
		"529.982.247-25":   "52998224725",
		"52998224725":      "52998224725",
		" 111.444.777-35 ": "11144477735",
		"000.000.001-91":   "00000000191",
	}
	for input, want := range valid {
		got, err := NormalizeCPF(input)
		if err != nil || got != want {
			t.Errorf("NormalizeCPF(%q) = %q, %v, want %q", input, got, err, want)
		}
	}

	invalid := []string{"", "529.982.247-24", "529.982.247-15", "5299822472", "529982247255", "111.111.111-11", "00000000000", "529 982 247 25", "52998224a25"}
	for _, input := range invalid {
		if got, err := NormalizeCPF(input); err == nil {
			t.Errorf("NormalizeCPF(%q) = %q, want an error", input, got)
		}
	}
}
