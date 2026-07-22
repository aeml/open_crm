package app

import (
	"net/http"
	"net/url"
	"strings"
)

var transparentTrackingPixel = []byte{
	'G', 'I', 'F', '8', '9', 'a', 1, 0, 1, 0, 0x80, 0, 0, 0, 0, 0, 0xff, 0xff, 0xff, ',',
	0, 0, 0, 0, 1, 0, 1, 0, 0, 2, 2, 0x44, 1, 0, ';',
}

func handleTrackEmailOpen(messages emailMessagesService, w http.ResponseWriter, r *http.Request) {
	trackingToken := strings.TrimSpace(r.PathValue("trackingToken"))
	if messages != nil && trackingToken != "" {
		_ = messages.MarkOpenedByToken(r.Context(), trackingToken)
	}
	setEmailTrackingResponseHeaders(w)
	w.Header().Set("Content-Type", "image/gif")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(transparentTrackingPixel)
}

func handleTrackEmailClick(messages emailMessagesService, w http.ResponseWriter, r *http.Request) {
	setEmailTrackingResponseHeaders(w)
	clickToken := strings.TrimSpace(r.PathValue("clickToken"))
	targetURL := ""
	if messages != nil && clickToken != "" {
		resolvedURL, err := messages.MarkClickedByToken(r.Context(), clickToken)
		if err == nil {
			targetURL = resolvedURL
		}
	}
	if !isSafeEmailClickURL(targetURL) {
		http.NotFound(w, r)
		return
	}
	// #nosec G710 -- click tracking intentionally redirects to the stored recipient URL after an explicit absolute HTTP(S)-only check.
	http.Redirect(w, r, targetURL, http.StatusFound)
}

func setEmailTrackingResponseHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
}

func isSafeEmailClickURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}
