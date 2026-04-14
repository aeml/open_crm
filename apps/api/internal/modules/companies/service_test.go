package companies

import "testing"

func TestNormalizeWebsite(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: ""},
		{name: "trim and add https", input: "  atlas.example  ", want: "https://atlas.example"},
		{name: "lowercase host and scheme", input: "HTTP://Atlas.Example/Portal", want: "http://atlas.example/Portal"},
		{name: "drop default https port and root slash", input: "https://Atlas.Example:443/", want: "https://atlas.example"},
		{name: "keep query", input: "atlas.example/pricing?plan=pro", want: "https://atlas.example/pricing?plan=pro"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := normalizeWebsite(test.input); got != test.want {
				t.Fatalf("normalizeWebsite(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

func TestNormalizeCreateInputNormalizesWebsite(t *testing.T) {
	input := normalizeCreateInput(CreateInput{Website: " Atlas.Example "})
	if input.Website != "https://atlas.example" {
		t.Fatalf("expected normalized website, got %q", input.Website)
	}
}

func TestNormalizeUpdateInputNormalizesWebsite(t *testing.T) {
	input := normalizeUpdateInput(UpdateInput{Website: "HTTP://Atlas.Example:80/"})
	if input.Website != "http://atlas.example" {
		t.Fatalf("expected normalized website, got %q", input.Website)
	}
}
