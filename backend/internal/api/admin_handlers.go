package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"charge-dashboard/internal/model"
)

func (s *Server) handleAdminIncidents(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	if _, err := s.manager.AdminStatsResult(); err != nil {
		logStructuredError("sync_admin_incidents", "", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "异常列表暂时不可用"})
		return
	}
	values := r.URL.Query()
	issues, err := s.manager.Incidents(
		values.Get("status"),
		values.Get("type"),
		values.Get("level"),
	)
	if err != nil {
		writePublicOperationError(w, http.StatusBadRequest, "list admin incidents", "异常筛选条件无效。", err)
		return
	}
	writeJSON(w, http.StatusOK, issues)
}

func (s *Server) handleAdminIncidentActions(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPatch {
		methodNotAllowed(w)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/admin/incidents/")
	if id == "" || strings.Contains(id, "/") {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	var req model.IncidentUpdateRequest
	if !decodeJSON(w, r, adminBodyLimit, &req) {
		return
	}
	issue, err := s.manager.UpdateIncident(id, req, admin)
	if err != nil {
		s.recordAdminAudit(admin, "incident.update", "incident", id, id, "failure")
		writePublicOperationError(w, http.StatusBadRequest, "update admin incident", "更新异常处理状态失败。", err)
		return
	}
	s.recordAdminAudit(admin, "incident.update", "incident", id, issue.Username, "success")
	writeJSON(w, http.StatusOK, issue)
}

func (s *Server) handleAdminAudit(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	values := r.URL.Query()
	page, err := s.manager.AuditPage(
		queryInt(values.Get("page"), 1),
		queryInt(values.Get("pageSize"), 20),
	)
	if err != nil {
		logStructuredError("list_admin_audit", "", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "操作日志暂时不可用"})
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) handleAdminOperations(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	status, err := s.manager.OperationsStatus()
	if err != nil {
		s.setAdminDegraded("数据库运维查询失败")
		logStructuredError("load_operations_status", "", err)
		_ = s.manager.RecordSystemIncident(model.SystemException{
			ID: "operations-database", Username: "系统", Type: "database",
			Level: "critical", Message: "数据库运维检查失败", Time: time.Now(),
		})
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "运维信息暂时不可用"})
		return
	}
	if status.BackupState != "healthy" {
		at := status.CheckedAt.Truncate(24 * time.Hour)
		if status.LastBackupAt != nil {
			at = *status.LastBackupAt
		}
		_ = s.manager.RecordSystemIncident(model.SystemException{
			ID: "operations-backup", Username: "系统", Type: "backup",
			Level: "warning", Message: status.BackupMessage, Time: at,
		})
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) setAdminDegraded(reason string) {
	s.setHealthDegraded("admin", reason)
}

func (s *Server) setHealthDegraded(component, reason string) {
	s.healthMu.Lock()
	if s.healthDegradations == nil {
		s.healthDegradations = make(map[string]string)
	}
	if strings.TrimSpace(reason) == "" {
		delete(s.healthDegradations, component)
	} else {
		s.healthDegradations[component] = reason
	}
	s.healthMu.Unlock()
}

func (s *Server) adminDegradation() string {
	s.healthMu.RLock()
	defer s.healthMu.RUnlock()
	reasons := make([]string, 0, len(s.healthDegradations))
	for _, reason := range s.healthDegradations {
		if reason != "" {
			reasons = append(reasons, reason)
		}
	}
	sort.Strings(reasons)
	return strings.Join(reasons, "；")
}

func (s *Server) recordAdminAudit(
	admin model.CurrentUser,
	action, targetType, targetID, targetLabel, result string,
) {
	if err := s.manager.RecordAdminAudit(
		admin, action, targetType, targetID, targetLabel, result, "",
	); err != nil {
		logStructuredError("record_admin_audit", action, err)
	}
}

func adminUserUpdateAction(req model.UserUpdateRequest) string {
	switch {
	case req.Enabled != nil:
		return "user.enabled_update"
	case req.Role != nil:
		return "user.role_update"
	case req.DeviceLimit != nil:
		return "user.device_limit_update"
	case req.RefreshEnabled != nil:
		return "user.refresh_policy_update"
	default:
		return "user.update"
	}
}

func logStructuredError(operation, subject string, err error) {
	payload, _ := json.Marshal(map[string]string{
		"level": "error", "operation": operation,
		"subject": subject, "error": err.Error(),
	})
	log.Print(string(payload))
}

func (s *Server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.manager.ListUsersPage(adminUserListQuery(r)))
	case http.MethodPost:
		var req model.UserCreateRequest
		if !decodeJSON(w, r, adminBodyLimit, &req) {
			return
		}
		user, err := s.manager.CreateUser(req)
		if err != nil {
			s.recordAdminAudit(admin, "user.create", "user", "", req.Username, "failure")
			writePublicOperationError(w, http.StatusBadRequest, "create admin user", "创建用户失败，请检查填写信息后重试。", err)
			return
		}
		s.recordAdminAudit(admin, "user.create", "user", user.ID, user.Username, "success")
		writeJSON(w, http.StatusCreated, user)
	default:
		methodNotAllowed(w)
	}
}

