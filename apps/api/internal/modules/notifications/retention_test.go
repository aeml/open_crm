package notifications

import (
	"errors"
	"testing"
	"time"
)

func TestNormalizeRetentionPolicy(t *testing.T) {
	defaults, err := normalizeRetentionPolicy(RetentionPolicy{})
	if err != nil || defaults != DefaultRetentionPolicy() {
		t.Fatalf("unexpected default retention policy: policy=%#v err=%v", defaults, err)
	}

	for _, policy := range []RetentionPolicy{
		{ReadFor: -time.Hour, UnreadFor: time.Hour, BatchSize: 1},
		{ReadFor: 48 * time.Hour, UnreadFor: 24 * time.Hour, BatchSize: 1},
		{ReadFor: time.Hour, UnreadFor: 2 * time.Hour, BatchSize: -1},
		{ReadFor: time.Hour, UnreadFor: 2 * time.Hour, BatchSize: maxRetentionBatch + 1},
	} {
		if _, err := normalizeRetentionPolicy(policy); !errors.Is(err, ErrInvalidRetentionPolicy) {
			t.Fatalf("policy %#v error=%v, want invalid policy", policy, err)
		}
	}
}
