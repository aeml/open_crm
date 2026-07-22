package timelinepagination

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultLimit = 50
	MaxLimit     = 100
	cursorSize   = 17
	cursorV1     = byte(1)
)

var ErrInvalid = errors.New("invalid timeline pagination")

type Cursor struct {
	CreatedAt time.Time
	ID        int64
}

type Query struct {
	Cursor *Cursor
	Limit  int
}

type Meta struct {
	Limit      int    `json:"limit"`
	HasMore    bool   `json:"hasMore"`
	NextCursor string `json:"nextCursor"`
}

func Parse(rawCursor, rawLimit string) (Query, error) {
	limit := DefaultLimit
	if strings.TrimSpace(rawLimit) != "" {
		parsed, err := strconv.Atoi(strings.TrimSpace(rawLimit))
		if err != nil || parsed <= 0 || parsed > MaxLimit {
			return Query{}, ErrInvalid
		}
		limit = parsed
	}

	query := Query{Limit: limit}
	rawCursor = strings.TrimSpace(rawCursor)
	if rawCursor == "" {
		return query, nil
	}
	cursor, err := Decode(rawCursor)
	if err != nil {
		return Query{}, err
	}
	query.Cursor = &cursor
	return query, nil
}

func Normalize(query Query) (Query, error) {
	if query.Limit == 0 {
		query.Limit = DefaultLimit
	}
	if query.Limit < 1 || query.Limit > MaxLimit {
		return Query{}, ErrInvalid
	}
	if query.Cursor != nil && (query.Cursor.ID <= 0 || query.Cursor.CreatedAt.IsZero()) {
		return Query{}, ErrInvalid
	}
	return query, nil
}

func Encode(createdAt time.Time, id int64) (string, error) {
	if createdAt.IsZero() || id <= 0 {
		return "", ErrInvalid
	}
	payload := make([]byte, cursorSize)
	payload[0] = cursorV1
	binary.BigEndian.PutUint64(payload[1:9], uint64(createdAt.UTC().UnixMicro()))
	binary.BigEndian.PutUint64(payload[9:17], uint64(id))
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func Decode(value string) (Cursor, error) {
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil || len(payload) != cursorSize || payload[0] != cursorV1 {
		return Cursor{}, ErrInvalid
	}
	microsValue := binary.BigEndian.Uint64(payload[1:9])
	idValue := binary.BigEndian.Uint64(payload[9:17])
	if microsValue == 0 || microsValue > math.MaxInt64 || idValue == 0 || idValue > math.MaxInt64 {
		return Cursor{}, ErrInvalid
	}
	micros := int64(microsValue)
	id := int64(idValue)
	return Cursor{CreatedAt: time.UnixMicro(micros).UTC(), ID: id}, nil
}

func MetaForPage(limit int, hasMore bool, createdAt time.Time, id int64) (Meta, error) {
	meta := Meta{Limit: limit, HasMore: hasMore}
	if !hasMore {
		return meta, nil
	}
	next, err := Encode(createdAt, id)
	if err != nil {
		return Meta{}, err
	}
	meta.NextCursor = next
	return meta, nil
}
