package app

import (
	"crypto/rand"
	"encoding/base64"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	moduleemailmessages "github.com/aeml/open_crm/apps/api/internal/modules/emailmessages"
)

var emailBodyURLPattern = regexp.MustCompile(`https?://[^\s<>"']+`)

func newEmailTrackingToken() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}

func emailTrackingBaseURL(r *http.Request) string {
	if r == nil {
		return ""
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	host := strings.TrimSpace(r.Host)
	if requestFromTrustedProxy(r) {
		if forwardedProto, _, _ := strings.Cut(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), ","); forwardedProto == "http" || forwardedProto == "https" {
			scheme = forwardedProto
		}
		if forwardedHost, _, _ := strings.Cut(strings.TrimSpace(r.Header.Get("X-Forwarded-Host")), ","); strings.TrimSpace(forwardedHost) != "" {
			host = strings.TrimSpace(forwardedHost)
		}
	}
	candidate := scheme + "://" + host
	parsed, err := url.Parse(candidate)
	if err != nil || parsed.Scheme != scheme || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" {
		return ""
	}
	return candidate
}

func emailTrackingURL(baseURL, token string) string {
	return emailTrackingRouteURL(baseURL, "/api/email-messages/open/", token)
}

func emailClickTrackingURL(baseURL, token string) string {
	return emailTrackingRouteURL(baseURL, "/api/email-messages/click/", token)
}

func emailTrackingRouteURL(baseURL, routePrefix, token string) string {
	token = strings.TrimSpace(token)
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if token == "" || baseURL == "" {
		return ""
	}
	return baseURL + routePrefix + url.PathEscape(token)
}

func trackedHTMLBody(textBody, trackingURL, trackingBaseURL, unsubscribeURL string) (string, []moduleemailmessages.TrackedLinkInput) {
	var body strings.Builder
	links := make([]moduleemailmessages.TrackedLinkInput, 0)
	last := 0
	for _, loc := range emailBodyURLPattern.FindAllStringIndex(textBody, -1) {
		if loc[0] < last {
			continue
		}
		candidate := textBody[loc[0]:loc[1]]
		targetURL, trailing := splitEmailBodyURL(candidate)
		if targetURL == "" || !isSafeEmailClickURL(targetURL) {
			continue
		}
		appendEscapedEmailHTML(&body, textBody[last:loc[0]])
		href := targetURL
		if trackingBaseURL != "" && len(links) < 100 {
			clickToken := newEmailTrackingToken()
			clickURL := emailClickTrackingURL(trackingBaseURL, clickToken)
			if clickURL != "" {
				href = clickURL
				links = append(links, moduleemailmessages.TrackedLinkInput{ClickToken: clickToken, TargetURL: targetURL})
			}
		}
		body.WriteString(`<a href="` + html.EscapeString(href) + `">` + html.EscapeString(targetURL) + `</a>`)
		appendEscapedEmailHTML(&body, trailing)
		last = loc[1]
	}
	appendEscapedEmailHTML(&body, textBody[last:])
	if unsubscribeURL != "" {
		body.WriteString(`<p style="margin-top:24px;font-size:12px;color:#666">To stop receiving emails from us, <a href="` + html.EscapeString(unsubscribeURL) + `">unsubscribe here</a>.</p>`)
	}
	trackingPixel := ""
	if trackingURL != "" {
		trackingPixel = `<img src="` + html.EscapeString(trackingURL) + `" width="1" height="1" alt="" style="display:none" />`
	}
	return `<!doctype html><html><body><div>` + body.String() + `</div>` + trackingPixel + `</body></html>`, links
}

func emailUnsubscribeURL(r *http.Request, suppressions emailSuppressionsService, organizationID int64, email string) string {
	if suppressions == nil {
		return ""
	}
	token, err := suppressions.UnsubscribeToken(organizationID, email)
	if err != nil {
		return ""
	}
	return emailTrackingRouteURL(emailTrackingBaseURL(r), "/api/email-unsubscribe/", token)
}

func textBodyWithUnsubscribe(body, unsubscribeURL string) string {
	if unsubscribeURL == "" {
		return body
	}
	return strings.TrimRight(body, " \t\r\n") + "\n\nTo stop receiving emails from us, unsubscribe here: " + unsubscribeURL
}

func appendEscapedEmailHTML(builder *strings.Builder, value string) {
	escaped := html.EscapeString(value)
	escaped = strings.ReplaceAll(escaped, "\r\n", "\n")
	escaped = strings.ReplaceAll(escaped, "\r", "\n")
	escaped = strings.ReplaceAll(escaped, "\n", "<br>\n")
	builder.WriteString(escaped)
}

func splitEmailBodyURL(value string) (string, string) {
	targetURL := strings.TrimRight(value, ".,;:!?)]}")
	if targetURL == "" {
		return "", value
	}
	return targetURL, value[len(targetURL):]
}
