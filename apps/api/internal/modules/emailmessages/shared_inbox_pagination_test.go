package emailmessages

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"math"
	"testing"
	"time"
)

func TestSharedInboxCursorRoundTripPreservesQueueAndPostgresPrecision(t *testing.T) {
	want := SharedInboxCursor{
		SnapshotAt: time.Date(2026, 7, 22, 1, 2, 3, 456789000, time.FixedZone("offset", -5*60*60)),
		Closed:     true,
		MessageAt:  time.Date(2026, 7, 21, 9, 8, 7, 654321000, time.UTC),
		ID:         91,
	}
	encoded, err := encodeSharedInboxCursor(want)
	if err != nil {
		t.Fatalf("encode cursor: %v", err)
	}
	decoded, err := DecodeSharedInboxCursor(encoded)
	if err != nil {
		t.Fatalf("decode cursor: %v", err)
	}
	if !decoded.SnapshotAt.Equal(want.SnapshotAt) || decoded.Closed != want.Closed || !decoded.MessageAt.Equal(want.MessageAt) || decoded.ID != want.ID {
		t.Fatalf("decoded cursor = %+v, want %+v", decoded, want)
	}
}

func TestParseSharedInboxQueryRejectsMalformedCursorAndUnsafeLimits(t *testing.T) {
	for _, input := range []struct{ cursor, limit string }{
		{cursor: "not-a-cursor"},
		{limit: "0"},
		{limit: "101"},
		{limit: "nope"},
	} {
		if _, err := ParseSharedInboxQuery(input.cursor, input.limit); !errors.Is(err, ErrInvalidSharedInboxPage) {
			t.Fatalf("ParseSharedInboxQuery(%q, %q) error = %v, want invalid page", input.cursor, input.limit, err)
		}
	}
}

func TestParseSharedInboxQueryDefaultsAndAcceptsMaximum(t *testing.T) {
	query, err := ParseSharedInboxQuery("", "")
	if err != nil || query.Limit != SharedInboxDefaultLimit || query.Cursor != nil {
		t.Fatalf("default query = %+v, err=%v", query, err)
	}
	query, err = ParseSharedInboxQuery("", "100")
	if err != nil || query.Limit != SharedInboxMaxLimit {
		t.Fatalf("maximum query = %+v, err=%v", query, err)
	}
}

func TestDecodeSharedInboxCursorRejectsInvalidFlagsAndOverflow(t *testing.T) {
	valid := make([]byte, sharedInboxCursorSize)
	valid[0] = sharedInboxCursorV1
	binary.BigEndian.PutUint64(valid[2:10], 1)
	binary.BigEndian.PutUint64(valid[10:18], 1)
	binary.BigEndian.PutUint64(valid[18:26], 1)

	for _, mutate := range []func([]byte){
		func(value []byte) { value[1] = 2 },
		func(value []byte) { binary.BigEndian.PutUint64(value[2:10], uint64(math.MaxInt64)+1) },
		func(value []byte) { binary.BigEndian.PutUint64(value[10:18], uint64(math.MaxInt64)+1) },
		func(value []byte) { binary.BigEndian.PutUint64(value[18:26], uint64(math.MaxInt64)+1) },
	} {
		payload := append([]byte(nil), valid...)
		mutate(payload)
		if _, err := DecodeSharedInboxCursor(base64.RawURLEncoding.EncodeToString(payload)); !errors.Is(err, ErrInvalidSharedInboxPage) {
			t.Fatalf("DecodeSharedInboxCursor(%v) error = %v, want invalid page", payload, err)
		}
	}
}
