package notes

import (
	"reflect"
	"testing"
)

func TestMentionedEmailsAreExplicitCaseInsensitiveAndDeduplicated(t *testing.T) {
	body := "Ask @Casey.Example+crm@example.test, then @casey.example+crm@EXAMPLE.TEST. Plain name @casey is not a mention."
	want := []string{"casey.example+crm@example.test"}
	if got := mentionedEmails(body); !reflect.DeepEqual(got, want) {
		t.Fatalf("mentionedEmails() = %#v, want %#v", got, want)
	}
}