func adminUserListQuery(r *http.Request) model.AdminUserListQuery {
	values := r.URL.Query()
	return model.AdminUserListQuery{
		Page:       queryInt(values.Get("page"), 1),
		PageSize:   queryInt(values.Get("pageSize"), 15),
		Search:     values.Get("search"),
		Account:    values.Get("account"),
		Credential: values.Get("credential"),
		Health:     values.Get("health"),
	}
}

func queryInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func (s *Server) handleAdminUserActions(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) == 5 && parts[4] == "detail" && r.Method == http.MethodGet {
		detail, err := s.manager.AdminUserDetail(parts[3])
		if err != nil {
			writePublicOperationError(w, http.StatusNotFound, "load admin user detail", "用户不存在或详情暂时不可用。", err)
			return
		}
		detail.Sessions, err = s.sessions.List(parts[3], "")
		if err != nil {
			writePublicOperationError(w, http.StatusInternalServerError, "list admin user sessions", "暂时无法读取该用户的登录会话。", err)
			return
		}
		writeJSON(w, http.StatusOK, detail)
		return
	}
	if len(parts) == 5 && parts[4] == "reset-password" && r.Method == http.MethodPost {
		target, _ := s.manager.User(parts[3])
		targetLabel := target.Username
		if targetLabel == "" {
			targetLabel = parts[3]
		}
		temporaryPassword, err := s.manager.ResetUserPassword(parts[3])
		if err != nil {
			s.recordAdminAudit(admin, "user.password_reset", "user", parts[3], targetLabel, "failure")
			writePublicOperationError(w, http.StatusBadRequest, "reset admin user password", "重置用户密码失败，请稍后重试。", err)
			return
		}
		if err := s.sessions.DeleteUser(parts[3]); err != nil {
			s.recordAdminAudit(admin, "user.password_reset", "user", parts[3], targetLabel, "failure")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "密码已重置，但撤销旧登录状态失败"})
			return
		}
		s.recordAdminAudit(admin, "user.password_reset", "user", parts[3], targetLabel, "success")
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, model.TemporaryPasswordResponse{
			TemporaryPassword: temporaryPassword,
		})
		return
	}
	if len(parts) == 5 && parts[4] == "refresh" && r.Method == http.MethodPost {
		var err error
		if s.yybClient != nil && s.moceleClient != nil {
			_, err = s.manager.RefreshWithYYB(parts[3], true, s.yybClient, s.moceleClient)
		} else {
			_, err = s.manager.Refresh(parts[3], true)
		}
		if err != nil {
			s.recordAdminAudit(admin, "user.refresh", "user", parts[3], parts[3], "failure")
			writePublicOperationError(w, http.StatusBadRequest, "refresh admin user", "刷新用户设备失败，请查看诊断记录。", err)
			return
		}
		s.recordAdminAudit(admin, "user.refresh", "user", parts[3], parts[3], "success")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if len(parts) != 4 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	userID := parts[3]
	target, _ := s.manager.User(userID)
	targetLabel := target.Username
	if targetLabel == "" {
		targetLabel = userID
	}
	switch r.Method {
	case http.MethodPatch:
		var req model.UserUpdateRequest
		if !decodeJSON(w, r, adminBodyLimit, &req) {
			return
		}
		user, err := s.manager.UpdateUser(userID, req)
		if err != nil {
			s.recordAdminAudit(admin, adminUserUpdateAction(req), "user", userID, targetLabel, "failure")
			writePublicOperationError(w, http.StatusBadRequest, "update admin user", "更新用户失败，请检查填写信息后重试。", err)
			return
		}
		if req.Role != nil || req.Enabled != nil {
			if err := s.sessions.DeleteUser(userID); err != nil {
				log.Printf("revoke sessions for user %s: %v", userID, err)
				s.recordAdminAudit(admin, adminUserUpdateAction(req), "user", userID, targetLabel, "failure")
				clearSessionCookie(w, r)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "用户已更新，但撤销旧登录状态失败"})
				return
			}
		}
		if userID == admin.ID {
			clearSessionCookie(w, r)
		}
		s.recordAdminAudit(admin, adminUserUpdateAction(req), "user", userID, user.Username, "success")
		writeJSON(w, http.StatusOK, user)
	case http.MethodDelete:
		if err := s.manager.DeleteUser(userID); err != nil {
			s.recordAdminAudit(admin, "user.delete", "user", userID, targetLabel, "failure")
			writePublicOperationError(w, http.StatusBadRequest, "delete admin user", "删除用户失败，请稍后重试。", err)
			return
		}
		if err := s.sessions.DeleteUser(userID); err != nil {
			log.Printf("revoke sessions for deleted user %s: %v", userID, err)
			s.recordAdminAudit(admin, "user.delete", "user", userID, targetLabel, "failure")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "用户已删除，但清理登录状态失败"})
			return
		}
		s.recordAdminAudit(admin, "user.delete", "user", userID, targetLabel, "success")
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	stats, err := s.manager.AdminStatsResult()
	if err != nil {
		s.setAdminDegraded("运营统计查询失败")
		logStructuredError("load_admin_stats", "", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "运营统计暂时不可用，服务健康已降级"})
		return
	}
	s.setAdminDegraded("")
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleAdminHealth(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	result := model.AdminHealth{
		CheckedAt: time.Now(),
		Charge: model.ServiceHealth{
			State: model.HealthHealthy, Message: "服务正常",
		},
		Database: model.ServiceHealth{
			State: model.HealthHealthy, Message: "存储正常",
		},
		YYB: model.ServiceHealth{
			State: model.HealthUnavailable, Message: "扫码服务未配置",
		},
	}
	if reason := s.adminDegradation(); reason != "" {
		result.Charge = model.ServiceHealth{
			State: model.HealthDegraded, Message: reason,
			RecoveryAdvice: "检查结构化错误日志和数据库状态，恢复后重新加载运营统计。",
		}
	}
	if err := s.manager.Ping(ctx); err != nil {
		result.Database = model.ServiceHealth{
			State: model.HealthUnavailable, Message: "存储暂不可用",
		}
	}
	if s.yybClient != nil {
		if err := s.yybClient.Health(ctx); err != nil {
			result.YYB = model.ServiceHealth{
				State: model.HealthDegraded, Message: "扫码服务连接异常",
				RecoveryAdvice: "确认扫码服务进程和网络可用后重新检查；仍异常时查看服务日志。",
			}
		} else {
			result.YYB = model.ServiceHealth{
				State: model.HealthHealthy, Message: "扫码服务正常",
			}
		}
	}
	for service, health := range map[string]model.ServiceHealth{
		"charge": result.Charge, "database": result.Database, "yyb": result.YYB,
	} {
		enriched, err := s.manager.RecordHealthCheck(service, health)
		if err != nil {
			logStructuredError("record_service_health", service, err)
			result.Database = model.ServiceHealth{
				State:          model.HealthDegraded,
				Message:        "健康历史暂时无法写入",
				RecoveryAdvice: "检查数据库写入权限和磁盘空间后重新检查。",
			}
			continue
		}
		switch service {
		case "charge":
			result.Charge = enriched
		case "database":
			result.Database = enriched
		case "yyb":
			result.YYB = enriched
		}
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleAdminSettings(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.manager.Settings())
	case http.MethodPut:
		var req model.RegistrationSettings
		if !decodeJSON(w, r, adminBodyLimit, &req) {
			return
		}
		if err := s.manager.UpdateSettings(req); err != nil {
			s.recordAdminAudit(admin, "settings.update", "settings", "registration", "注册与保留策略", "failure")
			writePublicOperationError(w, http.StatusBadRequest, "update settings", "保存系统策略失败，请检查设置后重试。", err)
			return
		}
		s.recordAdminAudit(admin, "settings.update", "settings", "registration", "注册与保留策略", "success")
		writeJSON(w, http.StatusOK, s.manager.Settings())
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleAdminInvites(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		values := r.URL.Query()
		writeJSON(w, http.StatusOK, s.manager.ListInviteCodesPage(
			queryInt(values.Get("page"), 1),
			queryInt(values.Get("pageSize"), 20),
		))
	case http.MethodPost:
		var req struct {
			Code      string     `json:"code,omitempty"`
			ExpiresAt *time.Time `json:"expiresAt,omitempty"`
		}
		if !decodeJSON(w, r, adminBodyLimit, &req) {
			return
		}
		invite, err := s.manager.CreateInvite(req.Code, req.ExpiresAt)
		if err != nil {
			s.recordAdminAudit(admin, "invite.create", "invite", "", "邀请码", "failure")
			writePublicOperationError(w, http.StatusBadRequest, "create invite", "创建邀请码失败，请稍后重试。", err)
			return
		}
		s.recordAdminAudit(admin, "invite.create", "invite", invite.ID, "邀请码", "success")
		writeJSON(w, http.StatusCreated, invite)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleAdminInviteActions(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodDelete {
		methodNotAllowed(w)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/admin/invites/")
	if id == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if err := s.manager.DeleteInvite(id); err != nil {
		s.recordAdminAudit(admin, "invite.delete", "invite", id, "邀请码", "failure")
		writePublicOperationError(w, http.StatusNotFound, "delete invite", "邀请码不存在或暂时无法删除，请稍后重试。", err)
		return
	}
	s.recordAdminAudit(admin, "invite.delete", "invite", id, "邀请码", "success")
	w.WriteHeader(http.StatusNoContent)
}
