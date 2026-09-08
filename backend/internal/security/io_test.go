package security

import (
	"strings"
	"testing"
)

func TestReadLimitedRejectsOversizedBody(t *testing.T) {
	if _, err := ReadLimited(strings.NewReader("12345"), 4); err == nil {
		t.Fatal("ReadLimited accepted an oversized response")
	}
	body, err := ReadLimited(strings.NewReader("1234"), 4)
	if err != nil || string(body) != "1234" {
		t.Fatalf("ReadLimited exact response = %q, %v", body, err)
	}
}

func TestSanitizeLogTextRedactsAndFlattensControlCharacters(t *testing.T) {
	got := SanitizeLogText("Cookie: sid=secret\nnext=\u0001value", 0)
	if strings.Contains(got, "secret") || strings.ContainsAny(got, "\r\n\x01") {
		t.Fatalf("SanitizeLogText leaked sensitive/control text: %q", got)
	}
}

func TestSanitizeLogTextHidesLocations(t *testing.T) {
	input := "open /srv/private/charge.db: denied\nCookie: sid=hidden\nGet https://user:pass@private.example/api?token=hidden: failed\x00"
	got := SanitizeLogText(input, 1024)
	for _, forbidden := range []string{"/srv", "charge.db", "hidden", "user:pass", "private.example", "\n", "\x00"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("log contains %q: %s", forbidden, got)
		}
	}
	if !strings.Contains(got, "denied") || !strings.Contains(got, "failed") {
		t.Fatalf("missing diagnostic: %s", got)
	}
}

func TestSanitizeLogTextTruncatesAtRuneBoundary(t *testing.T) {
	got := SanitizeLogText("故障详情", 4)
	if got != "故...<truncated>" {
		t.Fatalf("invalid truncation: %q", got)
	}
}
