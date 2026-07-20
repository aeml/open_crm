package mailboxsync

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"strings"

	moduleemail "github.com/aeml/open_crm/apps/api/internal/modules/email"
)

const maxDeliveryFeedbackEvents = 10

// DeliveryFeedback is machine-readable DSN or ARF evidence extracted from a
// connected mailbox message. Values remain untrusted until the storage layer
// correlates the opaque Message-ID with an accepted send in the same tenant
// and mailbox.
type DeliveryFeedback struct {
	Type              string
	OriginalMessageID string
	RecipientEmail    string
	Action            string
	StatusCode        string
}

type deliveryStatusRecipient struct {
	recipient string
	action    string
	status    string
}

func parseDeliveryFeedback(header mail.Header, rawBody []byte) []DeliveryFeedback {
	mediaType, params, err := mime.ParseMediaType(header.Get("Content-Type"))
	if err != nil || !strings.EqualFold(mediaType, "multipart/report") || params["boundary"] == "" {
		return nil
	}
	reportType := strings.ToLower(strings.TrimSpace(params["report-type"]))
	if reportType != "delivery-status" && reportType != "feedback-report" {
		return nil
	}

	var (
		originalMessageID  string
		originalRecipients []string
		dsnRecipients      []deliveryStatusRecipient
		arfType            string
		arfRecipients      []string
	)
	reader := multipartReader(rawBody, params["boundary"])
	for partCount := 0; partCount < 20; partCount++ {
		part, nextErr := reader.NextPart()
		if nextErr != nil {
			break
		}
		partType, _, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
		partBody, readErr := decodedPartBytes(part.Header, part, maxFetchedBodyBytes)
		_ = part.Close()
		if readErr != nil {
			continue
		}
		switch strings.ToLower(partType) {
		case "message/delivery-status":
			blocks := parseMIMEHeaderBlocks(partBody)
			if len(blocks) > 0 && originalMessageID == "" {
				originalMessageID = moduleemail.NormalizeMessageID(blocks[0].Get("Original-Message-ID"))
			}
			for _, block := range blocks[1:] {
				action := strings.ToLower(strings.TrimSpace(block.Get("Action")))
				status := strings.TrimSpace(block.Get("Status"))
				if action != "failed" || !isPermanentDeliveryStatus(status) {
					continue
				}
				recipient := typedMailbox(block.Get("Original-Recipient"))
				if recipient == "" {
					recipient = typedMailbox(block.Get("Final-Recipient"))
				}
				dsnRecipients = append(dsnRecipients, deliveryStatusRecipient{recipient: recipient, action: action, status: status})
			}
		case "message/feedback-report":
			blocks := parseMIMEHeaderBlocks(partBody)
			if len(blocks) == 0 {
				continue
			}
			candidate := strings.ToLower(strings.TrimSpace(blocks[0].Get("Feedback-Type")))
			if candidate == "abuse" || candidate == "fraud" || candidate == "virus" || candidate == "other" {
				arfType = candidate
			}
			for _, value := range headerValues(blocks[0], "Original-Rcpt-To") {
				if recipient := typedMailbox(value); recipient != "" {
					arfRecipients = appendUniqueString(arfRecipients, recipient, maxDeliveryFeedbackEvents)
				}
			}
		case "message/rfc822", "message/global", "text/rfc822-headers":
			messageID, recipients := returnedMessageIdentity(partBody)
			if originalMessageID == "" {
				originalMessageID = messageID
			}
			for _, recipient := range recipients {
				originalRecipients = appendUniqueString(originalRecipients, recipient, maxDeliveryFeedbackEvents)
			}
		}
	}

	events := make([]DeliveryFeedback, 0, maxDeliveryFeedbackEvents)
	if reportType == "delivery-status" {
		for _, recipient := range dsnRecipients {
			events = append(events, DeliveryFeedback{
				Type: "bounce", OriginalMessageID: originalMessageID, RecipientEmail: recipient.recipient,
				Action: recipient.action, StatusCode: recipient.status,
			})
			if len(events) == maxDeliveryFeedbackEvents {
				break
			}
		}
		return deduplicateDeliveryFeedback(events)
	}
	if arfType == "" {
		return nil
	}
	if len(arfRecipients) == 0 {
		arfRecipients = originalRecipients
	}
	if len(arfRecipients) == 0 {
		arfRecipients = []string{""}
	}
	for _, recipient := range arfRecipients {
		events = append(events, DeliveryFeedback{
			Type: "complaint", OriginalMessageID: originalMessageID, RecipientEmail: recipient,
			Action: "reported", StatusCode: arfType,
		})
		if len(events) == maxDeliveryFeedbackEvents {
			break
		}
	}
	return deduplicateDeliveryFeedback(events)
}

