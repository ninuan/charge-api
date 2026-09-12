package api

import (
	"charge-dashboard/internal/model"
	"net/http"
)

// Keep a tombstone for older clients; sparse observations cannot support usage analytics.
func (s *Server) handlePileHistory(w http.ResponseWriter, r *http.Request, _ model.CurrentUser, _ []string) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeCodedError(w, http.StatusGone, "HISTORY_REMOVED", "使用历史已下线，请查看充电桩最近状态。")
}
