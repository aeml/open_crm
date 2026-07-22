package emailmessages

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

const (
	SharedInboxDefaultLimit = 50
	SharedInboxMaxLimit     = 100
	sharedInboxCursorSize   = 26
	sharedInboxCursorV1     = byte(1)
)

var ErrInvalidSharedInboxPage = errors.New("invalid shared inbox pagination")

// SharedInboxCursor retains both the queue position and the first-page
// snapshot. Shared-inbox status is mutable, so the snapshot boundary prevents
// a message coordinated after page one from moving into a later page and
// appearing twice. Refreshing deliberately creates a new snapshot.
type SharedInboxCursor struct {
	SnapshotAt time.Time
	Closed     bool
	MessageAt  time.Time
	ID         int64
}

type SharedInboxQuery struct {
	Cursor *SharedInboxCursor
	Limit  int
}

type SharedInboxPageMeta struct {
	Limit      int    `json:"limit"`
	HasMore    bool   `json:"hasMore"`
	NextCursor string `json:"nextCursor"`
}

type SharedInboxPage struct {
	Messages []Message           `json:"messages"`
	Meta     SharedInboxPageMeta `json:"meta"`
}

// ListSharedInbox returns one stable page of shared inbound messages available
// to the team inbox. A continuation retains the first-page snapshot because a
// coordination change can move a message between the open and closed buckets.
func (s *Service) ListSharedInbox(ctx context.Context, organizationID int64, query SharedInboxQuery) (SharedInboxPage, error) {
	if s == nil || s.pool == nil {
		return SharedInboxPage{}, fmt.Errorf("email messages service not configured")
	}
	if organizationID <= 0 {
		return SharedInboxPage{}, ErrInvalidInput
	}
	query, err := normalizeSharedInboxQuery(query)
	if err != nil {
		return SharedInboxPage{}, err
	}

	var snapshotAt time.Time
	if query.Cursor != nil {
		snapshotAt = query.Cursor.SnapshotAt.UTC()
	} else if err := s.pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&snapshotAt); err != nil {
		return SharedInboxPage{}, fmt.Errorf("capture shared inbox snapshot: %w", err)
	}

	args := []any{organizationID, snapshotAt}
	cursorFilter := ""
	if query.Cursor != nil {
		closedBucket := 0
		if query.Cursor.Closed {
			closedBucket = 1
		}
		args = append(args, closedBucket, query.Cursor.MessageAt, query.Cursor.ID)
		cursorFilter = `
		  AND (
		    CASE WHEN m.shared_inbox_status = 'open' THEN 0 ELSE 1 END > $3
		    OR (
		      CASE WHEN m.shared_inbox_status = 'open' THEN 0 ELSE 1 END = $3
		      AND (COALESCE(m.received_at, m.created_at), m.id) < ($4, $5)
		    )
		  )`
	}
	args = append(args, query.Limit+1)
	rows, err := s.pool.Query(ctx, baseSelect+`
		WHERE m.organization_id = $1
		  AND m.direction = 'inbound'
		  AND m.visibility = 'shared'
		  AND COALESCE(m.shared_inbox_updated_at, m.created_at) < $2`+cursorFilter+`
		ORDER BY CASE WHEN m.shared_inbox_status = 'open' THEN 0 ELSE 1 END,
		         COALESCE(m.received_at, m.created_at) DESC,
		         m.id DESC
		LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return SharedInboxPage{}, fmt.Errorf("list shared inbox email messages: %w", err)
	}
	defer rows.Close()
	messages, err := scanMessages(rows)
	if err != nil {
		return SharedInboxPage{}, err
	}
	hasMore := len(messages) > query.Limit
	if hasMore {
		messages = messages[:query.Limit]
	}
	meta := SharedInboxPageMeta{Limit: query.Limit}
	if len(messages) > 0 {
		meta, err = sharedInboxMetaForPage(query.Limit, hasMore, snapshotAt, messages[len(messages)-1])
		if err != nil {
			return SharedInboxPage{}, err
		}
	}
	return SharedInboxPage{Messages: messages, Meta: meta}, nil
}

func ParseSharedInboxQuery(rawCursor, rawLimit string) (SharedInboxQuery, error) {
	limit := SharedInboxDefaultLimit
	if strings.TrimSpace(rawLimit) != "" {
		parsed, err := strconv.Atoi(strings.TrimSpace(rawLimit))
		if err != nil || parsed < 1 || parsed > SharedInboxMaxLimit {
			return SharedInboxQuery{}, ErrInvalidSharedInboxPage
		}
		limit = parsed
	}

	query := SharedInboxQuery{Limit: limit}
	if strings.TrimSpace(rawCursor) == "" {
		return query, nil
	}
	cursor, err := DecodeSharedInboxCursor(rawCursor)
	if err != nil {
		return SharedInboxQuery{}, err
	}
	query.Cursor = &cursor
	return query, nil
}

func normalizeSharedInboxQuery(query SharedInboxQuery) (SharedInboxQuery, error) {
	if query.Limit == 0 {
		query.Limit = SharedInboxDefaultLimit
	}
	if query.Limit < 1 || query.Limit > SharedInboxMaxLimit {
		return SharedInboxQuery{}, ErrInvalidSharedInboxPage
	}
	if query.Cursor != nil && (query.Cursor.SnapshotAt.IsZero() || query.Cursor.MessageAt.IsZero() || query.Cursor.ID <= 0) {
		return SharedInboxQuery{}, ErrInvalidSharedInboxPage
	}
	return query, nil
}

func encodeSharedInboxCursor(cursor SharedInboxCursor) (string, error) {
	if cursor.SnapshotAt.IsZero() || cursor.MessageAt.IsZero() || cursor.ID <= 0 {
		return "", ErrInvalidSharedInboxPage
	}
	snapshotMicros := cursor.SnapshotAt.UTC().UnixMicro()
	messageMicros := cursor.MessageAt.UTC().UnixMicro()
	if snapshotMicros <= 0 || messageMicros <= 0 {
		return "", ErrInvalidSharedInboxPage
	}

	payload := make([]byte, sharedInboxCursorSize)
	payload[0] = sharedInboxCursorV1
	if cursor.Closed {
		payload[1] = 1
	}
	binary.BigEndian.PutUint64(payload[2:10], uint64(snapshotMicros))
	binary.BigEndian.PutUint64(payload[10:18], uint64(messageMicros))
	binary.BigEndian.PutUint64(payload[18:26], uint64(cursor.ID))
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func DecodeSharedInboxCursor(value string) (SharedInboxCursor, error) {
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil || len(payload) != sharedInboxCursorSize || payload[0] != sharedInboxCursorV1 || payload[1] > 1 {
		return SharedInboxCursor{}, ErrInvalidSharedInboxPage
	}
	snapshotValue := binary.BigEndian.Uint64(payload[2:10])
	messageValue := binary.BigEndian.Uint64(payload[10:18])
	idValue := binary.BigEndian.Uint64(payload[18:26])
	if snapshotValue == 0 || snapshotValue > math.MaxInt64 || messageValue == 0 || messageValue > math.MaxInt64 || idValue == 0 || idValue > math.MaxInt64 {
		return SharedInboxCursor{}, ErrInvalidSharedInboxPage
	}
	return SharedInboxCursor{
		SnapshotAt: time.UnixMicro(int64(snapshotValue)).UTC(),
		Closed:     payload[1] == 1,
		MessageAt:  time.UnixMicro(int64(messageValue)).UTC(),
		ID:         int64(idValue),
	}, nil
}

func sharedInboxMetaForPage(limit int, hasMore bool, snapshotAt time.Time, last Message) (SharedInboxPageMeta, error) {
	meta := SharedInboxPageMeta{Limit: limit, HasMore: hasMore}
	if !hasMore {
		return meta, nil
	}
	next, err := encodeSharedInboxCursor(SharedInboxCursor{
		SnapshotAt: snapshotAt,
		Closed:     last.SharedInboxStatus == "closed",
		MessageAt:  sharedInboxMessageTime(last),
		ID:         last.ID,
	})
	if err != nil {
		return SharedInboxPageMeta{}, err
	}
	meta.NextCursor = next
	return meta, nil
}

func sharedInboxMessageTime(message Message) time.Time {
	if message.ReceivedAt != nil {
		return *message.ReceivedAt
	}
	return message.CreatedAt
}
