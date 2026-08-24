package api

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	appruntime "charge-dashboard/internal/runtime"
	"charge-dashboard/internal/wxpusher"
)

func (s *Server) handleWxPusherChannel(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireDashboardUser(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	switch r.Method {
	case http.MethodGet:
		state, err := s.manager.WxPusherChannelState(user.ID, s.wxPusherClient != nil)
		if err != nil {
			s.writeWxPusherError(w, "load_wxpusher_channel", err)
			return
		}
		s.setHealthDegraded("wxpusher", "")
		writeJSON(w, http.StatusOK, state)
	case http.MethodDelete:
		if err := s.manager.DeleteWxPusherChannel(user.ID); err != nil {
			s.writeWxPusherError(w, "delete_wxpusher_channel", err)
			return
		}
		s.setHealthDegraded("wxpusher", "")
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleWxPusherBindSessions(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireDashboardUser(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	session, err := s.manager.CreateWxPusherBindSession(r.Context(), user.ID, s.wxPusherClient)
	if err != nil {
		s.writeWxPusherError(w, "create_wxpusher_bind_session", err)
		return
	}
	s.setHealthDegraded("wxpusher", "")
	writeJSON(w, http.StatusCreated, session)
}

func (s *Server) handleWxPusherBindSession(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireDashboardUser(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	sessionID := strings.TrimPrefix(r.URL.Path, "/api/notification-channels/wxpusher/bind-sessions/")
	if !validAPIResourceID(sessionID) || strings.Contains(sessionID, "/") {
		writeCodedError(w, http.StatusNotFound, "WXPUSHER_BIND_SESSION_NOT_FOUND", "未找到微信绑定会话")
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	session, err := s.manager.PollWxPusherBindSession(r.Context(), user.ID, sessionID, s.wxPusherClient)
	if err != nil {
		s.writeWxPusherError(w, "poll_wxpusher_bind_session", err)
		return
	}
	s.setHealthDegraded("wxpusher", "")
	writeJSON(w, http.StatusOK, session)
}

func (s *Server) writeWxPusherError(w http.ResponseWriter, operation string, err error) {
	switch {
	case errors.Is(err, appruntime.ErrWxPusherNotConfigured):
		writeCodedError(w, http.StatusServiceUnavailable, "WXPUSHER_NOT_CONFIGURED", "管理员暂未启用微信提醒")
	case errors.Is(err, appruntime.ErrWxPusherAlreadyBound):
		writeCodedError(w, http.StatusConflict, "WXPUSHER_ALREADY_BOUND", "请先解除当前微信绑定")
	case errors.Is(err, appruntime.ErrWxPusherBindSessionActive):
		writeCodedError(w, http.StatusConflict, "WXPUSHER_BIND_SESSION_ACTIVE", "已有等待扫码的二维码，请稍后再试")
	case errors.Is(err, appruntime.ErrWxPusherBindSessionMissing):
		writeCodedError(w, http.StatusNotFound, "WXPUSHER_BIND_SESSION_NOT_FOUND", "未找到微信绑定会话")
	case errors.Is(err, appruntime.ErrWxPusherUIDConflict):
		writeCodedError(w, http.StatusConflict, "WXPUSHER_UID_CONFLICT", "这个微信接收账号已绑定其他账户")
	case errors.Is(err, appruntime.ErrWxPusherRateLimited):
		retryAfter := time.Minute
		var limited appruntime.WxPusherRateLimitError
		if errors.As(err, &limited) && limited.RetryAfter > 0 {
			retryAfter = limited.RetryAfter
		}
		w.Header().Set("Retry-After", strconv.Itoa(max(1, int(retryAfter.Seconds()))))
		writeCodedError(w, http.StatusTooManyRequests, "WXPUSHER_RATE_LIMITED", "获取二维码过于频繁，请稍后再试")
	default:
		providerCode := wxpusher.CodeOf(err)
		if providerCode != "" {
			s.setHealthDegraded("wxpusher", "微信提醒服务暂时异常")
			logStructuredError(operation, "wxpusher", err)
			writeCodedError(w, http.StatusServiceUnavailable, "WXPUSHER_UNAVAILABLE", "微信提醒服务暂时不可用，请稍后重试")
			return
		}
		s.setHealthDegraded("wxpusher", "微信提醒存储异常")
		log.Printf("operation=%s component=wxpusher error=%v", operation, err)
		writeCodedError(w, http.StatusServiceUnavailable, "WXPUSHER_UNAVAILABLE", "微信提醒暂时不可用，请稍后重试")
	}
}
