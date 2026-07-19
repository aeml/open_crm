package deals

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeCloseReviewUsesOutcomeSpecificFixedReasons(t *testing.T) {
	won, err := normalizeCloseReview("won", " SOLUTION_FIT ", "  Clear fit.  ")
	if err != nil || won.Code != "solution_fit" || won.Label != "Best solution fit" || won.Notes != "Clear fit." {
		t.Fatalf("unexpected won review: review=%#v err=%v", won, err)
	}
	lost, err := normalizeCloseReview("lost", "competitor", "")
	if err != nil || lost.Label != "Competitor" {
		t.Fatalf("unexpected lost review: review=%#v err=%v", lost, err)
	}
	for _, input := range []struct{ outcome, code, notes string }{
		{"won", "competitor", ""},
		{"lost", "solution_fit", ""},
		{"lost", "", ""},
		{"won", "other", strings.Repeat("x", maxCloseNotesLength+1)},
	} {
		if _, err := normalizeCloseReview(input.outcome, input.code, input.notes); !errors.Is(err, ErrInvalidCloseReview) {
			t.Fatalf("expected invalid close review for %#v, got %v", input, err)
		}
	}
	open, err := normalizeCloseReview("open", "competitor", "stale")
	if err != nil || open != (closeReview{}) {
		t.Fatalf("open transition did not clear close context: review=%#v err=%v", open, err)
	}
}
