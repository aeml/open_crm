package emailtemplates

import (
	"errors"
	"strings"
	"testing"
)

func TestEmailDefinitionContentBounds(t *testing.T) {
	validTemplate := Input{
		Name:    strings.Repeat("n", MaxTemplateNameLength),
		Subject: strings.Repeat("s", MaxTemplateSubjectLen),
		Body:    strings.Repeat("b", MaxTemplateBodyLength),
	}
	if err := validateInput(validTemplate); err != nil {
		t.Fatalf("maximum valid template rejected: %v", err)
	}
	for field, input := range map[string]Input{
		"name":    {Name: validTemplate.Name + "x", Subject: "subject", Body: "body"},
		"subject": {Name: "name", Subject: validTemplate.Subject + "x", Body: "body"},
		"body":    {Name: "name", Subject: "subject", Body: validTemplate.Body + "x"},
	} {
		if err := validateInput(input); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("oversized template %s returned %v", field, err)
		}
	}

	validSnippet := SnippetInput{Name: strings.Repeat("n", MaxSnippetNameLength), Body: strings.Repeat("b", MaxSnippetBodyLength)}
	if err := validateSnippetInput(validSnippet); err != nil {
		t.Fatalf("maximum valid snippet rejected: %v", err)
	}
	for field, input := range map[string]SnippetInput{
		"name": {Name: validSnippet.Name + "x", Body: "body"},
		"body": {Name: "name", Body: validSnippet.Body + "x"},
	} {
		if err := validateSnippetInput(input); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("oversized snippet %s returned %v", field, err)
		}
	}
}
