package emailtemplates

import "testing"

func TestRenderSubstitutesKnownFields(t *testing.T) {
	out := Render("Hi {{first_name}}, welcome to {{company}}!", map[string]string{
		"first_name": "Ada",
		"company":    "Acme",
	})
	if out != "Hi Ada, welcome to Acme!" {
		t.Fatalf("unexpected render: %q", out)
	}
}

func TestRenderLeavesUnknownPlaceholders(t *testing.T) {
	out := Render("Hi {{first_name}}, your rep is {{owner}}.", map[string]string{"first_name": "Ada"})
	if out != "Hi Ada, your rep is {{owner}}." {
		t.Fatalf("unknown placeholder should remain: %q", out)
	}
}

func TestRenderIsCaseAndSpaceInsensitive(t *testing.T) {
	out := Render("Hello {{ First_Name }}", map[string]string{"first_name": "Ada"})
	if out != "Hello Ada" {
		t.Fatalf("expected trimmed/case-insensitive match: %q", out)
	}
}

func TestRenderHandlesNoPlaceholders(t *testing.T) {
	if got := Render("plain text", nil); got != "plain text" {
		t.Fatalf("unexpected render: %q", got)
	}
}

func TestRenderHandlesUnclosedBrace(t *testing.T) {
	if got := Render("broken {{first_name", map[string]string{"first_name": "Ada"}); got != "broken {{first_name" {
		t.Fatalf("unclosed placeholder should be left intact: %q", got)
	}
}

func TestUnresolvedTokensReturnsSortedUniqueFields(t *testing.T) {
	got := UnresolvedTokens("Hi {{unknown}} {{ missing }}", "{{unknown}} {{}}")
	if len(got) != 3 || got[0] != "{{ missing }}" || got[1] != "{{unknown}}" || got[2] != "{{}}" {
		t.Fatalf("unexpected unresolved tokens %#v", got)
	}
}

func TestValidateInput(t *testing.T) {
	if err := validateInput(Input{Name: "Welcome", Subject: "Hi", Body: "Body"}); err != nil {
		t.Errorf("valid input should pass: %v", err)
	}
	for _, in := range []Input{
		{Name: "", Subject: "Hi", Body: "B"},
		{Name: "N", Subject: "", Body: "B"},
		{Name: "N", Subject: "Hi", Body: ""},
	} {
		if err := validateInput(normalizeInput(in)); err == nil {
			t.Errorf("expected invalid input for %+v", in)
		}
	}
}
