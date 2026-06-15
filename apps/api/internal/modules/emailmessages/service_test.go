package emailmessages

import "testing"

func TestSanitizedEntityLinksKeepsValidUniqueLinks(t *testing.T) {
	links := sanitizedEntityLinks([]EntityLinkInput{
		{EntityType: " Contact ", EntityID: 7},
		{EntityType: "contact", EntityID: 7},
		{EntityType: "company", EntityID: 9},
		{EntityType: "deal", EntityID: 11},
		{EntityType: "task", EntityID: 13},
		{EntityType: "contact", EntityID: 0},
	})

	if len(links) != 3 {
		t.Fatalf("expected three valid unique links, got %#v", links)
	}
	if links[0] != (EntityLinkInput{EntityType: "contact", EntityID: 7}) {
		t.Fatalf("unexpected first link: %#v", links[0])
	}
	if links[1] != (EntityLinkInput{EntityType: "company", EntityID: 9}) || links[2] != (EntityLinkInput{EntityType: "deal", EntityID: 11}) {
		t.Fatalf("unexpected remaining links: %#v", links)
	}
}

func TestContactEntityIDsReturnsUniqueContactLinksOnly(t *testing.T) {
	ids := contactEntityIDs([]EntityLinkInput{
		{EntityType: "contact", EntityID: 7},
		{EntityType: "company", EntityID: 9},
		{EntityType: "contact", EntityID: 7},
		{EntityType: "deal", EntityID: 11},
		{EntityType: "contact", EntityID: 12},
	})

	if len(ids) != 2 || ids[0] != 7 || ids[1] != 12 {
		t.Fatalf("unexpected contact IDs: %#v", ids)
	}
}

func TestNormalizedVisibilityUsesValidValueOrFallback(t *testing.T) {
	cases := []struct {
		name     string
		value    string
		fallback string
		expected string
	}{
		{name: "shared value", value: " shared ", fallback: "private", expected: "shared"},
		{name: "private value", value: "PRIVATE", fallback: "shared", expected: "private"},
		{name: "private fallback", value: "", fallback: "private", expected: "private"},
		{name: "invalid fallback defaults shared", value: "team", fallback: "org", expected: "shared"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizedVisibility(tc.value, tc.fallback); got != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}
