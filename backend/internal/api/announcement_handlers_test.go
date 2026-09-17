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

func TestAnnouncementAPIVisibilityAndSummary(t *testing.T) {
	s, m, sessions := newTestServer(t)
	admin := findUser(t, m, "admin")
	user, err := m.CreateUser(model.UserCreateRequest{Username: "ann-user", Password: "password123", Role: model.RoleUser})
	if err != nil {
		t.Fatal(err)
	}
	adminSession, err := sessions.Create(admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	userSession, err := sessions.Create(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	s.Register(mux)
	request := func(method, path, body, token string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if token != "" {
			r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	for _, tc := range []struct {
		path, token string
		status      int
	}{{"/api/announcements/summary", "", 401}, {"/api/announcements", adminSession.Token, 403}, {"/api/admin/announcements", userSession.Token, 403}} {
		if w := request("GET", tc.path, "", tc.token); w.Code != tc.status {
			t.Fatalf("%s: %d", tc.path, w.Code)
		}
	}
	in := model.AnnouncementInput{Title: "维护通知", Body: strings.Repeat("正文", 1000), Level: "normal", StartAt: time.Now().Add(-time.Minute)}
	raw, _ := json.Marshal(in)
	w := request("POST", "/api/admin/announcements", string(raw), adminSession.Token)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var a model.Announcement
	json.Unmarshal(w.Body.Bytes(), &a)
	if w = request("GET", "/api/announcements/"+a.ID, "", userSession.Token); w.Code != 404 {
		t.Fatalf("draft leak: %d", w.Code)
	}
	version := func() string {
		raw, _ := json.Marshal(map[string]int{"version": a.Version})
		return string(raw)
	}
	w = request("POST", "/api/admin/announcements/"+a.ID+"/publish", version(), adminSession.Token)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	json.Unmarshal(w.Body.Bytes(), &a)
	w = request("GET", "/api/announcements/summary", "", userSession.Token)
	var summary model.AnnouncementSummary
	json.Unmarshal(w.Body.Bytes(), &summary)
	if w.Code != 200 || summary.Item == nil || summary.Item.ID != a.ID || summary.Item.Body != "" || summary.UnreadCount != 1 || w.Body.Len() > 3072 || w.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("bad summary: %s", w.Body.String())
	}
	w = request("POST", "/api/announcements/"+a.ID+"/acknowledge", strings.TrimSuffix(version(), "}")+`,"reminderVersion":1}`, userSession.Token)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	w = request("GET", "/api/announcements/summary", "", userSession.Token)
	json.Unmarshal(w.Body.Bytes(), &summary)
	if summary.Item != nil || summary.UnreadCount != 0 {
		t.Fatalf("normal ack remains: %s", w.Body.String())
	}
	in.Version = a.Version
	in.Level = "important"
	raw, _ = json.Marshal(in)
	w = request("PATCH", "/api/admin/announcements/"+a.ID, string(raw), adminSession.Token)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	w = request("GET", "/api/announcements/summary", "", userSession.Token)
	json.Unmarshal(w.Body.Bytes(), &summary)
	if summary.Item == nil || !summary.Item.Acknowledged {
		t.Fatalf("important ack missing: %s", w.Body.String())
	}
	w = request("GET", "/api/announcements?page=0", "", userSession.Token)
	if w.Code != 400 {
		t.Fatalf("invalid pagination: %d", w.Code)
	}
}
