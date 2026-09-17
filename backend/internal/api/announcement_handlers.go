package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"charge-dashboard/internal/model"
)

func (s *Server) announcementError(w http.ResponseWriter, err error) {
	status, message := http.StatusInternalServerError, "公告暂时不可用，请稍后重试。"
	code := "ANNOUNCEMENT_UNAVAILABLE"
	switch {
	case errors.Is(err, model.ErrAnnouncementConflict):
		status = 409
		code = "ANNOUNCEMENT_CONFLICT"
		message = "公告已更新或状态已变化，请重新加载后操作。"
	case errors.Is(err, model.ErrAnnouncementUnavailable):
		status = 404
		code = "ANNOUNCEMENT_NOT_FOUND"
		message = "公告不存在、已撤下或当前不可操作。"
	case errors.Is(err, model.ErrAnnouncementInvalid):
		status = 400
		code = "ANNOUNCEMENT_INVALID"
		message = "请检查公告内容、链接和有效时间。"
	}
	writeJSON(w, status, map[string]string{"error": message, "code": code})
}
func (s *Server) handleAnnouncements(w http.ResponseWriter, r *http.Request) {
	admin := strings.HasPrefix(r.URL.Path, "/api/admin/")
	var user model.CurrentUser
	var ok bool
	if admin {
		user, ok = s.requireAdmin(w, r)
	} else {
		user, ok = s.requireDashboardUser(w, r)
	}
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	prefix := "/api/announcements"
	if admin {
		prefix = "/api/admin/announcements"
	}
	path := strings.TrimPrefix(r.URL.Path, prefix)
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	id := parts[0]
	if len(parts) > 2 {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodGet {
		if admin && len(parts) == 2 && parts[1] == "revisions" {
			page := 1
			if raw := r.URL.Query().Get("page"); raw != "" {
				n, err := strconv.Atoi(raw)
				if err != nil || n < 1 || n > 100000 {
					s.announcementError(w, model.ErrAnnouncementInvalid)
					return
				}
				page = n
			}
			out, err := s.manager.AnnouncementRevisions(r.Context(), id, page)
			if err != nil {
				s.announcementError(w, err)
				return
			}
			writeJSON(w, 200, out)
			return
		}
		if len(parts) > 1 {
			http.NotFound(w, r)
			return
		}
		if !admin && id == "summary" {
			out, err := s.manager.AnnouncementSummary(r.Context(), user.ID)
			if err != nil {
				s.announcementError(w, err)
				return
			}
			writeJSON(w, 200, out)
			return
		}
		if id != "" {
			out, err := s.manager.AnnouncementDetail(r.Context(), user.ID, id, admin)
			if err != nil {
				s.announcementError(w, err)
				return
			}
			writeJSON(w, 200, out)
			return
		}
		page := 1
		if raw := r.URL.Query().Get("page"); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 || n > 100000 {
				s.announcementError(w, model.ErrAnnouncementInvalid)
				return
			}
			page = n
		}
		state := r.URL.Query().Get("status")
		if state != "" && state != "active" && state != "ended" && (!admin || state != "draft" && state != "scheduled" && state != "withdrawn") {
			s.announcementError(w, model.ErrAnnouncementInvalid)
			return
		}
		out, err := s.manager.AnnouncementList(r.Context(), user.ID, admin, state, r.URL.Query().Get("q"), page)
		if err != nil {
			s.announcementError(w, err)
			return
		}
		writeJSON(w, 200, out)
		return
	}
	if !admin {
		if r.Method != http.MethodPost || len(parts) != 2 || id == "" || parts[1] != "acknowledge" {
			methodNotAllowed(w)
			return
		}
		var in struct {
			Version         int `json:"version"`
			ReminderVersion int `json:"reminderVersion"`
		}
		if !decodeJSON(w, r, 1024, &in) {
			return
		}
		if err := s.manager.AcknowledgeAnnouncement(r.Context(), user.ID, id, in.Version, in.ReminderVersion); err != nil {
			s.announcementError(w, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	action := ""
	switch {
	case r.Method == http.MethodPost && id == "":
		action = "save"
	case r.Method == http.MethodPatch && id != "" && len(parts) == 1:
		action = "save"
	case r.Method == http.MethodDelete && id != "" && len(parts) == 1:
		action = "delete"
	case r.Method == http.MethodPost && id != "" && len(parts) == 2 && (parts[1] == "publish" || parts[1] == "withdraw"):
		action = parts[1]
	default:
		methodNotAllowed(w)
		return
	}
	var in model.AnnouncementInput
	if !decodeJSON(w, r, 32*1024, &in) {
		return
	}
	out, err := s.manager.SaveAnnouncement(r.Context(), id, user.ID, action, in)
	if err != nil {
		s.announcementError(w, err)
		return
	}
	s.recordAdminAudit(user, "announcement."+action, "announcement", out.ID, out.Title, "success")
	writeJSON(w, 200, out)
}
