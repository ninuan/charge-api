package charger

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	curlHeaderCookieSingle = regexp.MustCompile(`(?i)(?:-H|--header)\s+'cookie:\s*([^']+)'`)
	curlHeaderCookieDouble = regexp.MustCompile(`(?i)(?:-H|--header)\s+"cookie:\s*([^"]+)"`)
	curlFlagCookieSingle   = regexp.MustCompile(`(?i)(?:-b|--cookie)\s+'([^']+)'`)
	curlFlagCookieDouble   = regexp.MustCompile(`(?i)(?:-b|--cookie)\s+"([^"]+)"`)
	rawHeaderCookieLine    = regexp.MustCompile(`(?im)^\s*cookie\s*:\s*(.+?)\s*$`)
)

// ExtractCookie accepts a raw Cookie string, full HTTP request headers, or a
// Copy-as-cURL snippet and returns a normalized Cookie header value.
func ExtractCookie(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("cookie 不能为空")
	}

	extracted := ""
	// 1. Copy-as-cURL (-H 'Cookie: ...' / -b '...')
	for _, re := range []*regexp.Regexp{
		curlHeaderCookieSingle,
		curlHeaderCookieDouble,
		curlFlagCookieSingle,
		curlFlagCookieDouble,
	} {
		if m := re.FindStringSubmatch(trimmed); len(m) == 2 {
			extracted = strings.TrimSpace(m[1])
			break
		}
	}

	// 2. Multiline HTTP headers containing "Cookie: ..."
	if extracted == "" {
		if m := rawHeaderCookieLine.FindStringSubmatch(trimmed); len(m) == 2 {
			extracted = strings.TrimSpace(m[1])
		}
	}

	// 3. Fallback: plain cookie string
	if extracted == "" {
		if strings.ContainsAny(trimmed, "\r\n") {
			return "", fmt.Errorf("未在粘贴内容中找到 Cookie 字段，请确认复制了包含 Cookie 的 cURL 或请求头")
		}
		extracted = trimmed
	}

	extracted = strings.Trim(extracted, `"'`)
	extracted = strings.TrimSpace(strings.TrimPrefix(extracted, "Cookie:"))
	extracted = strings.TrimSpace(strings.TrimPrefix(extracted, "cookie:"))
	extracted = strings.ReplaceAll(extracted, "\r", "")
	extracted = strings.ReplaceAll(extracted, "\n", "")
	if extracted == "" || !strings.Contains(extracted, "=") {
		return "", fmt.Errorf("Cookie 格式无效，需要包含有效的键值对（如 wxopenid=...; info=...）")
	}
	return extracted, nil
}
