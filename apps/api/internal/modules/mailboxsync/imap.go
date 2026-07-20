package mailboxsync

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"time"

	moduleemail "github.com/aeml/open_crm/apps/api/internal/modules/email"
	moduleuseremail "github.com/aeml/open_crm/apps/api/internal/modules/useremail"
)

const maxFetchedBodyBytes = 512 * 1024

var (
	imapLiteralRe      = regexp.MustCompile(`\{(\d+)\}\s*$`)
	imapUIDRe          = regexp.MustCompile(`\bUID\s+(\d+)\b`)
	imapInternalDateRe = regexp.MustCompile(`\bINTERNALDATE\s+"([^"]+)"`)
)

type IMAPFetcher struct{}

func NewIMAPFetcher() *IMAPFetcher {
	return &IMAPFetcher{}
}

func (f *IMAPFetcher) Fetch(ctx context.Context, creds moduleuseremail.SyncCredentials, limit int) ([]FetchedMessage, error) {
	if limit <= 0 || limit > 100 {
		limit = defaultFetchLimit
	}
	conn, err := dialIMAP(ctx, creds)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	client := &imapClient{reader: bufio.NewReader(conn), writer: conn}
	if _, err := client.readLine(); err != nil {
		return nil, fmt.Errorf("read imap greeting: %w", err)
	}
	if _, err := client.command("LOGIN %s %s", imapQuote(creds.IMAPUsername), imapQuote(creds.IMAPPassword)); err != nil {
		return nil, fmt.Errorf("imap login: %w", err)
	}
	if _, err := client.command("SELECT INBOX"); err != nil {
		return nil, fmt.Errorf("select inbox: %w", err)
	}
	searchResponses, err := client.command("UID SEARCH ALL")
	if err != nil {
		return nil, fmt.Errorf("search inbox: %w", err)
	}
	uids := recentUIDs(parseSearchUIDs(searchResponses), creds.SyncCursor, limit)
	if len(uids) == 0 {
		_, _ = client.command("LOGOUT")
		return nil, nil
	}

	fetchResponses, err := client.command("UID FETCH %s (UID INTERNALDATE BODY.PEEK[])", strings.Join(uids, ","))
	if err != nil {
		return nil, fmt.Errorf("fetch messages: %w", err)
	}
	_, _ = client.command("LOGOUT")
	return parseFetchResponses(fetchResponses, creds), nil
}

type imapResponse struct {
	Line    string
	Literal []byte
}

type imapClient struct {
	reader *bufio.Reader
	writer io.Writer
	tag    int
}

func (c *imapClient) command(format string, args ...any) ([]imapResponse, error) {
	c.tag++
	tag := fmt.Sprintf("A%03d", c.tag)
	command := fmt.Sprintf(format, args...)
	if _, err := fmt.Fprintf(c.writer, "%s %s\r\n", tag, command); err != nil {
		return nil, err
	}
	responses := make([]imapResponse, 0)
	for {
		line, err := c.readLine()
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(line, tag+" ") {
			if strings.Contains(line, " OK") {
				return responses, nil
			}
			return nil, fmt.Errorf("%s", line)
		}
		response := imapResponse{Line: line}
		if literalSize, ok := imapLiteralSize(line); ok {
			literal := make([]byte, literalSize)
			if _, err := io.ReadFull(c.reader, literal); err != nil {
				return nil, err
			}
			response.Literal = literal
		}
		responses = append(responses, response)
	}
}

func (c *imapClient) readLine() (string, error) {
	line, err := c.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func dialIMAP(ctx context.Context, creds moduleuseremail.SyncCredentials) (net.Conn, error) {
	addr := net.JoinHostPort(creds.IMAPHost, strconv.Itoa(creds.IMAPPort))
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	raw, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	if !creds.IMAPUseTLS {
		return raw, nil
	}
	tlsConn := tls.Client(raw, &tls.Config{ServerName: creds.IMAPHost, MinVersion: tls.VersionTLS12})
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		_ = raw.Close()
		return nil, err
	}
	return tlsConn, nil
}

func imapQuote(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	value = strings.ReplaceAll(value, "\r", "")
	value = strings.ReplaceAll(value, "\n", "")
	return `"` + value + `"`
}

func imapLiteralSize(line string) (int, bool) {
	matches := imapLiteralRe.FindStringSubmatch(line)
	if len(matches) != 2 {
		return 0, false
	}
	size, err := strconv.Atoi(matches[1])
	if err != nil || size < 0 || size > maxFetchedBodyBytes*4 {
		return 0, false
	}
	return size, true
}

func parseSearchUIDs(responses []imapResponse) []string {
	for _, response := range responses {
		if strings.HasPrefix(response.Line, "* SEARCH") {
			parts := strings.Fields(strings.TrimPrefix(response.Line, "* SEARCH"))
			uids := make([]string, 0, len(parts))
			for _, part := range parts {
				if _, err := strconv.ParseUint(part, 10, 64); err == nil {
					uids = append(uids, part)
				}
			}
			return uids
		}
	}
	return nil
}

