package mailboxsync

import moduleemail "github.com/aeml/open_crm/apps/api/internal/modules/email"

func appendMessageIDReferences(existing []string, additions ...string) []string {
	seen := make(map[string]struct{}, len(existing)+len(additions))
	result := make([]string, 0, min(len(existing)+len(additions), moduleemail.MaxMessageIDReferences))
	for _, value := range append(existing, additions...) {
		value = moduleemail.NormalizeMessageID(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
		if len(result) == moduleemail.MaxMessageIDReferences {
			break
		}
	}
	return result
}
