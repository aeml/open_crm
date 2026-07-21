package quotetemplates

import "testing"

func TestValidateAndRenderQuoteTemplate(t *testing.T) {
	input := normalizeInput(Input{
		Name: " Standard services ", Terms: "Net 30.", DefaultValidityDays: 30,
		DeliverySubjectTemplate: "Quote {{quote_number}} for {{deal_name}}",
		DeliveryMessageTemplate: "Hi {{recipient_name}}, review {{currency}} {{total}} by {{valid_until}}.",
	})
	if err := validateInput(input); err != nil {
		t.Fatalf("expected valid quote template: %v", err)
	}
	values := MergeValues{QuoteNumber: "Q-7-V2", RecipientName: "Avery", DealName: "Pilot", Currency: "USD", Total: "125.00", ValidUntil: "2026-08-20"}
	if rendered := Render(input.DeliverySubjectTemplate, values); rendered != "Quote Q-7-V2 for Pilot" {
		t.Fatalf("unexpected rendered subject %q", rendered)
	}
	if rendered := Render(input.DeliveryMessageTemplate, values); rendered != "Hi Avery, review USD 125.00 by 2026-08-20." {
		t.Fatalf("unexpected rendered message %q", rendered)
	}
}

func TestRejectsUnknownOrMalformedQuoteTemplateTokens(t *testing.T) {
	base := Input{Name: "Terms", Terms: "Net 30", DefaultValidityDays: 30, DeliverySubjectTemplate: "Quote {{quote_number}}", DeliveryMessageTemplate: "Hi {{recipient_name}}"}
	for _, value := range []string{"{{unknown}}", "{{quote_number", "quote_number}}", "{{Quote_Number}}"} {
		input := base
		input.DeliveryMessageTemplate = value
		if err := validateInput(input); err == nil {
			t.Fatalf("expected invalid token %q", value)
		}
	}
}

func TestMergeTokensAreStable(t *testing.T) {
	tokens := MergeTokens()
	if len(tokens) != 6 || tokens[0] != "{{quote_number}}" || tokens[5] != "{{valid_until}}" {
		t.Fatalf("unexpected quote merge tokens %#v", tokens)
	}
}
