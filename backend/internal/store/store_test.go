package store

import (
	"testing"
	"time"

	"charge-dashboard/internal/model"
)

func TestMergeCapturePilesPreservesDevicesWithoutNewData(t *testing.T) {
	now := time.Now()
	dashboard := NewDashboardStore([]model.Pile{
		{ID: "device-1", Name: "old-success", CreatedAt: now, Source: "remote"},
		{ID: "device-2", Name: "cached-failure", CreatedAt: now, Source: "remote"},
	})

	dashboard.MergeCapturePiles([]model.Pile{
		{ID: "device-1", Name: "new-success", CreatedAt: now.Add(time.Hour), Source: "remote"},
	})

	snapshot := dashboard.Snapshot()
	if len(snapshot.Piles) != 2 {
		t.Fatalf("expected failed device cache to remain, got %+v", snapshot.Piles)
	}
	if snapshot.Piles[0].Name != "new-success" || snapshot.Piles[1].Name != "cached-failure" {
		t.Fatalf("unexpected merged devices: %+v", snapshot.Piles)
	}
	if !snapshot.Piles[0].CreatedAt.Equal(now) {
		t.Fatal("updated device should retain its original creation time")
	}
}
