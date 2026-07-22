package pagination

import (
	"errors"
	"math"
	"testing"
)

func TestParseUsesDefaultsAndReturnsExactOffset(t *testing.T) {
	page, err := Parse("", "", 20)
	if err != nil || page != (Page{Number: 1, Size: 20, Offset: 0}) {
		t.Fatalf("unexpected default page: %#v err=%v", page, err)
	}
	page, err = Parse("3", "100", 20)
	if err != nil || page != (Page{Number: 3, Size: 100, Offset: 200}) {
		t.Fatalf("unexpected explicit page: %#v err=%v", page, err)
	}
}

func TestParseRejectsMalformedOversizedAndDeepPages(t *testing.T) {
	for _, input := range []struct {
		page     string
		pageSize string
	}{
		{page: "nope", pageSize: "20"},
		{page: "0", pageSize: "20"},
		{page: "1", pageSize: "101"},
		{page: "502", pageSize: "100"},
	} {
		if _, err := Parse(input.page, input.pageSize, 20); !errors.Is(err, ErrInvalid) {
			t.Fatalf("Parse(%q, %q) error=%v; expected ErrInvalid", input.page, input.pageSize, err)
		}
	}
}

func TestNormalizeDefaultsInternalCallersAndRejectsOverflow(t *testing.T) {
	page, err := Normalize(0, 0, 20)
	if err != nil || page != (Page{Number: 1, Size: 20, Offset: 0}) {
		t.Fatalf("unexpected normalized page: %#v err=%v", page, err)
	}
	if _, err := Normalize(math.MaxInt, 100, 20); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected extreme page to fail safely, got %v", err)
	}
}
