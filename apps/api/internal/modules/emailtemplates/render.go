package emailtemplates

import (
	"sort"
	"strings"
)

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

// UnresolvedTokens returns the distinct placeholders that remain after
// rendering. It gives previews a fail-visible warning without treating an
// intentionally unknown token as an empty value.
func UnresolvedTokens(values ...string) []string {
	found := map[string]struct{}{}
	for _, value := range values {
		for {
			start := strings.Index(value, "{{")
			if start < 0 {
				break
			}
			endOffset := strings.Index(value[start+2:], "}}")
			if endOffset < 0 {
				break
			}
			end := start + 2 + endOffset
			token := strings.TrimSpace(value[start : end+2])
			found[token] = struct{}{}
			value = value[end+2:]
		}
	}
	tokens := make([]string, 0, len(found))
	for token := range found {
		tokens = append(tokens, token)
	}
	sort.Strings(tokens)
	return tokens
}
