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
