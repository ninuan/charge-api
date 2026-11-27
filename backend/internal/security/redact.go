package security

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

var sensitiveNames = []string{
	"cookie",
	"info",
	"wxopenid",
	"openid",
	"code",
	"login_buffer",
	"access_token",
	"accesstoken",
	"refresh_token",
	"refreshtoken",
	"token",
	"apptoken",
	"app_token",
	"credentials",
	"sid",
	"verifycode",
	"password",
	"passwd",
	"secret",
	"session",
	"sessionid",
	"session_id",
	"charge_session",
	"authorization",
	"apikey",
	"api_key",
}

// Authorization/Cookie 等头不是 name=value 也不是 JSON 形态，需要单独匹配。
var sensitiveHeaders = []string{
	"authorization",
	"proxy-authorization",
	"cookie",
	"set-cookie",
	"x-api-key",
}

type namedPattern struct {
	name    string
	pattern *regexp.Regexp
}

var (
	compiledJSONPatterns   []namedPattern
	compiledCookiePatterns []namedPattern
	compiledHeaderPatterns []namedPattern
)

func init() {
	compiledJSONPatterns = make([]namedPattern, 0, len(sensitiveNames))
	compiledCookiePatterns = make([]namedPattern, 0, len(sensitiveNames))
	for _, name := range sensitiveNames {
		quoted := regexp.QuoteMeta(name)
		compiledJSONPatterns = append(compiledJSONPatterns, namedPattern{
			name:    name,
			pattern: regexp.MustCompile(`(?i)(["']` + quoted + `["']\s*:\s*["'])([^"']*)(["'])`),
		})
		compiledCookiePatterns = append(compiledCookiePatterns, namedPattern{
			name:    name,
			pattern: regexp.MustCompile(`(?i)(^|[;\s?&])(` + quoted + `)=([^;\s&#'"]+)`),
		})
	}
	compiledHeaderPatterns = make([]namedPattern, 0, len(sensitiveHeaders))
	for _, name := range sensitiveHeaders {
		quoted := regexp.QuoteMeta(name)
		compiledHeaderPatterns = append(compiledHeaderPatterns, namedPattern{
			name:    name,
			pattern: regexp.MustCompile(`(?im)(^|[ \t,;])(` + quoted + `[ \t]*:[ \t]*)([^\r\n]+)`),
		})
	}
}

func RedactValue(name string, value string) string {
	return fmt.Sprintf("<redacted:%s:len=%d>", name, len(value))
}

func RedactText(text string, maxBytes int) string {
	// 必须先脱敏再截断：截断切在敏感值中间会破坏匹配模式，
	// 让残余的半截密钥以原文进日志。
	for i := range compiledJSONPatterns {
		text = redactJSONLike(text, compiledJSONPatterns[i])
		text = redactCookieLike(text, compiledCookiePatterns[i])
	}
	for i := range compiledHeaderPatterns {
		text = redactHeaderLike(text, compiledHeaderPatterns[i])
	}
	if maxBytes > 0 && len(text) > maxBytes {
		cut := maxBytes
		for cut > 0 && !utf8.RuneStart(text[cut]) {
			cut--
		}
		text = text[:cut] + "...<truncated>"
	}
	return text
}

func redactJSONLike(text string, entry namedPattern) string {
	return entry.pattern.ReplaceAllStringFunc(text, func(match string) string {
		parts := entry.pattern.FindStringSubmatch(match)
		if len(parts) != 4 {
			return match
		}
		return parts[1] + RedactValue(entry.name, parts[2]) + parts[3]
	})
}

func redactCookieLike(text string, entry namedPattern) string {
	return entry.pattern.ReplaceAllStringFunc(text, func(match string) string {
		parts := entry.pattern.FindStringSubmatch(match)
		if len(parts) != 4 {
			return match
		}
		return parts[1] + parts[2] + "=" + RedactValue(strings.ToLower(parts[2]), parts[3])
	})
}

func redactHeaderLike(text string, entry namedPattern) string {
	return entry.pattern.ReplaceAllStringFunc(text, func(match string) string {
		parts := entry.pattern.FindStringSubmatch(match)
		if len(parts) != 4 {
			return match
		}
		return parts[1] + parts[2] + RedactValue(entry.name, parts[3])
	})
}
