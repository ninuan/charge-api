package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"charge-dashboard/internal/model"
	appruntime "charge-dashboard/internal/runtime"
)

func (s *Server) handleSharedPiles(w http.ResponseWriter, r *http.Request) {
	_, ok := s.requireDashboardUser(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := 30
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}
	items, err := s.manager.SearchSharedPiles(query, limit)
	if err != nil {
		writePublicOperationError(w, http.StatusInternalServerError, "search shared piles", "暂时无法搜索公共充电桩库，请稍后重试。", err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleUserQuota(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireDashboardUser(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	quota, err := s.manager.UserRefreshQuota(user.ID)
	if err != nil {
		writePublicOperationError(w, http.StatusInternalServerError, "load user quota", "暂时无法读取刷新额度，请稍后重试。", err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, quota)
}

func (s *Server) handleAdminPublicPool(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		items, err := s.manager.ListPublicPoolCookies()
		if err != nil {
			writePublicOperationError(w, http.StatusInternalServerError, "list public pool cookies", "暂时无法读取公共 Cookie 池。", err)
			return
		}
		writeJSON(w, http.StatusOK, items)
	case http.MethodPost:
		var req model.PublicPoolCookieCreateRequest
		if !decodeJSON(w, r, cookieBodyLimit, &req) {
			return
		}
		item, err := s.manager.CreatePublicPoolCookie(req)
		if err != nil {
			s.recordAdminAuditMessage(admin, "public_pool.create", "public_pool", "pool", strings.TrimSpace(req.Label), "failure", err.Error())
			if errors.Is(err, appruntime.ErrPublicPoolCookieDuplicate) {
				writeCodedError(w, http.StatusConflict, "PUBLIC_POOL_COOKIE_DUPLICATE", "公共池中已存在相同的上游账号凭据，请勿重复添加。")
				return
			}
			writeCodedError(w, http.StatusBadRequest, "PUBLIC_POOL_INVALID", err.Error())
			return
		}
		s.recordAdminAuditMessage(admin, "public_pool.create", "public_pool", item.ID, item.Label, "success", "新增公共池 Cookie")
		writeJSON(w, http.StatusCreated, item)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleAdminPublicPoolActions(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	trimmed := strings.TrimPrefix(r.URL.Path, "/api/admin/public-pool/")
	if strings.HasSuffix(trimmed, "/test") {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		id := strings.TrimSuffix(trimmed, "/test")
		if !validAPIResourceID(id) || strings.Contains(id, "/") {
			writeCodedError(w, http.StatusNotFound, "PUBLIC_POOL_NOT_FOUND", "未找到对应的公共 Cookie")
			return
		}
		item, err := s.manager.TestPublicPoolCookie(id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeCodedError(w, http.StatusNotFound, "PUBLIC_POOL_NOT_FOUND", "未找到对应的公共 Cookie")
				return
			}
			logStructuredError("test_public_pool_cookie", id, err)
			writeCodedError(w, http.StatusBadRequest, "PUBLIC_POOL_TEST_FAILED", "测试公共 Cookie 失败，请检查网络或确认公共桩库中至少有 1 台充电桩。")
			return
		}
		s.recordAdminAuditMessage(admin, "public_pool.test", "public_pool", item.ID, item.Label, "success", "实测结果："+item.Status)
		writeJSON(w, http.StatusOK, item)
		return
	}
	id := trimmed
	if !validAPIResourceID(id) || strings.Contains(id, "/") {
		writeCodedError(w, http.StatusNotFound, "PUBLIC_POOL_NOT_FOUND", "未找到对应的公共 Cookie")
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var req model.PublicPoolCookieUpdateRequest
		if !decodeJSON(w, r, cookieBodyLimit, &req) {
			return
		}
		item, err := s.manager.UpdatePublicPoolCookie(id, req)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeCodedError(w, http.StatusNotFound, "PUBLIC_POOL_NOT_FOUND", "未找到对应的公共 Cookie")
				return
			}
			s.recordAdminAuditMessage(admin, "public_pool.update", "public_pool", id, id, "failure", err.Error())
			if errors.Is(err, appruntime.ErrPublicPoolCookieDuplicate) {
				writeCodedError(w, http.StatusConflict, "PUBLIC_POOL_COOKIE_DUPLICATE", "公共池中已存在相同的上游账号凭据，请勿重复添加。")
				return
			}
			writeCodedError(w, http.StatusBadRequest, "PUBLIC_POOL_INVALID", err.Error())
			return
		}
		s.recordAdminAuditMessage(admin, "public_pool.update", "public_pool", item.ID, item.Label, "success", "更新公共池 Cookie")
		writeJSON(w, http.StatusOK, item)
	case http.MethodDelete:
		if err := s.manager.DeletePublicPoolCookie(id); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeCodedError(w, http.StatusNotFound, "PUBLIC_POOL_NOT_FOUND", "未找到对应的公共 Cookie")
				return
			}
			writePublicOperationError(w, http.StatusInternalServerError, "delete public pool cookie", "删除公共 Cookie 失败。", err)
			return
		}
		s.recordAdminAuditMessage(admin, "public_pool.delete", "public_pool", id, id, "success", "删除公共池 Cookie")
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleAdminSharedPiles(w http.ResponseWriter, r *http.Request) {
	_, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}
	items, err := s.manager.SearchSharedPiles(query, limit)
	if err != nil {
		writePublicOperationError(w, http.StatusInternalServerError, "list admin shared piles", "暂时无法读取公共桩库。", err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleAdminSharedPileActions(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	trimmed := strings.TrimPrefix(r.URL.Path, "/api/admin/shared-piles/")
	if strings.HasSuffix(trimmed, "/refresh") {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		deviceID := strings.TrimSuffix(trimmed, "/refresh")
		if !deviceIDPattern.MatchString(deviceID) {
			writeCodedError(w, http.StatusNotFound, "SHARED_PILE_NOT_FOUND", "未找到对应的公共充电桩")
			return
		}
		item, err := s.manager.AdminRefreshSharedPile(deviceID)
		if err != nil {
			s.recordAdminAuditMessage(admin, "shared_pile.refresh", "shared_pile", deviceID, deviceID, "failure", err.Error())
			if errors.Is(err, sql.ErrNoRows) {
				writeCodedError(w, http.StatusNotFound, "SHARED_PILE_NOT_FOUND", "未找到对应的公共充电桩")
				return
			}
			logStructuredError("refresh_shared_pile", deviceID, err)
			writeCodedError(w, http.StatusBadRequest, "SHARED_PILE_REFRESH_FAILED", "刷新公共桩失败，请确认公共 Cookie 池有可用凭据。")
			return
		}
		s.recordAdminAuditMessage(admin, "shared_pile.refresh", "shared_pile", item.DeviceID, item.Name, "success", "公共池刷新桩状态")
		writeJSON(w, http.StatusOK, item)
		return
	}
	deviceID := trimmed
	if !deviceIDPattern.MatchString(deviceID) {
		writeCodedError(w, http.StatusNotFound, "SHARED_PILE_NOT_FOUND", "未找到对应的公共充电桩")
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var req struct {
			Name    string `json:"name"`
			Address string `json:"address"`
		}
		if !decodeJSON(w, r, pileBodyLimit, &req) {
			return
		}
		item, err := s.manager.AdminUpdateSharedPile(deviceID, req.Name, req.Address)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeCodedError(w, http.StatusNotFound, "SHARED_PILE_NOT_FOUND", "未找到对应的公共充电桩")
				return
			}
			writeCodedError(w, http.StatusBadRequest, "SHARED_PILE_INVALID", err.Error())
			return
		}
		s.recordAdminAuditMessage(admin, "shared_pile.update", "shared_pile", item.DeviceID, item.Name, "success", "更新公共桩名称/位置")
		writeJSON(w, http.StatusOK, item)
	case http.MethodDelete:
		if err := s.manager.AdminDeleteSharedPile(deviceID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeCodedError(w, http.StatusNotFound, "SHARED_PILE_NOT_FOUND", "未找到对应的公共充电桩")
				return
			}
			writePublicOperationError(w, http.StatusInternalServerError, "delete shared pile", "删除公共桩失败。", err)
			return
		}
		s.recordAdminAuditMessage(admin, "shared_pile.delete", "shared_pile", deviceID, deviceID, "success", "从公共桩库删除")
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w)
	}
}