func recentUIDs(uids []string, cursor string, limit int) []string {
	if len(uids) == 0 {
		return nil
	}
	cursorValue, _ := strconv.ParseUint(strings.TrimSpace(cursor), 10, 64)
	filtered := make([]string, 0, len(uids))
	for _, uid := range uids {
		value, err := strconv.ParseUint(uid, 10, 64)
		if err != nil {
			continue
		}
		if cursorValue == 0 || value > cursorValue {
			filtered = append(filtered, uid)
		}
	}
	if len(filtered) > limit {
		filtered = filtered[len(filtered)-limit:]
	}
	return filtered
}

func parseFetchResponses(responses []imapResponse, creds moduleuseremail.SyncCredentials) []FetchedMessage {
	messages := make([]FetchedMessage, 0)
	for _, response := range responses {
		if len(response.Literal) == 0 {
			continue
		}
		uid := firstRegexpGroup(imapUIDRe, response.Line)
		internalDate := parseIMAPInternalDate(firstRegexpGroup(imapInternalDateRe, response.Line))
		messages = append(messages, parseFetchedMessage(response.Literal, uid, internalDate, creds))
	}
	return messages
}

func parseFetchedMessage(raw []byte, uid string, fallbackDate time.Time, creds moduleuseremail.SyncCredentials) FetchedMessage {
	message := FetchedMessage{ProviderMessageID: uid, ReceivedAt: fallbackDate, ToEmail: creds.FromEmail}
	parsed, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		message.Body = string(limitBytes(raw))
		return message
	}
	message.Subject = decodeHeader(parsed.Header.Get("Subject"))
	message.RFCMessageID = moduleemail.NormalizeMessageID(parsed.Header.Get("Message-ID"))
	inReplyTo := moduleemail.ParseMessageIDReferences(parsed.Header.Get("In-Reply-To"))
	if len(inReplyTo) > 0 {
		message.InReplyTo = inReplyTo[0]
		message.ReferenceMessageIDs = append(message.ReferenceMessageIDs, inReplyTo[1:]...)
	}
	message.ReferenceMessageIDs = appendMessageIDReferences(message.ReferenceMessageIDs, moduleemail.ParseMessageIDReferences(parsed.Header.Get("References"))...)
	if from := firstAddress(parsed.Header.Get("From")); from != "" {
		message.FromEmail = from
	}
	if to := firstAddress(parsed.Header.Get("To")); to != "" {
		message.ToEmail = to
	}
	if date, err := parsed.Header.Date(); err == nil {
		message.ReceivedAt = date
	}
	if message.ReceivedAt.IsZero() {
		message.ReceivedAt = time.Now().UTC()
	}
	body, err := decodedMessageBody(parsed.Header, parsed.Body)
	if err == nil {
		message.Body = body
	}
	return message
}

func decodedMessageBody(header mail.Header, body io.Reader) (string, error) {
	contentType := header.Get("Content-Type")
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err == nil && strings.HasPrefix(mediaType, "multipart/") && params["boundary"] != "" {
		return multipartBody(body, params["boundary"]), nil
	}
	return decodedPartBody(header, body)
}

type mimeHeader interface {
	Get(string) string
}

func decodedPartBody(header mimeHeader, body io.Reader) (string, error) {
	var reader io.Reader = body
	switch strings.ToLower(strings.TrimSpace(header.Get("Content-Transfer-Encoding"))) {
	case "base64":
		reader = base64.NewDecoder(base64.StdEncoding, body)
	case "quoted-printable":
		reader = quotedprintable.NewReader(body)
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxFetchedBodyBytes))
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func multipartBody(body io.Reader, boundary string) string {
	reader := multipart.NewReader(body, boundary)
	fallback := ""
	for {
		part, err := reader.NextPart()
		if err != nil {
			break
		}
		mediaType, _, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
		body, _ := decodedPartBody(part.Header, part)
		if strings.EqualFold(mediaType, "text/plain") {
			return body
		}
		if fallback == "" && strings.EqualFold(mediaType, "text/html") {
			fallback = body
		}
	}
	return fallback
}

func firstAddress(value string) string {
	addresses, err := mail.ParseAddressList(value)
	if err != nil || len(addresses) == 0 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(addresses[0].Address))
}

func decodeHeader(value string) string {
	decoded, err := (&mime.WordDecoder{}).DecodeHeader(value)
	if err != nil {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(decoded)
}

func parseIMAPInternalDate(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	parsed, err := time.Parse("_2-Jan-2006 15:04:05 -0700", value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func firstRegexpGroup(expression *regexp.Regexp, value string) string {
	matches := expression.FindStringSubmatch(value)
	if len(matches) < 2 {
		return ""
	}
	return matches[1]
}

func limitBytes(raw []byte) []byte {
	if len(raw) <= maxFetchedBodyBytes {
		return raw
	}
	return raw[:maxFetchedBodyBytes]
}
