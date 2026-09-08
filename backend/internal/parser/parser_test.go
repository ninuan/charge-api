package parser

import (
	"strings"
	"testing"

	"charge-dashboard/internal/model"
)

func TestParsePayloadTreatsNotOnlineAsOffline(t *testing.T) {
	body := []byte(`{"id":"device-1","status":"当前不在线","opennum":2,"used":[1]}`)

	pile, err := ParsePayload("test", body)
	if err != nil {
		t.Fatalf("ParsePayload: %v", err)
	}
	if pile.Online {
		t.Fatal("expected pile to be offline")
	}
	if len(pile.UsedPortIDs) != 0 {
		t.Fatalf("offline pile should not expose used ports: %v", pile.UsedPortIDs)
	}
	for _, port := range pile.Ports {
		if port.Status != model.PortOffline {
			t.Fatalf("expected port %d to be offline, got %s", port.ID, port.Status)
		}
	}
}

func TestParsePayloadErrorDoesNotIncludeRemoteBody(t *testing.T) {
	_, err := ParsePayload("remote", []byte(`{"password":"remote-secret","message":"bad"}`))
	if err == nil {
		t.Fatal("ParsePayload expected an error")
	}
	if strings.Contains(err.Error(), "remote-secret") || strings.Contains(err.Error(), "password") {
		t.Fatalf("ParsePayload error exposed remote response: %v", err)
	}
}

func TestValidateCaptureURLRequiresHTTPSOrLoopbackHTTP(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{name: "https", url: "https://example.com/api", want: true},
		{name: "loopback ipv4", url: "http://127.0.0.1:8080/api", want: true},
		{name: "loopback ipv6", url: "http://[::1]:8080/api", want: true},
		{name: "localhost", url: "http://localhost:8080/api", want: true},
		{name: "public http", url: "http://example.com/api", want: false},
		{name: "ftp", url: "ftp://example.com/api", want: false},
		{name: "credentials", url: "https://user:pass@example.com/api", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCaptureURL(tt.url)
			if (err == nil) != tt.want {
				t.Fatalf("validateCaptureURL(%q) error = %v, want valid=%v", tt.url, err, tt.want)
			}
		})
	}
}

func TestParsePayloadDoesNotInventUsageMetrics(t *testing.T) {
	body := []byte(`{"id":"device-1","status":"在线","opennum":1,"used":[1]}`)

	pile, err := ParsePayload("test", body)
	if err != nil {
		t.Fatalf("ParsePayload: %v", err)
	}
	port := pile.Ports[0]
	if port.Status != model.PortInUse {
		t.Fatalf("expected in-use port, got %s", port.Status)
	}
	if port.PowerKW != 0 || port.EnergyKWh != 0 {
		t.Fatalf("unexpected invented power metrics: power=%v energy=%v", port.PowerKW, port.EnergyKWh)
	}
	if port.UsedSeconds != 0 || port.SessionMin != 0 || port.UsedText != "" || port.StartedAt != nil {
		t.Fatalf("unexpected invented usage duration: %+v", port)
	}
}

func TestParsePayloadCalculatesRemainingDuration(t *testing.T) {
	body := []byte(`{"id":"device-1","status":"设备在线","opennum":1,"useds":[{"i":1,"u":5400,"s":"8小时"}]}`)

	pile, err := ParsePayload("test", body)
	if err != nil {
		t.Fatalf("ParsePayload: %v", err)
	}
	port := pile.Ports[0]
	if !pile.Online || port.UsedSeconds != 5400 || port.SessionMin != 90 {
		t.Fatalf("reported duration was not preserved: %+v", port)
	}
	if port.UsedText != "1小时30分钟" || port.RemainingText != "6小时30分钟" || port.StartedAt == nil {
		t.Fatalf("reported usage details were not preserved: %+v", port)
	}
}

func TestParsePayloadPreservesFullChargeStop(t *testing.T) {
	body := []byte(`{"id":"device-1","status":"设备在线","opennum":1,"useds":[{"i":1,"u":5400,"s":"充满自停"}]}`)

	pile, err := ParsePayload("test", body)
	if err != nil {
		t.Fatalf("ParsePayload: %v", err)
	}
	if got := pile.Ports[0].RemainingText; got != "充满自停" {
		t.Fatalf("remaining text = %q, want 充满自停", got)
	}
}

func TestRemainingDurationTextSupportsMinutePlansAndExpiredSessions(t *testing.T) {
	tests := []struct {
		name       string
		configured string
		used       int
		want       string
	}{
		{name: "minute plan", configured: "30 分钟", used: 61, want: "28分钟"},
		{name: "expired", configured: "1小时", used: 3601, want: "0分钟"},
		{name: "unknown value", configured: "未知模式", used: 60, want: "未知模式"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := remainingDurationText(tt.configured, tt.used); got != tt.want {
				t.Fatalf("remainingDurationText(%q, %d) = %q, want %q", tt.configured, tt.used, got, tt.want)
			}
		})
	}
}
