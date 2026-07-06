package security

import (
	"fmt"
	"regexp"
	"strings"
)

var sensitiveNames = []string{
	"cookie",
	"info",
	"wxopenid",
	"code",
	"login_buffer",
	"access_token",
	"accesstoken",
	"refresh_token",
	"refreshtoken",
	"credentials",
	"sid",
	"verifycode",
}

func RedactValue(name string, value string) string {
	return fmt.Sprintf("<redacted:%s:len=%d>", name, len(value))
}

func RedactText(text string, maxBytes int) string {
	if maxBytes > 0 && len(text) > maxBytes {
		text = text[:maxBytes] + "...<truncated>"
	}
	for _, name := range sensitiveNames {
		text = redactJSONLike(text, name)
		text = redactCookieLike(text, name)
	}
	return text
}

func redactJSONLike(text, name string) string {
	pattern := regexp.MustCompile(`(?i)(["']` + regexp.QuoteMeta(name) + `["']\s*:\s*["'])([^"']*)(["'])`)
	return pattern.ReplaceAllStringFunc(text, func(match string) string {
		parts := pattern.FindStringSubmatch(match)
		if len(parts) != 4 {
			return match
		}
		return parts[1] + RedactValue(name, parts[2]) + parts[3]
	})
}

func redactCookieLike(text, name string) string {
	pattern := regexp.MustCompile(`(?i)(^|[;\s])(` + regexp.QuoteMeta(name) + `)=([^;\s]+)`)
	return pattern.ReplaceAllStringFunc(text, func(match string) string {
		parts := pattern.FindStringSubmatch(match)
		if len(parts) != 4 {
			return match
		}
		return parts[1] + parts[2] + "=" + RedactValue(strings.ToLower(parts[2]), parts[3])
	})
}
