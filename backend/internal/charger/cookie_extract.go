package charger

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
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
	seenSensitive := make(map[string]struct{}, 2)
	for _, part := range strings.Split(extracted, ";") {
		part = strings.TrimSpace(part)
		eq := strings.IndexByte(part, '=')
		if eq <= 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(part[:eq]))
		if key == "wxopenid" || key == "info" {
			if _, dup := seenSensitive[key]; dup {
				return "", fmt.Errorf("Cookie 包含重复的 %s 字段，请仅粘贴单份有效 Cookie", key)
			}
			seenSensitive[key] = struct{}{}
		}
	}
	return extracted, nil
}

func normalizeCookieValue(raw string) string {
	val := strings.TrimSpace(raw)
	if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
		val = strings.TrimSpace(val[1 : len(val)-1])
	}
	if unescaped, err := url.QueryUnescape(val); err == nil {
		val = strings.TrimSpace(unescaped)
	}
	return val
}

// CookieIdentity returns a canonical upstream identity string for deduplication.
// Uses first-wins semantics (matching RFC 6265 / upstream behavior), strips quotes
// and URL-encoding differences, and ignores mutable deviceid.
func CookieIdentity(cookie string) string {
	trimmed := strings.TrimSpace(cookie)
	if trimmed == "" {
		return ""
	}
	pairs := make(map[string]string)
	for _, part := range strings.Split(trimmed, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		eq := strings.IndexByte(part, '=')
		if eq <= 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(part[:eq]))
		val := normalizeCookieValue(part[eq+1:])
		if key == "" || val == "" {
			continue
		}
		if _, exists := pairs[key]; exists {
			continue
		}
		pairs[key] = val
	}
	if wxopenid := pairs["wxopenid"]; wxopenid != "" {
		return "wxopenid:" + wxopenid
	}
	delete(pairs, "deviceid")
	if len(pairs) == 0 {
		return "raw:" + trimmed
	}
	keys := make([]string, 0, len(pairs))
	for k := range pairs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+pairs[k])
	}
	return "cookie:" + strings.Join(parts, ";")
}
