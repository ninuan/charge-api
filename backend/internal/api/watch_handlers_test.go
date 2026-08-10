package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"charge-dashboard/internal/model"
)

func TestWatchRuleAPIIsAuthenticatedScopedAndStable(t *testing.T) {
	fixture := newHistoryAPIFixture(t)
	const deviceID = "2601201412385560088"

	anonymous := watchAPIRequest(t, fixture, "", http.MethodGet, "/api/watch-rules", "")
	if anonymous.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d, want 401", anonymous.Code)
	}
	create := watchAPIRequest(
		t, fixture, fixture.owner.ID, http.MethodPost, "/api/watch-rules",
		`{"deviceId":"`+deviceID+`","portId":1,"notifyIdle":true}`,
	)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d: %s", create.Code, create.Body.String())
	}
	if create.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("create cache control = %q", create.Header().Get("Cache-Control"))
	}
	var rule model.WatchRule
	if err := json.NewDecoder(create.Body).Decode(&rule); err != nil {
		t.Fatalf("decode created watch rule: %v", err)
	}
	if rule.UserID != fixture.owner.ID || rule.DeviceID != deviceID || rule.PortID == nil || *rule.PortID != 1 || !rule.NotifyIdle {
		t.Fatalf("unexpected created rule: %+v", rule)
	}

	list := watchAPIRequest(t, fixture, fixture.owner.ID, http.MethodGet, "/api/watch-rules", "")
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", list.Code, list.Body.String())
	}
	var rules []model.WatchRule
	if err := json.NewDecoder(list.Body).Decode(&rules); err != nil || len(rules) != 1 {
		t.Fatalf("watch rule list = %+v, err %v", rules, err)
	}
	otherList := watchAPIRequest(t, fixture, fixture.other.ID, http.MethodGet, "/api/watch-rules", "")
	if otherList.Code != http.StatusOK || strings.TrimSpace(otherList.Body.String()) != "[]" {
		t.Fatalf("other user list = %d: %s", otherList.Code, otherList.Body.String())
	}

	duplicate := watchAPIRequest(
		t, fixture, fixture.owner.ID, http.MethodPost, "/api/watch-rules",
		`{"deviceId":"`+deviceID+`","portId":1,"notifyIdle":false}`,
	)
	if duplicate.Code != http.StatusConflict || !strings.Contains(duplicate.Body.String(), "WATCH_RULE_CONFLICT") {
		t.Fatalf("duplicate status = %d: %s", duplicate.Code, duplicate.Body.String())
	}
	unknownPort := watchAPIRequest(
		t, fixture, fixture.owner.ID, http.MethodPost, "/api/watch-rules",
		`{"deviceId":"`+deviceID+`","portId":10,"notifyIdle":true}`,
	)
	if unknownPort.Code != http.StatusNotFound || !strings.Contains(unknownPort.Body.String(), "WATCH_TARGET_NOT_FOUND") {
		t.Fatalf("unknown port status = %d: %s", unknownPort.Code, unknownPort.Body.String())
	}

	disabled := watchAPIRequest(
		t, fixture, fixture.owner.ID, http.MethodPatch, "/api/watch-rules/"+rule.ID,
		`{"enabled":false}`,
	)
	if disabled.Code != http.StatusOK || !strings.Contains(disabled.Body.String(), `"enabled":false`) {
		t.Fatalf("disable status = %d: %s", disabled.Code, disabled.Body.String())
	}
	crossUserUpdate := watchAPIRequest(
		t, fixture, fixture.other.ID, http.MethodPatch, "/api/watch-rules/"+rule.ID,
		`{"enabled":true}`,
	)
	if crossUserUpdate.Code != http.StatusNotFound || !strings.Contains(crossUserUpdate.Body.String(), "WATCH_RULE_NOT_FOUND") {
		t.Fatalf("cross-user update = %d: %s", crossUserUpdate.Code, crossUserUpdate.Body.String())
	}
	deleted := watchAPIRequest(t, fixture, fixture.owner.ID, http.MethodDelete, "/api/watch-rules/"+rule.ID, "")
	if deleted.Code != http.StatusNoContent || deleted.Body.Len() != 0 {
		t.Fatalf("delete status = %d: %s", deleted.Code, deleted.Body.String())
	}
}

