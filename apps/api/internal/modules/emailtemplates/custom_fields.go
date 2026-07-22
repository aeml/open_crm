package emailtemplates

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"

	modulecustomfields "github.com/aeml/open_crm/apps/api/internal/modules/customfields"
)

// AddCustomMergeFields adds active organization-defined values under a
// collision-safe namespace such as contact.custom.region. Missing values are
// present as empty strings so a known field never leaks its raw merge token to
// a recipient.
func AddCustomMergeFields(fields map[string]string, namespace string, definitions []modulecustomfields.Definition, values modulecustomfields.Values) {
	namespace = strings.ToLower(strings.TrimSpace(namespace))
	if fields == nil || namespace == "" {
		return
	}
	for _, definition := range definitions {
		if definition.ArchivedAt != nil || definition.FieldKey == "" {
			continue
		}
		fields[namespace+".custom."+definition.FieldKey] = customMergeValue(values[definition.FieldKey])
	}
}

func customMergeValue(raw json.RawMessage) string {
	if len(bytes.TrimSpace(raw)) == 0 {
		return ""
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case bool:
		return strconv.FormatBool(typed)
	default:
		return ""
	}
}
