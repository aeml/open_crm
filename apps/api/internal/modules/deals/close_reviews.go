package deals

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const maxCloseNotesLength = 2000

var closeReasonsByOutcome = map[string]map[string]string{
	"won": {
		"relationship":    "Existing relationship",
		"solution_fit":    "Best solution fit",
		"price_value":     "Price / value",
		"service_quality": "Service quality",
		"timing":          "Timing",
		"other":           "Other",
	},
	"lost": {
		"budget":       "Budget / price",
		"competitor":   "Competitor",
		"no_decision":  "No decision",
		"timing":       "Timing",
		"scope_fit":    "Scope / solution fit",
		"unresponsive": "Unresponsive",
		"other":        "Other",
	},
}

type closeReview struct {
	Code  string
	Label string
	Notes string
}

func normalizeCloseReview(outcome, code, notes string) (closeReview, error) {
	if outcome == "open" {
		return closeReview{}, nil
	}
	code = strings.ToLower(strings.TrimSpace(code))
	notes = strings.TrimSpace(notes)
	label, ok := closeReasonsByOutcome[outcome][code]
	if !ok || utf8.RuneCountInString(notes) > maxCloseNotesLength {
		return closeReview{}, fmt.Errorf("%w: choose a valid %s reason and keep notes within %d characters", ErrInvalidCloseReview, outcome, maxCloseNotesLength)
	}
	return closeReview{Code: code, Label: label, Notes: notes}, nil
}

func closeActivitySummary(stageName, outcome string, review closeReview) string {
	if outcome == "won" || outcome == "lost" {
		return fmt.Sprintf("Deal moved to %s — %s: %s", stageName, outcome, review.Label)
	}
	return fmt.Sprintf("Deal moved to %s", stageName)
}
