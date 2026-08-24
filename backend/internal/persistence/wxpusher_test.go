package persistence

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"charge-dashboard/internal/model"
)

func TestWxPusherPersistenceEncryptsIdentifiersAndEnforcesOwnership(t *testing.T) {
	store, users, now := newWxPusherTestStore(t)
	defer store.Close()
	binding := model.WxPusherBinding{
		UserID: users[0].ID, UID: "UID_sensitive_alice", Enabled: true,
		EventTypes: model.WxPusherDefaultEventTypes,
		BoundAt:    now, UpdatedAt: now,
	}
	if err := store.SaveWxPusherBinding(binding); err != nil {
		t.Fatalf("SaveWxPusherBinding: %v", err)
	}
	loaded, ok, err := store.LoadWxPusherBinding(users[0].ID)
	if err != nil || !ok || loaded.UID != binding.UID || loaded.EventTypes != model.WxPusherDefaultEventTypes {
		t.Fatalf("LoadWxPusherBinding = %+v, ok %v, err %v", loaded, ok, err)
	}
	var ciphertext []byte
	if err := store.db.QueryRow(`SELECT uid_ciphertext FROM wxpusher_bindings WHERE user_id=?`, users[0].ID).Scan(&ciphertext); err != nil {
		t.Fatalf("read encrypted uid: %v", err)
	}
	if bytes.Contains(ciphertext, []byte(binding.UID)) {
		t.Fatal("wxpusher uid was stored in plaintext")
	}
	duplicateUID := binding
	duplicateUID.UserID = users[1].ID
	if err := store.SaveWxPusherBinding(duplicateUID); err == nil {
		t.Fatal("one wxpusher uid was bound to multiple users")
	}

	session := model.WxPusherBindSession{
		ID: "bind-1", UserID: users[0].ID, ProviderCode: "provider-secret-code",
		QRURL:     "https://wxpusher.zjiecode.com/api/qrcode/test",
		CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute), NextPollAt: now.Add(10 * time.Second),
	}
	if err := store.SaveWxPusherBindSession(session); err != nil {
		t.Fatalf("SaveWxPusherBindSession: %v", err)
	}
	loadedSession, ok, err := store.LoadWxPusherBindSession(users[0].ID, session.ID)
	if err != nil || !ok || loadedSession.ProviderCode != session.ProviderCode {
		t.Fatalf("LoadWxPusherBindSession = %+v, ok %v, err %v", loadedSession, ok, err)
	}
	var providerCiphertext []byte
	if err := store.db.QueryRow(`SELECT provider_code_ciphertext FROM wxpusher_bind_sessions WHERE id=?`, session.ID).Scan(&providerCiphertext); err != nil {
		t.Fatalf("read encrypted provider code: %v", err)
	}
	if bytes.Contains(providerCiphertext, []byte(session.ProviderCode)) {
		t.Fatal("wxpusher provider code was stored in plaintext")
	}
	secondActive := session
	secondActive.ID = "bind-2"
	if err := store.SaveWxPusherBindSession(secondActive); err == nil {
		t.Fatal("multiple active bind sessions were accepted for one user")
	}
	completedAt := now.Add(time.Minute)
	session.CompletedAt = &completedAt
	if err := store.SaveWxPusherBindSession(session); err != nil {
		t.Fatalf("complete bind session: %v", err)
	}
	if err := store.SaveWxPusherBindSession(secondActive); err != nil {
		t.Fatalf("save replacement bind session: %v", err)
	}
	invalidURL := secondActive
	invalidURL.ID = "bind-http"
	invalidURL.UserID = users[1].ID
	invalidURL.QRURL = "http://example.com/unsafe"
	if err := store.SaveWxPusherBindSession(invalidURL); err == nil {
		t.Fatal("non-https wxpusher qr url was accepted")
	}
}

