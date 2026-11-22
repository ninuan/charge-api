package charger

import (
	"fmt"
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
	return extracted, nil
}

// CookieIdentity returns a canonical upstream identity string for deduplication.
// When wxopenid is present it is used directly so reordering fields, adding noise
// keys, or refreshing info for the same WeChat account still maps to one identity.
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
		val := strings.TrimSpace(part[eq+1:])
		if key == "" || val == "" {
			continue
		}
		pairs[key] = val
	}
	if wxopenid := pairs["wxopenid"]; wxopenid != "" {
		return "wxopenid:" + wxopenid
	}
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
