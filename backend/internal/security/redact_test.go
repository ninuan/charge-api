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