func TestNotificationAPIManagesPreferencesInboxAndOwnership(t *testing.T) {
	fixture := newHistoryAPIFixture(t)

	preference := watchAPIRequest(t, fixture, fixture.owner.ID, http.MethodGet, "/api/notification-preferences", "")
	if preference.Code != http.StatusOK || !strings.Contains(preference.Body.String(), `"quietStartMinute":1320`) {
		t.Fatalf("default preference = %d: %s", preference.Code, preference.Body.String())
	}
	updatedPreference := watchAPIRequest(
		t, fixture, fixture.owner.ID, http.MethodPatch, "/api/notification-preferences",
		`{"browserEnabled":true,"quietStartMinute":1380}`,
	)
	if updatedPreference.Code != http.StatusOK ||
		!strings.Contains(updatedPreference.Body.String(), `"browserEnabled":true`) ||
		!strings.Contains(updatedPreference.Body.String(), `"quietStartMinute":1380`) {
		t.Fatalf("updated preference = %d: %s", updatedPreference.Code, updatedPreference.Body.String())
	}
	invalidPreference := watchAPIRequest(
		t, fixture, fixture.owner.ID, http.MethodPatch, "/api/notification-preferences",
		`{"quietEndMinute":1440}`,
	)
	if invalidPreference.Code != http.StatusBadRequest || !strings.Contains(invalidPreference.Body.String(), "NOTIFICATION_PREFERENCE_INVALID") {
		t.Fatalf("invalid preference = %d: %s", invalidPreference.Code, invalidPreference.Body.String())
	}

	notification, err := fixture.manager.RecordNotification(model.Notification{
		UserID: fixture.owner.ID, Type: model.NotificationCredentialExpired,
		Severity: "warning", Title: "凭据已失效", Message: "请重新扫码登录",
		DedupeKey: "credential-expired",
	})
	if err != nil {
		t.Fatalf("RecordNotification: %v", err)
	}
	resolvedAt := time.Now().UTC().Truncate(time.Second)
	if _, err := fixture.manager.RecordNotification(model.Notification{
		UserID: fixture.owner.ID, Type: model.NotificationPileRecovered,
		Severity: "info", Title: "充电桩已恢复", Message: "设备已恢复在线",
		DeviceID: "2601201412385560088", ResolvedAt: &resolvedAt,
	}); err != nil {
		t.Fatalf("RecordNotification resolved: %v", err)
	}

	list := watchAPIRequest(t, fixture, fixture.owner.ID, http.MethodGet, "/api/notifications?limit=1&status=all", "")
	if list.Code != http.StatusOK {
		t.Fatalf("notification list = %d: %s", list.Code, list.Body.String())
	}
	var page model.NotificationPage
	if err := json.NewDecoder(list.Body).Decode(&page); err != nil {
		t.Fatalf("decode notification page: %v", err)
	}
	if len(page.Items) != 1 || page.NextCursor == "" || page.UnreadCount != 2 {
		t.Fatalf("unexpected notification page: %+v", page)
	}
	invalidCursor := watchAPIRequest(t, fixture, fixture.owner.ID, http.MethodGet, "/api/notifications?cursor=missing", "")
	if invalidCursor.Code != http.StatusBadRequest || !strings.Contains(invalidCursor.Body.String(), "NOTIFICATION_QUERY_INVALID") {
		t.Fatalf("invalid cursor = %d: %s", invalidCursor.Code, invalidCursor.Body.String())
	}
	otherList := watchAPIRequest(t, fixture, fixture.other.ID, http.MethodGet, "/api/notifications", "")
	if otherList.Code != http.StatusOK || !strings.Contains(otherList.Body.String(), `"items":[]`) {
		t.Fatalf("other notification list = %d: %s", otherList.Code, otherList.Body.String())
	}
	crossUserRead := watchAPIRequest(
		t, fixture, fixture.other.ID, http.MethodPost,
		"/api/notifications/"+notification.ID+"/read", "",
	)
	if crossUserRead.Code != http.StatusNotFound || !strings.Contains(crossUserRead.Body.String(), "NOTIFICATION_NOT_FOUND") {
		t.Fatalf("cross-user read = %d: %s", crossUserRead.Code, crossUserRead.Body.String())
	}
	read := watchAPIRequest(
		t, fixture, fixture.owner.ID, http.MethodPost,
		"/api/notifications/"+notification.ID+"/read", "",
	)
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), `"readAt"`) {
		t.Fatalf("mark notification read = %d: %s", read.Code, read.Body.String())
	}
	readAll := watchAPIRequest(t, fixture, fixture.owner.ID, http.MethodPost, "/api/notifications/read-all", "")
	if readAll.Code != http.StatusOK || !strings.Contains(readAll.Body.String(), `"updated":1`) {
		t.Fatalf("mark all read = %d: %s", readAll.Code, readAll.Body.String())
	}
	clearResolved := watchAPIRequest(t, fixture, fixture.owner.ID, http.MethodDelete, "/api/notifications/resolved", "")
	if clearResolved.Code != http.StatusOK || !strings.Contains(clearResolved.Body.String(), `"deleted":1`) {
		t.Fatalf("clear resolved = %d: %s", clearResolved.Code, clearResolved.Body.String())
	}
}

func TestWatchAndNotificationAPIsRejectUnsupportedMethodsAndBodies(t *testing.T) {
	fixture := newHistoryAPIFixture(t)
	tests := []struct {
		method string
		path   string
		body   string
		status int
	}{
		{method: http.MethodPut, path: "/api/watch-rules", status: http.StatusMethodNotAllowed},
		{method: http.MethodPost, path: "/api/notification-preferences", status: http.StatusMethodNotAllowed},
		{method: http.MethodPost, path: "/api/notifications", status: http.StatusMethodNotAllowed},
		{method: http.MethodPost, path: "/api/watch-rules", body: `{"unknown":true}`, status: http.StatusBadRequest},
	}
	for _, test := range tests {
		recorder := watchAPIRequest(t, fixture, fixture.owner.ID, test.method, test.path, test.body)
		if recorder.Code != test.status {
			t.Fatalf("%s %s status = %d, want %d: %s", test.method, test.path, recorder.Code, test.status, recorder.Body.String())
		}
	}
}

func watchAPIRequest(
	t *testing.T,
	fixture historyAPIFixture,
	userID, method, path, body string,
) *httptest.ResponseRecorder {
	t.Helper()
	var request *http.Request
	if body == "" {
		request = httptest.NewRequest(method, path, nil)
	} else {
		request = httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
	}
	if userID != "" {
		session, err := fixture.sessions.Create(userID)
		if err != nil {
			t.Fatalf("Create session: %v", err)
		}
		request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.Token})
	}
	recorder := httptest.NewRecorder()
	fixture.mux.ServeHTTP(recorder, request)
	return recorder
}
