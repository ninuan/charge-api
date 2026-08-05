package api

import (
	"errors"
	"net/http"
	"strconv"

	"charge-dashboard/internal/model"
	appruntime "charge-dashboard/internal/runtime"
)

func (s *Server) handlePileHistory(
	w http.ResponseWriter,
	r *http.Request,
	user model.CurrentUser,
	parts []string,
) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	deviceID := parts[2]
	if !deviceIDPattern.MatchString(deviceID) {
		writeCodedError(w, http.StatusBadRequest, "DEVICE_ID_INVALID", "设备 ID 格式无效")
		return
	}
	rangeName := r.URL.Query().Get("range")
	timezone := r.URL.Query().Get("timezone")
	w.Header().Set("Cache-Control", "private, no-store")

	if len(parts) == 4 {
		result, err := s.manager.DeviceHistory(user.ID, deviceID, rangeName, timezone)
		if err != nil {
			s.writeHistoryError(w, "load_device_history", deviceID, err)
			return
		}
		s.setHealthDegraded("history", "")
		writeJSON(w, http.StatusOK, result)
		return
	}

	portID, err := strconv.Atoi(parts[4])
	if err != nil || portID <= 0 || portID > 100 {
		writeCodedError(w, http.StatusBadRequest, "PORT_ID_INVALID", "充电口编号格式无效")
		return
	}
	result, err := s.manager.PortHistory(user.ID, deviceID, portID, rangeName, timezone)
	if err != nil {
		s.writeHistoryError(w, "load_port_history", deviceID, err)
		return
	}
	s.setHealthDegraded("history", "")
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) writeHistoryError(w http.ResponseWriter, operation, deviceID string, err error) {
	switch {
	case errors.Is(err, appruntime.ErrHistoryQueryInvalid):
		writeCodedError(w, http.StatusBadRequest, "HISTORY_QUERY_INVALID", "历史范围或时区参数无效")
	case errors.Is(err, appruntime.ErrHistoryNotFound):
		writeCodedError(w, http.StatusNotFound, "HISTORY_NOT_FOUND", "未找到对应的历史记录")
	case errors.Is(err, appruntime.ErrHistoryTooLarge):
		writeCodedError(w, http.StatusUnprocessableEntity, "HISTORY_RANGE_TOO_LARGE", "该范围内历史变化过多，请缩短查询范围")
	default:
		s.setHealthDegraded("history", "端口历史查询失败")
		logStructuredError(operation, deviceSuffixForLog(deviceID), err)
		writeCodedError(w, http.StatusServiceUnavailable, "HISTORY_UNAVAILABLE", "历史数据暂时不可用，请稍后重试")
	}
}

func deviceSuffixForLog(deviceID string) string {
	if len(deviceID) <= 4 {
		return deviceID
	}
	return deviceID[len(deviceID)-4:]
}
