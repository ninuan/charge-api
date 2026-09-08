package security

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxUpstreamBodyBytes caps responses from integrations and remote devices.
// These responses are parsed as JSON or small HTML fragments; accepting an
// unbounded body lets an unhealthy upstream exhaust the server's memory.
const MaxUpstreamBodyBytes int64 = 1 << 20

// ReadLimited reads at most maxBytes and rejects a response that exceeds the
// limit. Reading one extra byte distinguishes an exact-size response from an
// oversized one without retaining the complete body.
func ReadLimited(reader io.Reader, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = MaxUpstreamBodyBytes
	}
	body, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("response exceeds %d-byte limit", maxBytes)
	}
	return body, nil
}

// Paths and remote URLs may contain credentials or identify the deployment.
var logLocationPattern = regexp.MustCompile(`(?i)https?://[^\s<>"']+|(?:[a-z]:[\\/]|/)[^\s<>"']+`)

// SanitizeLogText removes secrets and control characters before text reaches
// journald or a log collector. In particular, newlines must not be allowed to
// forge additional log records.
func SanitizeLogText(text string, maxBytes int) string {
	text = RedactText(text, 0)
	text = logLocationPattern.ReplaceAllString(text, "<redacted:location>")
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, text)
	if maxBytes > 0 && len(text) > maxBytes {
		for maxBytes > 0 && !utf8.RuneStart(text[maxBytes]) {
			maxBytes--
		}
		text = text[:maxBytes] + "...<truncated>"
	}
	return text
}
