package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"charge-dashboard/internal/model"
	appruntime "charge-dashboard/internal/runtime"
)

func (s *Server) handleWatchRules(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireDashboardUser(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	switch r.Method {
	case http.MethodGet:
		rules, err := s.manager.WatchRules(user.ID)
		if err != nil {
			s.writeWatchError(w, "list_watch_rules", err)
			return
		}
		s.setHealthDegraded("watch", "")
		writeJSON(w, http.StatusOK, rules)
	case http.MethodPost:
		var request model.WatchRuleCreateRequest
		if !decodeJSON(w, r, watchBodyLimit, &request) {
			return
		}
		result, err := s.manager.CreateWatchRuleWithInitialCheck(user.ID, request)
		if err != nil {
			s.writeWatchError(w, "create_watch_rule", err)
			return
		}
		s.setHealthDegraded("watch", "")
		writeJSON(w, http.StatusCreated, result)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleWatchOverview(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireDashboardUser(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	overview, err := s.manager.WatchOverview(user.ID)
	if err != nil {
		s.writeWatchError(w, "load_watch_overview", err)
		return
	}
	s.setHealthDegraded("watch", "")
	writeJSON(w, http.StatusOK, overview)
}

func (s *Server) handleWatchRuleActions(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireDashboardUser(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	ruleID := strings.TrimPrefix(r.URL.Path, "/api/watch-rules/")
	if !validAPIResourceID(ruleID) || strings.Contains(ruleID, "/") {
		writeCodedError(w, http.StatusNotFound, "WATCH_RULE_NOT_FOUND", "未找到空闲提醒规则")
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var request model.WatchRuleUpdateRequest
		if !decodeJSON(w, r, watchBodyLimit, &request) {
			return
		}
		rule, err := s.manager.UpdateWatchRule(user.ID, ruleID, request)
		if err != nil {
			s.writeWatchError(w, "update_watch_rule", err)
			return
		}
		s.setHealthDegraded("watch", "")
		writeJSON(w, http.StatusOK, rule)
	case http.MethodDelete:
		if err := s.manager.DeleteWatchRule(user.ID, ruleID); err != nil {
			s.writeWatchError(w, "delete_watch_rule", err)
			return
		}
		s.setHealthDegraded("watch", "")
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleNotificationPreference(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireDashboardUser(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	switch r.Method {
	case http.MethodGet:
		preference, err := s.manager.NotificationPreference(user.ID)
		if err != nil {
			s.writeNotificationError(w, "load_notification_preference", err)
			return
		}
		s.setHealthDegraded("notifications", "")
		writeJSON(w, http.StatusOK, preference)
	case http.MethodPatch:
		var request model.NotificationPreferenceUpdateRequest
		if !decodeJSON(w, r, watchBodyLimit, &request) {
			return
		}
		preference, err := s.manager.UpdateNotificationPreference(user.ID, request)
		if err != nil {
			s.writeNotificationError(w, "update_notification_preference", err)
			return
		}
		s.setHealthDegraded("notifications", "")
		writeJSON(w, http.StatusOK, preference)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleNotifications(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireDashboardUser(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	limit := 0
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		value, err := strconv.Atoi(rawLimit)
		if err != nil {
			writeCodedError(w, http.StatusBadRequest, "NOTIFICATION_QUERY_INVALID", "通知筛选或分页参数无效")
			return
		}
		limit = value
	}
	page, err := s.manager.Notifications(
		user.ID,
		r.URL.Query().Get("cursor"),
		r.URL.Query().Get("status"),
		limit,
	)
	if err != nil {
		s.writeNotificationError(w, "list_notifications", err)
		return
	}
	s.setHealthDegraded("notifications", "")
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) handleNotificationActions(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireDashboardUser(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	path := strings.TrimPrefix(r.URL.Path, "/api/notifications/")
	switch path {
	case "read-all":
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		updated, err := s.manager.MarkAllNotificationsRead(user.ID)
		if err != nil {
			s.writeNotificationError(w, "mark_all_notifications_read", err)
			return
		}
		s.setHealthDegraded("notifications", "")
		writeJSON(w, http.StatusOK, map[string]int64{"updated": updated})
		return
	case "resolved":
		if r.Method != http.MethodDelete {
			methodNotAllowed(w)
			return
		}
		deleted, err := s.manager.DeleteResolvedNotifications(user.ID)
		if err != nil {
			s.writeNotificationError(w, "delete_resolved_notifications", err)
			return
		}
		s.setHealthDegraded("notifications", "")
		writeJSON(w, http.StatusOK, map[string]int64{"deleted": deleted})
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[1] != "read" || !validAPIResourceID(parts[0]) {
		writeCodedError(w, http.StatusNotFound, "NOTIFICATION_NOT_FOUND", "未找到通知")
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	notification, err := s.manager.MarkNotificationRead(user.ID, parts[0])
	if err != nil {
		s.writeNotificationError(w, "mark_notification_read", err)
		return
	}
	s.setHealthDegraded("notifications", "")
	writeJSON(w, http.StatusOK, notification)
}

func (s *Server) writeWatchError(w http.ResponseWriter, operation string, err error) {
	switch {
	case errors.Is(err, appruntime.ErrWatchRuleInvalid):
		writeCodedError(w, http.StatusBadRequest, "WATCH_RULE_INVALID", "空闲提醒内容无效")
	case errors.Is(err, appruntime.ErrWatchTargetNotFound):
		writeCodedError(w, http.StatusNotFound, "WATCH_TARGET_NOT_FOUND", "未找到当前账户下的充电桩")
	case errors.Is(err, appruntime.ErrWatchRuleNotFound):
		writeCodedError(w, http.StatusNotFound, "WATCH_RULE_NOT_FOUND", "未找到空闲提醒规则")
	case errors.Is(err, appruntime.ErrWatchRuleConflict):
		writeCodedError(w, http.StatusConflict, "WATCH_RULE_CONFLICT", "该充电桩已设置空闲提醒")
	case errors.Is(err, appruntime.ErrWatchPileLimit):
		writeCodedError(w, http.StatusConflict, "WATCH_PILE_LIMIT_REACHED", "已达到当前账户的空闲提醒充电桩上限")
	case errors.Is(err, appruntime.ErrWatchPowerOff):
		var outage appruntime.WatchPowerOffError
		if errors.As(err, &outage) {
			writeCodedError(w, http.StatusConflict, "WATCH_POWER_OFF_ACTIVE", outage.Error())
			return
		}
		writeCodedError(w, http.StatusConflict, "WATCH_POWER_OFF_ACTIVE", "当前处于计划断电时段，请在恢复供电后再开启提醒")
	case errors.Is(err, appruntime.ErrWatchCredentialExpired):
		writeCodedError(w, http.StatusConflict, "WATCH_CREDENTIAL_EXPIRED", "登录已过期，请先重新扫码后再开启提醒")
	case errors.Is(err, appruntime.ErrWatchRecurringDisabled):
		writeCodedError(w, http.StatusConflict, "WATCH_RECURRING_DISABLED", "固定时段提醒已停用，请开启临时提醒")
	default:
		s.setHealthDegraded("watch", "空闲提醒存储异常")
		logStructuredError(operation, "watch", err)
		writeCodedError(w, http.StatusServiceUnavailable, "WATCH_UNAVAILABLE", "空闲提醒功能暂时不可用，请稍后重试")
	}
}

func (s *Server) writeNotificationError(w http.ResponseWriter, operation string, err error) {
	switch {
	case errors.Is(err, appruntime.ErrNotificationQueryInvalid):
		writeCodedError(w, http.StatusBadRequest, "NOTIFICATION_QUERY_INVALID", "通知筛选或分页参数无效")
	case errors.Is(err, appruntime.ErrNotificationInputInvalid):
		writeCodedError(w, http.StatusBadRequest, "NOTIFICATION_PREFERENCE_INVALID", "通知偏好内容无效")
	case errors.Is(err, appruntime.ErrNotificationNotFound):
		writeCodedError(w, http.StatusNotFound, "NOTIFICATION_NOT_FOUND", "未找到通知")
	default:
		s.setHealthDegraded("notifications", "通知存储异常")
		logStructuredError(operation, "notifications", err)
		writeCodedError(w, http.StatusServiceUnavailable, "NOTIFICATION_UNAVAILABLE", "通知功能暂时不可用，请稍后重试")
	}
}

func validAPIResourceID(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || strings.ContainsRune("_.-", character) {
			continue
		}
		return false
	}
	return true
}
