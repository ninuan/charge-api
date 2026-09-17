package persistence

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"charge-dashboard/internal/model"
)

func TestAnnouncementLifecycleAndAccountIsolation(t *testing.T) {
	s, err := OpenSQLite(t.TempDir()+"/a.db", bytes.Repeat([]byte{1}, CookieKeySize))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Now().UTC().Truncate(time.Millisecond)
	end := now.Add(time.Hour)
	users := []model.User{}
	for _, id := range []string{"u1", "u2"} {
		users = append(users, model.User{ID: id, Username: id, PasswordHash: "hash", Role: model.RoleUser, Enabled: true, CreatedAt: now, UpdatedAt: now})
	}
	if err = s.Save(State{Users: users, UserStates: map[string]UserState{}}); err != nil {
		t.Fatal(err)
	}
	in := model.AnnouncementInput{Title: "维护", Body: "消息", Level: "normal", StartAt: now, EndAt: &end}
	a, err := s.SaveAnnouncement(t.Context(), "", "admin", "save", in, now)
	if err != nil {
		t.Fatal(err)
	}
	list, err := s.Announcements(t.Context(), "u1", false, now)
	if err != nil || len(list) != 0 {
		t.Fatalf("draft visible: %v %v", list, err)
	}
	a, err = s.SaveAnnouncement(t.Context(), a.ID, "admin", "publish", model.AnnouncementInput{Version: a.Version}, now)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = s.AcknowledgeAnnouncement(t.Context(), "u1", a.ID, a.Version, a.ReminderVersion, now); err != nil {
			t.Fatal(err)
		}
	}
	for _, user := range []string{"u1", "u2"} {
		list, err = s.Announcements(t.Context(), user, false, now)
		if err != nil || len(list) != 1 || list[0].Acknowledged != (user == "u1") {
			t.Fatalf("isolation: %v %v", list, err)
		}
	}
	old := a
	in.Version = a.Version
	in.Body = "修正"
	a, err = s.SaveAnnouncement(t.Context(), a.ID, "admin", "save", in, now)
	if err != nil {
		t.Fatal(err)
	}
	if a.ReminderVersion != old.ReminderVersion {
		t.Fatal("correction reset acknowledgement")
	}
	if err = s.AcknowledgeAnnouncement(t.Context(), "u2", a.ID, old.Version, old.ReminderVersion, now); !errors.Is(err, model.ErrAnnouncementConflict) {
		t.Fatalf("stale ack: %v", err)
	}
	if _, err = s.SaveAnnouncement(t.Context(), a.ID, "admin", "save", in, now); !errors.Is(err, model.ErrAnnouncementConflict) {
		t.Fatalf("lost update: %v", err)
	}
	in.Version = a.Version
	in.RemindAgain = true
	a, err = s.SaveAnnouncement(t.Context(), a.ID, "admin", "save", in, now)
	if err != nil {
		t.Fatal(err)
	}
	list, err = s.Announcements(t.Context(), "u1", false, now)
	if err != nil || list[0].Acknowledged {
		t.Fatalf("re-remind: %v %v", list, err)
	}
	if err = s.AcknowledgeAnnouncement(t.Context(), "u1", a.ID, a.Version, a.ReminderVersion, end); !errors.Is(err, model.ErrAnnouncementUnavailable) {
		t.Fatalf("end boundary: %v", err)
	}
	list, err = s.Announcements(t.Context(), "u1", false, end)
	if err != nil || list[0].Status != "ended" {
		t.Fatalf("ended history: %v %v", list, err)
	}
	a, err = s.SaveAnnouncement(t.Context(), a.ID, "admin", "withdraw", model.AnnouncementInput{Version: a.Version}, now)
	if err != nil {
		t.Fatal(err)
	}
	list, err = s.Announcements(t.Context(), "u1", false, now)
	if err != nil || len(list) != 0 {
		t.Fatalf("withdrawn visible: %v %v", list, err)
	}
	if _, err = s.SaveAnnouncement(t.Context(), a.ID, "admin", "delete", model.AnnouncementInput{Version: a.Version}, now); !errors.Is(err, model.ErrAnnouncementConflict) {
		t.Fatalf("published deletion: %v", err)
	}
	var revisions int
	if err = s.db.QueryRow(`SELECT COUNT(*) FROM announcement_revisions WHERE announcement_id=?`, a.ID).Scan(&revisions); err != nil || revisions != 5 {
		t.Fatalf("audit revisions=%d %v", revisions, err)
	}
	// Reopening/initialization is additive and retains published content/confirmations.
	if err = s.initialize(); err != nil {
		t.Fatal(err)
	}
}
func TestAnnouncementScheduleAndValidation(t *testing.T) {
	now := time.Now().UTC()
	end := now.Add(time.Hour)
	a := model.Announcement{Status: "published", StartAt: now, EndAt: &end}
	if a.State(now.Add(-time.Nanosecond)) != "scheduled" || a.State(now) != "active" || a.State(end) != "ended" {
		t.Fatal("invalid time boundary")
	}
	in := model.AnnouncementInput{Title: "标题", Body: "正文", Level: "important", StartAt: now}
	for _, link := range []string{"javascript:alert(1)", "//evil.test", "/\\evil.test", "http://example.com", "https://user:pass@example.com"} {
		in.LinkURL = link
		in.LinkLabel = "打开"
		if in.Validate() == nil {
			t.Fatalf("unsafe link accepted: %s", link)
		}
	}
	for _, link := range []string{"/account?tab=connection", "https://example.com/path"} {
		in.LinkURL = link
		if err := in.Validate(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAnnouncementQueryPaginationScheduleAndReopen(t *testing.T) {
	path := t.TempDir() + "/announcements.db"
	key := bytes.Repeat([]byte{2}, CookieKeySize)
	s, err := OpenSQLite(path, key)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	future := now.Add(time.Hour)
	for i := 0; i < 22; i++ {
		start := now
		if i == 21 {
			start = future
		}
		in := model.AnnouncementInput{Title: "公告", Body: "不应随列表传输的正文", Level: "normal", StartAt: start}
		a, e := s.SaveAnnouncement(t.Context(), "", "admin", "save", in, now)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = s.SaveAnnouncement(t.Context(), a.ID, "admin", "publish", model.AnnouncementInput{Version: a.Version}, now); e != nil {
			t.Fatal(e)
		}
	}
	boundary, err := s.AnnouncementBoundary(t.Context(), now)
	if err != nil || boundary == nil || !boundary.Equal(future) {
		t.Fatalf("boundary %v %v", boundary, err)
	}
	for page, count := range map[int]int{1: 20, 2: 1, 3: 0} {
		out, e := s.QueryAnnouncementPage(t.Context(), "u", false, "", "", "", page, false, now)
		if e != nil || out.Total != 21 || len(out.Items) != count {
			t.Fatalf("page %d %+v %v", page, out, e)
		}
		for _, a := range out.Items {
			if a.Body != "" {
				t.Fatal("list leaked body")
			}
		}
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenSQLite(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	out, err := s.QueryAnnouncementPage(t.Context(), "u", false, "active", "", "", 1, false, future)
	if err != nil || out.Total != 22 {
		t.Fatalf("reopened scheduled visibility: %+v %v", out, err)
	}
}
