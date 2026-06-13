package emailtemplates

import "strings"

// Render substitutes {{field}} placeholders in a template string with values
// from fields. Unknown placeholders are left untouched so missing data is
// visible rather than silently blanked. Field lookups are case-insensitive and
// tolerate surrounding whitespace inside the braces.
func Render(template string, fields map[string]string) string {
	normalized := make(map[string]string, len(fields))
	for key, value := range fields {
		normalized[strings.ToLower(strings.TrimSpace(key))] = value
	}

	var b strings.Builder
	for {
		start := strings.Index(template, "{{")
		if start < 0 {
			b.WriteString(template)
			break
		}
		end := strings.Index(template[start:], "}}")
		if end < 0 {
			b.WriteString(template)
			break
		}
		end += start
		b.WriteString(template[:start])
		key := strings.ToLower(strings.TrimSpace(template[start+2 : end]))
		if value, ok := normalized[key]; ok {
			b.WriteString(value)
		} else {
			b.WriteString(template[start : end+2])
		}
		template = template[end+2:]
	}
	return b.String()
}