func multipartReader(rawBody []byte, boundary string) *multipart.Reader {
	return multipart.NewReader(bytes.NewReader(rawBody), boundary)
}

func decodedPartBytes(header textproto.MIMEHeader, body io.Reader, limit int64) ([]byte, error) {
	var reader io.Reader = body
	switch strings.ToLower(strings.TrimSpace(header.Get("Content-Transfer-Encoding"))) {
	case "base64":
		reader = base64.NewDecoder(base64.StdEncoding, body)
	case "quoted-printable":
		reader = quotedprintable.NewReader(body)
	}
	return io.ReadAll(io.LimitReader(reader, limit))
}

func parseMIMEHeaderBlocks(raw []byte) []textproto.MIMEHeader {
	normalized := strings.ReplaceAll(string(raw), "\r\n", "\n")
	blocks := make([]textproto.MIMEHeader, 0)
	for _, block := range strings.Split(normalized, "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		reader := textproto.NewReader(bufio.NewReader(strings.NewReader(strings.ReplaceAll(block, "\n", "\r\n") + "\r\n\r\n")))
		header, err := reader.ReadMIMEHeader()
		if err == nil && len(header) > 0 {
			blocks = append(blocks, header)
		}
	}
	return blocks
}

func returnedMessageIdentity(raw []byte) (string, []string) {
	message, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return "", nil
	}
	messageID := moduleemail.NormalizeMessageID(message.Header.Get("Message-ID"))
	recipients := make([]string, 0)
	for _, field := range []string{"To", "Cc"} {
		addresses, parseErr := mail.ParseAddressList(message.Header.Get(field))
		if parseErr != nil {
			continue
		}
		for _, address := range addresses {
			if recipient := normalizedMailbox(address.Address); recipient != "" {
				recipients = appendUniqueString(recipients, recipient, maxDeliveryFeedbackEvents)
			}
		}
	}
	return messageID, recipients
}

func typedMailbox(value string) string {
	if _, address, ok := strings.Cut(strings.TrimSpace(value), ";"); ok {
		value = address
	}
	return normalizedMailbox(strings.Trim(strings.TrimSpace(value), "<>"))
}

func normalizedMailbox(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	parsed, err := mail.ParseAddress(value)
	if err != nil || strings.ToLower(strings.TrimSpace(parsed.Address)) != value || len(value) > 320 {
		return ""
	}
	return value
}

func headerValues(header textproto.MIMEHeader, name string) []string {
	for key, values := range header {
		if strings.EqualFold(key, name) {
			return values
		}
	}
	return nil
}

func appendUniqueString(values []string, value string, limit int) []string {
	if value == "" || len(values) >= limit {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func deduplicateDeliveryFeedback(input []DeliveryFeedback) []DeliveryFeedback {
	result := make([]DeliveryFeedback, 0, min(len(input), maxDeliveryFeedbackEvents))
	seen := make(map[string]struct{})
	for _, feedback := range input {
		key := feedback.Type + "\x00" + feedback.OriginalMessageID + "\x00" + feedback.RecipientEmail
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, feedback)
		if len(result) == maxDeliveryFeedbackEvents {
			break
		}
	}
	return result
}

func isPermanentDeliveryStatus(value string) bool {
	parts := strings.Split(strings.TrimSpace(value), ".")
	if len(parts) != 3 || parts[0] != "5" {
		return false
	}
	for _, part := range parts[1:] {
		if len(part) == 0 || len(part) > 3 {
			return false
		}
		for _, character := range part {
			if character < '0' || character > '9' {
				return false
			}
		}
	}
	return true
}
