package api

import (
	"errors"
	"net/http"
	"strings"

	"charge-dashboard/internal/model"
	appruntime "charge-dashboard/internal/runtime"
)

func (s *Server) handlePiles(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireDashboardUser(w, r)
	if !ok {
		return
	}

	switch r.Method {
	case http.MethodGet:
		snapshot, err := s.manager.Snapshot(user.ID)
		if err != nil {
			writePublicOperationError(w, http.StatusInternalServerError, "load pile snapshot", "暂时无法加载充电桩信息，请稍后重试。", err)
			return
		}
		writeJSON(w, http.StatusOK, snapshot)
	case http.MethodPost:
		var req model.PileUpsertRequest
		if !decodeJSON(w, r, pileBodyLimit, &req) {
			return
		}
		req.ID = strings.TrimSpace(req.ID)
		req.Number = strings.TrimSpace(req.Number)
		if req.ID == "" && req.Number == "" {
			s.recordDashboardDiagnostic(user, "add_pile", "pile_identifier_required", "", http.StatusBadRequest)
			writeCodedError(w, http.StatusBadRequest, "PILE_IDENTIFIER_REQUIRED", "请输入桩号或设备长ID")
			return
		}
		if req.ID != "" && !deviceIDPattern.MatchString(req.ID) {
			s.recordDashboardDiagnostic(user, "add_pile", "pile_id_invalid", req.ID, http.StatusBadRequest)
			writeCodedError(w, http.StatusBadRequest, "PILE_ID_INVALID", "设备ID必须是 6-64 位数字")
			return
		}
		if req.Number != "" && !pileNumberPattern.MatchString(req.Number) {
			s.recordDashboardDiagnostic(user, "add_pile", "pile_number_invalid", "", http.StatusBadRequest)
			writeCodedError(w, http.StatusBadRequest, "PILE_NUMBER_INVALID", "桩号必须是 6-64 位数字")
			return
		}
		if len(req.Name) > 128 || len(req.Number) > 64 || len(req.Status) > 32 || len(req.Address) > 256 {
			s.recordDashboardDiagnostic(user, "add_pile", "pile_fields_invalid", req.ID, http.StatusBadRequest)
			writeCodedError(w, http.StatusBadRequest, "PILE_FIELDS_INVALID", "充电桩字段长度超出限制")
			return
		}
		if req.OpenNum < 0 || req.OpenNum > 20 {
			s.recordDashboardDiagnostic(user, "add_pile", "pile_port_count_invalid", req.ID, http.StatusBadRequest)
			writeCodedError(w, http.StatusBadRequest, "PILE_PORT_COUNT_INVALID", "充电口数量必须在 1-20 之间")
			return
		}
		var pile model.Pile
		var err error
		if s.yybClient != nil && s.moceleClient != nil {
			pile, err = s.manager.AddPileWithYYB(user.ID, req, s.yybClient, s.moceleClient)
		} else {
			pile, err = s.manager.AddPile(user.ID, req)
		}
		if err != nil {
			s.recordDashboardDiagnostic(user, "add_pile", "add_pile_failed", req.ID, appruntime.DiagnosticStatusCode(err))
			writeAddPileError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, pile)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) recordDashboardDiagnostic(user model.CurrentUser, operation, code, deviceID string, statusCode int) {
	s.manager.RecordOperationDiagnostic(user.ID, operation, code, deviceID, statusCode)
}

func writeAddPileError(w http.ResponseWriter, err error) {
	if errors.Is(err, appruntime.ErrYYBBindingRequired) {
		writeJSON(w, http.StatusConflict, map[string]string{
			"code":  "YYB_BINDING_REQUIRED",
			"error": "请先完成扫码登录绑定，再添加充电桩",
		})
		return
	}
	writePublicOperationError(w, http.StatusBadRequest, "add pile", "添加充电桩失败，请检查桩号后重试。", err)
}

func (s *Server) handlePileActions(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireDashboardUser(w, r)
	if !ok {
		return
	}

	if r.URL.Path == "/api/piles/order" {
		if r.Method != http.MethodPut {
			methodNotAllowed(w)
			return
		}
		var req struct {
			IDs []string `json:"ids"`
		}
		if !decodeJSON(w, r, pileBodyLimit, &req) {
			return
		}
		snapshot, err := s.manager.ReorderPiles(user.ID, req.IDs)
		if err != nil {
			writePublicOperationError(w, http.StatusBadRequest, "reorder piles", "调整充电桩顺序失败，请刷新后重试。", err)
			return
		}
		writeJSON(w, http.StatusOK, snapshot)
		return
	}

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if (len(parts) == 4 && parts[3] == "history") ||
		(len(parts) == 6 && parts[3] == "ports" && parts[5] == "history") {
		s.handlePileHistory(w, r, user, parts)
		return
	}
	if len(parts) != 3 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if !deviceIDPattern.MatchString(parts[2]) {
		s.recordDashboardDiagnostic(user, "add_pile", "pile_id_invalid", parts[2], http.StatusBadRequest)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid device id"})
		return
	}

	switch r.Method {
	case http.MethodDelete:
		if err := s.manager.DeletePile(user.ID, parts[2]); err != nil {
			s.recordDashboardDiagnostic(user, "add_pile", "pile_delete_failed", parts[2], appruntime.DiagnosticStatusCode(err))
			writePublicOperationError(w, http.StatusNotFound, "delete pile", "未找到对应充电桩或暂时无法删除，请稍后重试。", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPatch:
		var req struct {
			Name      string `json:"name"`
			Address   string `json:"address"`
			SortOrder int    `json:"sortOrder"`
		}
		if !decodeJSON(w, r, pileBodyLimit, &req) {
			return
		}
		pile, err := s.manager.UpdatePile(user.ID, parts[2], req.Name, req.Address, req.SortOrder)
		if err != nil {
			s.recordDashboardDiagnostic(user, "add_pile", "pile_update_failed", parts[2], appruntime.DiagnosticStatusCode(err))
			writePublicOperationError(w, http.StatusBadRequest, "update pile", "更新充电桩失败，请检查填写信息后重试。", err)
			return
		}
		writeJSON(w, http.StatusOK, pile)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireDashboardUser(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	var snapshot model.DashboardSnapshot
	var err error
	if s.consumeDevForceAuthExpired() {
		if s.yybClient == nil || s.moceleClient == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "本地鉴权失效测试需要先配置 YYB 扫码服务"})
			return
		}
		snapshot, err = s.manager.RecoverRefreshWithYYB(user.ID, s.yybClient, s.moceleClient)
	} else if s.yybClient != nil && s.moceleClient != nil {
		snapshot, err = s.manager.RefreshWithYYB(user.ID, false, s.yybClient, s.moceleClient)
	} else {
		snapshot, err = s.manager.Refresh(user.ID, false)
	}
	if err != nil {
		s.recordDashboardDiagnostic(user, "refresh", "refresh_failed", "", appruntime.DiagnosticStatusCode(err))
		writePublicOperationError(w, http.StatusInternalServerError, "refresh piles", "暂时无法刷新设备状态，请稍后重试。", err)
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}
