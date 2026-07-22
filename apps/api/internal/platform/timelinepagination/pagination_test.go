package timelinepagination

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"math"
	"testing"
	"time"
)

func TestCursorRoundTripPreservesPostgreSQLPrecision(t *testing.T) {
	wantTime := time.Date(2026, 7, 22, 1, 2, 3, 456789000, time.FixedZone("offset", -5*60*60))
	encoded, err := Encode(wantTime, 91)
	if err != nil {
		t.Fatalf("encode cursor: %v", err)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("decode cursor: %v", err)
	}
	if !decoded.CreatedAt.Equal(wantTime) || decoded.ID != 91 {
		t.Fatalf("decoded cursor = %+v, want time %s id 91", decoded, wantTime)
	}
}

func TestParseRejectsMalformedCursorAndUnsafeLimits(t *testing.T) {
	for _, input := range []struct{ cursor, limit string }{
		{cursor: "not-a-cursor"},
		{limit: "0"},
		{limit: "101"},
		{limit: "nope"},
	} {
		if _, err := Parse(input.cursor, input.limit); !errors.Is(err, ErrInvalid) {
			t.Fatalf("Parse(%q, %q) error = %v, want ErrInvalid", input.cursor, input.limit, err)
		}
	}
}

func TestParseDefaultsAndAcceptsMaximumLimit(t *testing.T) {
	query, err := Parse("", "")
	if err != nil || query.Limit != DefaultLimit || query.Cursor != nil {
		t.Fatalf("default query = %+v, err=%v", query, err)
	}
	query, err = Parse("", "100")
	if err != nil || query.Limit != MaxLimit {
		t.Fatalf("maximum query = %+v, err=%v", query, err)
	}
}

func TestDecodeRejectsUnsignedValuesOutsideSignedCursorRange(t *testing.T) {
	for _, overflowingField := range []string{"created_at", "id"} {
		payload := make([]byte, cursorSize)
		payload[0] = cursorV1
		binary.BigEndian.PutUint64(payload[1:9], 1)
		binary.BigEndian.PutUint64(payload[9:17], 1)
		if overflowingField == "created_at" {
			binary.BigEndian.PutUint64(payload[1:9], uint64(math.MaxInt64)+1)
		} else {
			binary.BigEndian.PutUint64(payload[9:17], uint64(math.MaxInt64)+1)
		}
		encoded := base64.RawURLEncoding.EncodeToString(payload)
		if _, err := Decode(encoded); !errors.Is(err, ErrInvalid) {
			t.Fatalf("Decode overflow in %s error = %v, want ErrInvalid", overflowingField, err)
		}
	}
}