func TestNotificationDeliveryQueueClaimsDueRowsOnceAndCascades(t *testing.T) {
	store, users, now := newWxPusherTestStore(t)
	defer store.Close()
	notification := model.Notification{
		ID: "notice-1", UserID: users[0].ID, Type: model.NotificationPileOffline,
		Severity: "warning", Title: "offline", Message: "offline", CreatedAt: now,
	}
	if err := store.SaveNotification(notification); err != nil {
		t.Fatalf("SaveNotification: %v", err)
	}
	createDelivery := func(delivery model.NotificationDelivery) {
		t.Helper()
		if err := store.SaveNotificationDelivery(delivery); err != nil {
			t.Fatalf("SaveNotificationDelivery %s: %v", delivery.ID, err)
		}
	}
	notificationID := notification.ID
	due := now.Add(-time.Minute)
	future := now.Add(time.Hour)
	staleClaim := now.Add(-20 * time.Minute)
	base := model.NotificationDelivery{
		UserID: users[0].ID, Channel: "wxpusher", Status: model.NotificationDeliveryPending,
		CreatedAt: now.Add(-2 * time.Minute), UpdatedAt: now.Add(-2 * time.Minute),
	}
	associated := base
	associated.ID = "delivery-associated"
	associated.NotificationID = &notificationID
	associated.NextAttemptAt = &due
	createDelivery(associated)
	duplicate := associated
	duplicate.ID = "delivery-duplicate"
	if err := store.SaveNotificationDelivery(duplicate); err == nil {
		t.Fatal("duplicate notification/channel delivery was accepted")
	}
	futureTest := base
	futureTest.ID = "delivery-future"
	futureTest.IsTest = true
	futureTest.NextAttemptAt = &future
	createDelivery(futureTest)
	retry := base
	retry.ID = "delivery-retry"
	retry.IsTest = true
	retry.Status = model.NotificationDeliveryRetryWait
	retry.AttemptCount = 1
	retry.NextAttemptAt = &due
	createDelivery(retry)
	stale := base
	stale.ID = "delivery-stale"
	stale.IsTest = true
	stale.Status = model.NotificationDeliverySending
	stale.AttemptCount = 1
	stale.ClaimedAt = &staleClaim
	createDelivery(stale)
	accepted := base
	accepted.ID = "delivery-accepted"
	accepted.IsTest = true
	accepted.Status = model.NotificationDeliveryAccepted
	accepted.ProviderRecordID = "record-accepted"
	accepted.AcceptedAt = &due
	accepted.NextAttemptAt = &due
	createDelivery(accepted)

	claimed, err := store.ClaimNotificationDeliveries(now, now.Add(-5*time.Minute), 10)
	if err != nil {
		t.Fatalf("ClaimNotificationDeliveries: %v", err)
	}
	if len(claimed) != 2 {
		t.Fatalf("claimed %d deliveries, want 2: %+v", len(claimed), claimed)
	}
	claimedIDs := make(map[string]model.NotificationDelivery, len(claimed))
	for _, delivery := range claimed {
		claimedIDs[delivery.ID] = delivery
		if delivery.Status != model.NotificationDeliverySending || delivery.ClaimedAt == nil || !delivery.ClaimedAt.Equal(now) {
			t.Fatalf("delivery was not atomically claimed: %+v", delivery)
		}
	}
	if claimedIDs[associated.ID].AttemptCount != 1 || claimedIDs[retry.ID].AttemptCount != 2 {
		t.Fatalf("unexpected claim attempts: %+v", claimedIDs)
	}
	if recovered, err := store.RecoverStaleSendingDeliveries(now.Add(-5*time.Minute), now); err != nil || recovered != 1 {
		t.Fatalf("RecoverStaleSendingDeliveries = %d, %v", recovered, err)
	}
	if loaded, ok, err := store.LoadNotificationDelivery(users[0].ID, stale.ID); err != nil || !ok || loaded.Status != model.NotificationDeliveryUncertain {
		t.Fatalf("stale delivery recovery = %+v, ok %v, err %v", loaded, ok, err)
	}
	acceptedClaims, err := store.ClaimAcceptedNotificationDeliveries(now, now.Add(-5*time.Minute), 10)
	if err != nil || len(acceptedClaims) != 1 || acceptedClaims[0].ID != accepted.ID {
		t.Fatalf("accepted delivery claims = %+v, err %v", acceptedClaims, err)
	}
	claimedAgain, err := store.ClaimNotificationDeliveries(now.Add(time.Second), now.Add(-5*time.Minute), 10)
	if err != nil || len(claimedAgain) != 0 {
		t.Fatalf("fresh claims were reclaimed: %+v, err %v", claimedAgain, err)
	}
	if loaded, ok, err := store.LoadNotificationDelivery(users[0].ID, associated.ID); err != nil || !ok || loaded.AttemptCount != 1 {
		t.Fatalf("LoadNotificationDelivery = %+v, ok %v, err %v", loaded, ok, err)
	}
	if err := store.SaveWxPusherBinding(model.WxPusherBinding{
		UserID: users[0].ID, UID: "UID_cascade", Enabled: true,
		EventTypes: model.WxPusherDefaultEventTypes, BoundAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("save binding for cascade: %v", err)
	}
	if err := store.SaveWxPusherBindSession(model.WxPusherBindSession{
		ID: "bind-cascade", UserID: users[0].ID, ProviderCode: "cascade-code",
		QRURL:     "https://wxpusher.zjiecode.com/api/qrcode/cascade",
		CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute), NextPollAt: now.Add(10 * time.Second),
	}); err != nil {
		t.Fatalf("save bind session for cascade: %v", err)
	}

	if _, err := store.db.Exec(`DELETE FROM users WHERE id=?`, users[0].ID); err != nil {
		t.Fatalf("delete delivery user: %v", err)
	}
	for _, table := range []string{"wxpusher_bindings", "wxpusher_bind_sessions", "notification_deliveries"} {
		var count int
		if err := store.db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE user_id=?`, users[0].ID).Scan(&count); err != nil {
			t.Fatalf("count cascaded %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("%s retained %d rows after user deletion", table, count)
		}
	}
}

func newWxPusherTestStore(t *testing.T) (*Store, []model.User, time.Time) {
	t.Helper()
	store, err := OpenSQLite(t.TempDir()+"/wxpusher.db", bytes.Repeat([]byte{0x74}, CookieKeySize))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	now := time.Date(2026, 8, 17, 8, 0, 0, 0, time.UTC)
	users := []model.User{
		{ID: "wx-user-1", Username: "alice", PasswordHash: "hash", Role: model.RoleUser, Enabled: true, CreatedAt: now, UpdatedAt: now},
		{ID: "wx-user-2", Username: "bob", PasswordHash: "hash", Role: model.RoleUser, Enabled: true, CreatedAt: now, UpdatedAt: now},
	}
	states := map[string]UserState{users[0].ID: {}, users[1].ID: {}}
	if err := store.Save(State{Version: schemaVersion, Users: users, UserStates: states}); err != nil {
		store.Close()
		t.Fatalf("Save users: %v", err)
	}
	return store, users, now
}

func TestWxPusherModelsRejectOversizedProviderErrors(t *testing.T) {
	store, users, now := newWxPusherTestStore(t)
	defer store.Close()
	delivery := model.NotificationDelivery{
		ID: "invalid-error", UserID: users[0].ID, IsTest: true,
		Status:           model.NotificationDeliveryFailed,
		LastErrorMessage: strings.Repeat("x", 513),
		CreatedAt:        now, UpdatedAt: now,
	}
	if err := store.SaveNotificationDelivery(delivery); err == nil {
		t.Fatal("oversized provider error was accepted")
	}
}

func TestWxPusherOperationsStatusSeparatesSystemAndBindingFailures(t *testing.T) {
	store, users, now := newWxPusherTestStore(t)
	defer store.Close()
	if err := store.SaveWxPusherBinding(model.WxPusherBinding{
		UserID: users[0].ID, UID: "UID_operations", Enabled: true,
		EventTypes: model.WxPusherDefaultEventTypes, BoundAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("SaveWxPusherBinding: %v", err)
	}
	succeededAt := now.Add(-10 * time.Minute)
	acceptedAt := succeededAt.Add(-time.Minute)
	for _, delivery := range []model.NotificationDelivery{
		{
			ID: "operations-success", UserID: users[0].ID, IsTest: true,
			Status: model.NotificationDeliveryProviderSucceeded, AttemptCount: 1,
			AcceptedAt: &acceptedAt, ProviderSucceededAt: &succeededAt,
			CreatedAt: now.Add(-20 * time.Minute), UpdatedAt: succeededAt,
		},
		{
			ID: "operations-system-failure", UserID: users[0].ID, IsTest: true,
			Status: model.NotificationDeliveryRetryWait, AttemptCount: 1,
			LastErrorCode: "wxpusher_provider_timeout",
			NextAttemptAt: timePointerForWxPusherTest(now.Add(time.Minute)),
			CreatedAt:     now.Add(-15 * time.Minute), UpdatedAt: now.Add(-5 * time.Minute),
		},
		{
			ID: "operations-binding-failure", UserID: users[1].ID, IsTest: true,
			Status: model.NotificationDeliveryFailed, AttemptCount: 1,
			LastErrorCode: "wxpusher_invalid_uid",
			CreatedAt:     now.Add(-12 * time.Minute), UpdatedAt: now.Add(-4 * time.Minute),
		},
	} {
		if err := store.SaveNotificationDelivery(delivery); err != nil {
			t.Fatalf("SaveNotificationDelivery %s: %v", delivery.ID, err)
		}
	}
	status, err := store.WxPusherOperationsStatus(now.Add(-24 * time.Hour))
	if err != nil {
		t.Fatalf("WxPusherOperationsStatus: %v", err)
	}
	if status.ActiveBindings != 1 || status.Attempts24Hours != 3 || status.Accepted24Hours != 1 ||
		status.ProviderSucceeded24Hours != 1 || status.RetryingDeliveries != 1 ||
		status.SystemFailures24Hours != 1 || status.BindingFailures24Hours != 1 ||
		status.AffectedBindingUsers != 1 || status.ConsecutiveSystemFailures != 1 ||
		status.LastErrorCategory != "binding_invalid" {
		t.Fatalf("unexpected wxpusher operations status: %+v", status)
	}
}

func timePointerForWxPusherTest(value time.Time) *time.Time {
	return &value
}
