package emailtemplates

import (
	"encoding/json"
	"testing"
	"time"

	modulecustomfields "github.com/aeml/open_crm/apps/api/internal/modules/customfields"
)

func TestMergeFieldCatalogAndValuesUseActiveNamespacedCustomFields(t *testing.T) {
	archivedAt := time.Now()
	contactDefinitions := []modulecustomfields.Definition{
		{FieldKey: "region", Label: "Region", DataType: "select"},
		{FieldKey: "annual_value", Label: "Annual value", DataType: "number"},
		{FieldKey: "former_tier", Label: "Former tier", DataType: "text", ArchivedAt: &archivedAt},
	}
	companyDefinitions := []modulecustomfields.Definition{{FieldKey: "priority", Label: "Priority", DataType: "boolean"}}
	groups := MergeFieldCatalogWithCustomFields(contactDefinitions, companyDefinitions)
	if len(groups) != len(MergeFieldCatalog())+2 {
		t.Fatalf("unexpected catalog groups: %#v", groups)
	}
	if token := groups[len(groups)-2].Fields[0].Token; token != "{{contact.custom.region}}" {
		t.Fatalf("unexpected contact custom token %q", token)
	}
	if token := groups[len(groups)-1].Fields[0].Token; token != "{{company.custom.priority}}" {
		t.Fatalf("unexpected company custom token %q", token)
	}
	for _, field := range groups[len(groups)-2].Fields {
		if field.Token == "{{contact.custom.former_tier}}" {
			t.Fatal("archived custom field was exposed in merge catalog")
		}
	}

	fields := map[string]string{}
	AddCustomMergeFields(fields, "contact", contactDefinitions, modulecustomfields.Values{
		"region":       json.RawMessage(`"West"`),
		"annual_value": json.RawMessage(`1250.50`),
		"former_tier":  json.RawMessage(`"Legacy"`),
	})
	AddCustomMergeFields(fields, "company", companyDefinitions, modulecustomfields.Values{"priority": json.RawMessage(`true`)})
	rendered := Render("{{contact.custom.region}}|{{contact.custom.annual_value}}|{{company.custom.priority}}|{{contact.custom.former_tier}}", fields)
	if rendered != "West|1250.50|true|{{contact.custom.former_tier}}" {
		t.Fatalf("unexpected custom merge output %q", rendered)
	}
}

func TestAddCustomMergeFieldsBlanksKnownMissingOrInvalidValues(t *testing.T) {
	definitions := []modulecustomfields.Definition{
		{FieldKey: "missing", Label: "Missing"},
		{FieldKey: "broken", Label: "Broken"},
	}
	fields := map[string]string{}
	AddCustomMergeFields(fields, "contact", definitions, modulecustomfields.Values{"broken": json.RawMessage(`{`)})
	if rendered := Render("{{contact.custom.missing}}/{{contact.custom.broken}}", fields); rendered != "/" {
		t.Fatalf("known empty custom values should render blank, got %q", rendered)
	}
}
