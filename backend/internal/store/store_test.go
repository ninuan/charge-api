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

func TestReorderPilesUpdatesEveryPileAtomically(t *testing.T) {
	dashboard := NewDashboardStore([]model.Pile{
		{ID: "device-1", SortOrder: 0},
		{ID: "device-2", SortOrder: 1},
		{ID: "device-3", SortOrder: 2},
	})

	snapshot, err := dashboard.ReorderPiles([]string{
		"device-3",
		"device-1",
		"device-2",
	})
	if err != nil {
		t.Fatalf("ReorderPiles: %v", err)
	}
	for index, id := range []string{"device-3", "device-1", "device-2"} {
		if snapshot.Piles[index].ID != id || snapshot.Piles[index].SortOrder != index {
			t.Fatalf("unexpected reordered piles: %+v", snapshot.Piles)
		}
	}
}

func TestReorderPilesRejectsIncompleteAndDuplicateIDsWithoutMutation(t *testing.T) {
	dashboard := NewDashboardStore([]model.Pile{
		{ID: "device-1", SortOrder: 0},
		{ID: "device-2", SortOrder: 1},
	})

	for _, ids := range [][]string{
		{"device-2"},
		{"device-1", "device-1"},
		{"device-1", "unknown"},
	} {
		if _, err := dashboard.ReorderPiles(ids); err == nil {
			t.Fatalf("ReorderPiles(%v) unexpectedly succeeded", ids)
		}
	}
	snapshot := dashboard.Snapshot()
	if snapshot.Piles[0].ID != "device-1" || snapshot.Piles[1].ID != "device-2" {
		t.Fatalf("invalid reorder mutated piles: %+v", snapshot.Piles)
	}
}
