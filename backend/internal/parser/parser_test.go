package parser

import (
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

func TestParsePayloadUsesReportedDuration(t *testing.T) {
	body := []byte(`{"id":"device-1","status":"设备在线","opennum":1,"useds":[{"i":1,"u":61,"s":"29分钟"}]}`)

	pile, err := ParsePayload("test", body)
	if err != nil {
		t.Fatalf("ParsePayload: %v", err)
	}
	port := pile.Ports[0]
	if !pile.Online || port.UsedSeconds != 61 || port.SessionMin != 2 {
		t.Fatalf("reported duration was not preserved: %+v", port)
	}
	if port.UsedText != "1分钟" || port.RemainingText != "29分钟" || port.StartedAt == nil {
		t.Fatalf("reported usage details were not preserved: %+v", port)
	}
}
