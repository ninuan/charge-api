package persistence

import (
	"bytes"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"charge-dashboard/internal/model"
)

func TestSQLiteUpgradesLegacySessionsWithClientMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE sessions (
			token_hash BLOB PRIMARY KEY,
			user_id TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			expires_at INTEGER NOT NULL
		)
	`); err != nil {
		t.Fatalf("create legacy sessions: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}

	store, err := OpenSQLite(path, bytes.Repeat([]byte{0x77}, CookieKeySize))
	if err != nil {
		t.Fatalf("upgrade database: %v", err)
	}
	defer store.Close()
	now := time.Now().UTC()
	record := SessionRecord{
		TokenHash: []byte("session-hash"), UserID: "user-1",
		CreatedAt: now, ExpiresAt: now.Add(time.Hour), LastActiveAt: now,
		Browser: "Safari", OS: "iOS", DeviceType: "手机",
		IPLabel: "192.168.*.*",
	}
	if err := store.SaveSession(record, 5); err != nil {
		t.Fatalf("save upgraded session: %v", err)
	}
	sessions, err := store.ListUserSessions("user-1")
	if err != nil {
		t.Fatalf("list upgraded sessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].Browser != "Safari" ||
		sessions[0].DeviceType != "手机" ||
		!sessions[0].LastActiveAt.Equal(now) {
		t.Fatalf("unexpected upgraded session: %+v", sessions)
	}
}

func TestIncidentsMergeAndPreserveAdminWorkflow(t *testing.T) {
	store, err := OpenSQLite(
		filepath.Join(t.TempDir(), "state.db"),
		bytes.Repeat([]byte{0x78}, CookieKeySize),
	)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer store.Close()
	first := time.Now().Add(-time.Minute)
	issue := model.SystemException{
		ID: "offline-user-1-device-1", UserID: "user-1", Username: "alice",
		DeviceID: "device-1", Type: "offline", Level: "warning",
		Message: "设备离线", Time: first,
	}
	if err := store.UpsertIncident(issue); err != nil {
		t.Fatalf("first UpsertIncident: %v", err)
	}
	issue.Time = first.Add(time.Minute)
	if err := store.UpsertIncident(issue); err != nil {
		t.Fatalf("second UpsertIncident: %v", err)
	}
	updated, err := store.UpdateIncident(
		issue.ID, "acknowledged", "已联系用户", "admin", time.Now(),
	)
	if err != nil {
		t.Fatalf("UpdateIncident: %v", err)
	}
	if updated.Occurrences != 2 || updated.Status != "acknowledged" ||
		updated.Note != "已联系用户" || updated.HandledBy != "admin" {
		t.Fatalf("unexpected incident workflow: %+v", updated)
	}
}

func TestAuditAndOperationsStatusAreQueryable(t *testing.T) {
	store, err := OpenSQLite(
		filepath.Join(t.TempDir(), "state.db"),
		bytes.Repeat([]byte{0x79}, CookieKeySize),
	)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer store.Close()
	if err := store.RecordAudit(model.AuditEntry{
		ActorID: "admin-1", Actor: "admin", Action: "user.delete",
		TargetType: "user", TargetID: "user-1", TargetLabel: "alice",
		Result: "success", CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("RecordAudit: %v", err)
	}
	page, err := store.ListAudit(1, 20)
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if page.Total != 1 || page.Items[0].TargetLabel != "alice" {
		t.Fatalf("unexpected audit page: %+v", page)
	}
	status, err := store.OperationsStatus(90)
	if err != nil {
		t.Fatalf("OperationsStatus: %v", err)
	}
	if status.DatabaseSizeBytes <= 0 || status.IntegrityResult != "ok" ||
		status.MetricRetentionDays != 90 {
		t.Fatalf("unexpected operations status: %+v", status)
	}
}
