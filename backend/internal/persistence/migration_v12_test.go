package persistence

import (
	"bytes"
	"testing"
)

func TestSQLiteV12DisablesRecurringRulesWithoutDeletingConfiguration(t *testing.T) {
	path := t.TempDir() + "/state.db"
	key := bytes.Repeat([]byte{0x72}, CookieKeySize)
	store, err := OpenSQLite(path, key)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	statements := []string{
		`INSERT INTO users(id,username,password_hash,role,enabled,created_at,updated_at)
		 VALUES('user-1','alice','hash','user',1,'2026-08-26T00:00:00Z','2026-08-26T00:00:00Z')`,
		`INSERT INTO watch_rules(
			id,user_id,device_id,mode,enabled,active_weekdays,active_start_minute,
			active_end_minute,timezone,created_at,updated_at,stop_after_notify
		) VALUES('recurring-1','user-1','61034278','recurring',1,31,480,1320,'Asia/Shanghai',100,200,0)`,
		`INSERT INTO watch_rules(
			id,user_id,device_id,mode,enabled,active_weekdays,active_start_minute,
			active_end_minute,timezone,created_at,updated_at,expires_at,stop_after_notify
		) VALUES('temporary-1','user-1','61034279','temporary',1,127,0,0,'Asia/Shanghai',100,200,500,1)`,
		`INSERT INTO metadata(key,value) VALUES(
			'registration_settings','{"backgroundRemindersEnabled":true,"recurringRemindersEnabled":true}'
		) ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		`DELETE FROM metadata WHERE key='schema_v12_recurring_retired'`,
		`UPDATE metadata SET value='11' WHERE key='schema_version'`,
	}
	for _, statement := range statements {
		if _, err := store.db.Exec(statement); err != nil {
			t.Fatalf("prepare schema v11 fixture with %q: %v", statement, err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close v11 fixture: %v", err)
	}

	migrated, err := OpenSQLite(path, key)
	if err != nil {
		t.Fatalf("migrate schema v11 fixture: %v", err)
	}
	defer migrated.Close()

	var enabled, weekdays, start, end int
	if err := migrated.db.QueryRow(`
		SELECT enabled, active_weekdays, active_start_minute, active_end_minute
		FROM watch_rules WHERE id='recurring-1'
	`).Scan(&enabled, &weekdays, &start, &end); err != nil {
		t.Fatalf("read retired recurring rule: %v", err)
	}
	if enabled != 0 || weekdays != 31 || start != 480 || end != 1320 {
		t.Fatalf("recurring rule data changed: enabled=%d weekdays=%d start=%d end=%d", enabled, weekdays, start, end)
	}
	if err := migrated.db.QueryRow(`SELECT enabled FROM watch_rules WHERE id='temporary-1'`).Scan(&enabled); err != nil {
		t.Fatalf("read temporary rule: %v", err)
	}
	if enabled != 1 {
		t.Fatalf("temporary rule enabled = %d, want 1", enabled)
	}
	var rawSettings string
	if err := migrated.db.QueryRow(`SELECT value FROM metadata WHERE key='registration_settings'`).Scan(&rawSettings); err != nil {
		t.Fatalf("read migrated settings: %v", err)
	}
	if rawSettings != `{"backgroundRemindersEnabled":true,"recurringRemindersEnabled":false}` {
		t.Fatalf("migrated settings = %s", rawSettings)
	}
	var markerCount int
	if err := migrated.db.QueryRow(`SELECT COUNT(*) FROM metadata WHERE key='schema_v12_recurring_retired'`).Scan(&markerCount); err != nil {
		t.Fatalf("read schema v12 marker: %v", err)
	}
	if markerCount != 1 {
		t.Fatalf("schema v12 marker count = %d, want 1", markerCount)
	}
}
