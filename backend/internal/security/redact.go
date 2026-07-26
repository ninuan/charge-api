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
	"openid",
	"code",
	"login_buffer",
	"access_token",
	"accesstoken",
	"refresh_token",
	"refreshtoken",
	"token",
	"credentials",
	"sid",
	"verifycode",
	"password",
	"passwd",
	"secret",
	"session",
	"sessionid",
	"session_id",
	"authorization",
	"apikey",
	"api_key",
}

// Authorization/Cookie 等头不是 name=value 也不是 JSON 形态，需要按行单独匹配。
var sensitiveHeaders = []string{
	"authorization",
	"proxy-authorization",
	"cookie",
	"set-cookie",
	"x-api-key",
}

func RedactValue(name string, value string) string {
	return fmt.Sprintf("<redacted:%s:len=%d>", name, len(value))
}

func RedactText(text string, maxBytes int) string {
	// 必须先脱敏再截断：截断切在敏感值中间会破坏匹配模式，
	// 让残余的半截密钥以原文进日志。
	for _, name := range sensitiveNames {
		text = redactJSONLike(text, name)
		text = redactCookieLike(text, name)
	}
	for _, name := range sensitiveHeaders {
		text = redactHeaderLike(text, name)
	}
	if maxBytes > 0 && len(text) > maxBytes {
		text = text[:maxBytes] + "...<truncated>"
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

func redactHeaderLike(text, name string) string {
	pattern := regexp.MustCompile(`(?im)^([ \t]*` + regexp.QuoteMeta(name) + `[ \t]*:[ \t]*)(.+)$`)
	return pattern.ReplaceAllStringFunc(text, func(match string) string {
		parts := pattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		return parts[1] + RedactValue(name, parts[2])
	})
}
