package security

import (
	"strings"
	"testing"
)

func TestRedactValueHidesSensitiveValueAndKeepsLength(t *testing.T) {
	got := RedactValue("access_token", "secret-refresh")
	if got != "<redacted:access_token:len=14>" {
		t.Fatalf("RedactValue() = %q", got)
	}
	if strings.Contains(got, "secret-refresh") {
		t.Fatalf("redacted value leaked original secret: %q", got)
	}
}

func TestRedactTextCoversCredentialShapes(t *testing.T) {
	input := `{"password":"hunter2-secret","openid":"oX9-abcdef"}` + "\n" +
		"Authorization: Bearer very-secret-token\n" +
		"Set-Cookie: sid=abcdef; Path=/\n" +
		"session=deadbeef; other=1"
	got := RedactText(input, 0)

	for _, leaked := range []string{"hunter2-secret", "oX9-abcdef", "very-secret-token", "deadbeef"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("RedactText leaked %q in %q", leaked, got)
		}
	}
}

func TestRedactTextRedactsBeforeTruncating(t *testing.T) {
	// 敏感值恰好横跨截断点：先截断会留下半截密钥原文，
	// 先脱敏则整个值都被替换后才截断。
	secret := strings.Repeat("s", 64)
	input := `{"prefix":"x","password":"` + secret + `"}`
	cut := strings.Index(input, secret) + 16

	got := RedactText(input, cut)
	if strings.Contains(got, secret[:8]) {
		t.Fatalf("RedactText leaked a truncated secret fragment: %q", got)
	}
}
