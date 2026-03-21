package logging

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
)

const redacted = "[REDACTED]"

func RedactSecret(_ string) string {
	return redacted
}

func RedactURLString(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return redacted
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func RedactHeaderMap(headers http.Header, allowlist ...string) http.Header {
	allowed := make([]string, 0, len(allowlist))
	for _, key := range allowlist {
		allowed = append(allowed, strings.ToLower(http.CanonicalHeaderKey(key)))
	}

	sanitized := make(http.Header)
	for key, values := range headers {
		if !slices.Contains(allowed, strings.ToLower(http.CanonicalHeaderKey(key))) {
			continue
		}
		sanitized[key] = append([]string(nil), values...)
	}
	return sanitized
}
