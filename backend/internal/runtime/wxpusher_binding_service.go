package runtime

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"charge-dashboard/internal/model"
	"charge-dashboard/internal/persistence"
	"charge-dashboard/internal/wxpusher"
)

const (
	wxPusherBindValidity    = 10 * time.Minute
	wxPusherPollInterval    = 10 * time.Second
	wxPusherCreateMinuteMax = 1
	wxPusherCreateDayMax    = 10
	wxPusherDeliveryNotice  = "消息由 WxPusher 转发，绑定成功不代表微信一定已展示。"
)

var (
	ErrWxPusherAlreadyBound       = errors.New("wxpusher already bound")
	ErrWxPusherBindSessionActive  = errors.New("wxpusher bind session active")
	ErrWxPusherBindSessionMissing = errors.New("wxpusher bind session missing")
	ErrWxPusherUIDConflict        = errors.New("wxpusher uid conflict")
	ErrWxPusherRateLimited        = errors.New("wxpusher bind rate limited")
	ErrWxPusherNotConfigured      = errors.New("wxpusher not configured")
)

type WxPusherBindingClient interface {
	CreateQRCode(context.Context, string, time.Duration) (wxpusher.QRCode, error)
	QueryScanUID(context.Context, string) (wxpusher.ScanResult, error)
}

type WxPusherRateLimitError struct {
	RetryAfter time.Duration
}

func (e WxPusherRateLimitError) Error() string { return ErrWxPusherRateLimited.Error() }
func (e WxPusherRateLimitError) Unwrap() error { return ErrWxPusherRateLimited }

func (m *Manager) WxPusherChannelState(userID string, configured bool) (model.WxPusherChannelState, error) {
	if _, ok := m.User(userID); !ok {
		return model.WxPusherChannelState{}, fmt.Errorf("user not found or disabled")
	}
	binding, found, err := m.repository.LoadWxPusherBinding(userID)
	if err != nil {
		return model.WxPusherChannelState{}, err
	}
	state := model.WxPusherChannelState{
		Configured:         configured,
		EventTypes:         wxPusherEventTypeList(model.WxPusherDefaultEventTypes),
		DeliveryDisclaimer: wxPusherDeliveryNotice,
	}
	if !found {
		return state, nil
	}
	boundAt := binding.BoundAt
	state.Bound = true
	state.Enabled = binding.Enabled
	state.EventTypes = wxPusherEventTypeList(binding.EventTypes)
	state.MaskedUID = maskWxPusherUID(binding.UID)
	state.BoundAt = &boundAt
	state.LastTestAt = binding.LastTestAt
	return state, nil
}

func (m *Manager) CreateWxPusherBindSession(ctx context.Context, userID string, client WxPusherBindingClient) (model.WxPusherBindSessionView, error) {
	if client == nil {
		return model.WxPusherBindSessionView{}, ErrWxPusherNotConfigured
	}
	if _, ok := m.User(userID); !ok {
		return model.WxPusherBindSessionView{}, fmt.Errorf("user not found or disabled")
	}
	now := m.reminderSchedulerNow().UTC().Truncate(time.Second)
	m.wxPusherMu.Lock()
	if err := m.repository.CompleteExpiredWxPusherBindSessions(userID, now); err != nil {
		m.wxPusherMu.Unlock()
		return model.WxPusherBindSessionView{}, err
	}
	if _, found, err := m.repository.LoadWxPusherBinding(userID); err != nil {
		m.wxPusherMu.Unlock()
		return model.WxPusherBindSessionView{}, err
	} else if found {
		m.wxPusherMu.Unlock()
		return model.WxPusherBindSessionView{}, ErrWxPusherAlreadyBound
	}
	if _, found, err := m.repository.LoadActiveWxPusherBindSession(userID); err != nil {
		m.wxPusherMu.Unlock()
		return model.WxPusherBindSessionView{}, err
	} else if found {
		m.wxPusherMu.Unlock()
		return model.WxPusherBindSessionView{}, ErrWxPusherBindSessionActive
	}
	if err := m.consumeWxPusherCreateAttemptLocked(userID, now); err != nil {
		m.wxPusherMu.Unlock()
		return model.WxPusherBindSessionView{}, err
	}
	count, err := m.repository.CountWxPusherBindSessionsSince(userID, now.Add(-24*time.Hour))
	if err != nil {
		m.wxPusherMu.Unlock()
		return model.WxPusherBindSessionView{}, err
	}
	if count >= wxPusherCreateDayMax {
		m.wxPusherMu.Unlock()
		return model.WxPusherBindSessionView{}, WxPusherRateLimitError{RetryAfter: time.Hour}
	}
	sessionID, err := secureWxPusherSessionID()
	if err != nil {
		m.wxPusherMu.Unlock()
		return model.WxPusherBindSessionView{}, fmt.Errorf("generate wxpusher bind session: %w", err)
	}
	m.wxPusherMu.Unlock()

	qr, err := client.CreateQRCode(ctx, sessionID, wxPusherBindValidity)
	if err != nil {
		return model.WxPusherBindSessionView{}, err
	}
	expiresAt := qr.ExpiresAt.UTC()
	if limit := now.Add(wxPusherBindValidity); expiresAt.After(limit) {
		expiresAt = limit
	}
	if !expiresAt.After(now) {
		return model.WxPusherBindSessionView{}, fmt.Errorf("wxpusher qr code already expired")
	}
	session := model.WxPusherBindSession{
		ID: sessionID, UserID: userID, ProviderCode: qr.Code, QRURL: qr.URL,
		ExpiresAt: expiresAt, NextPollAt: now.Add(wxPusherPollInterval), CreatedAt: now,
	}
	if err := m.repository.SaveWxPusherBindSession(session); err != nil {
		return model.WxPusherBindSessionView{}, err
	}
	return wxPusherSessionView(session, model.WxPusherBindWaiting, "请使用微信扫码完成绑定。", ""), nil
}

func (m *Manager) PollWxPusherBindSession(ctx context.Context, userID, sessionID string, client WxPusherBindingClient) (model.WxPusherBindSessionView, error) {
	if client == nil {
		return model.WxPusherBindSessionView{}, ErrWxPusherNotConfigured
	}
	now := m.reminderSchedulerNow().UTC().Truncate(time.Second)
	m.wxPusherMu.Lock()
	session, found, err := m.repository.LoadWxPusherBindSession(userID, sessionID)
	if err != nil {
		m.wxPusherMu.Unlock()
		return model.WxPusherBindSessionView{}, err
	}
	if !found {
		m.wxPusherMu.Unlock()
		return model.WxPusherBindSessionView{}, ErrWxPusherBindSessionMissing
	}
	if session.CompletedAt != nil {
		m.wxPusherMu.Unlock()
		if _, bound, loadErr := m.repository.LoadWxPusherBinding(userID); loadErr != nil {
			return model.WxPusherBindSessionView{}, loadErr
		} else if bound {
			return wxPusherSessionView(session, model.WxPusherBindBound, "微信提醒已绑定。", ""), nil
		}
		if !session.ExpiresAt.After(now) {
			return wxPusherSessionView(session, model.WxPusherBindExpired, "二维码已过期，请重新获取。", ""), nil
		}
		return wxPusherSessionView(session, model.WxPusherBindFailed, "绑定未完成，请重新获取二维码。", "invalid_response"), nil
	}
	if !session.ExpiresAt.After(now) {
		_ = m.repository.CompleteWxPusherBindSession(userID, sessionID, now)
		completed := now
		session.CompletedAt = &completed
		m.wxPusherMu.Unlock()
		return wxPusherSessionView(session, model.WxPusherBindExpired, "二维码已过期，请重新获取。", ""), nil
	}
	if session.NextPollAt.After(now) {
		m.wxPusherMu.Unlock()
		return wxPusherSessionView(session, model.WxPusherBindWaiting, "等待扫码确认。", ""), nil
	}
	// Persist the next allowed provider request before performing I/O. A second
	// request, including one after a process restart, must observe this value.
	session.NextPollAt = now.Add(wxPusherPollInterval)
	if err := m.repository.SaveWxPusherBindSession(session); err != nil {
		m.wxPusherMu.Unlock()
		return model.WxPusherBindSessionView{}, err
	}
	m.wxPusherMu.Unlock()

	result, err := client.QueryScanUID(ctx, session.ProviderCode)
	if err != nil {
		if wxpusher.IsRetryable(err) {
			return wxPusherSessionView(session, model.WxPusherBindWaiting, "暂时无法确认扫码结果，将自动重试。", wxPusherPublicErrorCode(err)), nil
		}
		_ = m.repository.CompleteWxPusherBindSession(userID, sessionID, now)
		completed := now
		session.CompletedAt = &completed
		return wxPusherSessionView(session, model.WxPusherBindFailed, "绑定未完成，请重新获取二维码。", wxPusherPublicErrorCode(err)), nil
	}
	if !result.Scanned || strings.TrimSpace(result.UID) == "" {
		return wxPusherSessionView(session, model.WxPusherBindWaiting, "等待扫码确认。", ""), nil
	}
	if err := m.repository.BindWxPusherUID(userID, sessionID, result.UID, now); err != nil {
		switch {
		case errors.Is(err, persistence.ErrWxPusherUIDInUse):
			_ = m.repository.CompleteWxPusherBindSession(userID, sessionID, now)
			completed := now
			session.CompletedAt = &completed
			return wxPusherSessionView(session, model.WxPusherBindFailed, "这个微信接收账号已绑定其他账户。", "invalid_uid"), nil
		case errors.Is(err, persistence.ErrWxPusherBindingExists):
			return model.WxPusherBindSessionView{}, ErrWxPusherAlreadyBound
		default:
			return model.WxPusherBindSessionView{}, err
		}
	}
	completed := now
	session.CompletedAt = &completed
	return wxPusherSessionView(session, model.WxPusherBindBound, "微信提醒已绑定。", ""), nil
}

func (m *Manager) DeleteWxPusherChannel(userID string) error {
	if _, ok := m.User(userID); !ok {
		return fmt.Errorf("user not found or disabled")
	}
	_, err := m.repository.DeleteWxPusherChannel(userID, m.reminderSchedulerNow().UTC().Truncate(time.Second))
	return err
}

func (m *Manager) consumeWxPusherCreateAttemptLocked(userID string, now time.Time) error {
	cutoff := now.Add(-24 * time.Hour)
	attempts := m.wxPusherAttempts[userID][:0]
	for _, attempt := range m.wxPusherAttempts[userID] {
		if attempt.After(cutoff) {
			attempts = append(attempts, attempt)
		}
	}
	if len(attempts) >= wxPusherCreateDayMax {
		m.wxPusherAttempts[userID] = attempts
		return WxPusherRateLimitError{RetryAfter: time.Hour}
	}
	if len(attempts) >= wxPusherCreateMinuteMax {
		last := attempts[len(attempts)-1]
		if remaining := time.Minute - now.Sub(last); remaining > 0 {
			m.wxPusherAttempts[userID] = attempts
			return WxPusherRateLimitError{RetryAfter: remaining}
		}
	}
	m.wxPusherAttempts[userID] = append(attempts, now)
	return nil
}

func secureWxPusherSessionID() (string, error) {
	raw := make([]byte, 18)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "wxp_" + base64.RawURLEncoding.EncodeToString(raw), nil
}

func wxPusherSessionView(session model.WxPusherBindSession, status model.WxPusherBindSessionStatus, message, errorCode string) model.WxPusherBindSessionView {
	view := model.WxPusherBindSessionView{
		ID: session.ID, Status: status, ExpiresAt: session.ExpiresAt,
		NextPollAt: session.NextPollAt, CompletedAt: session.CompletedAt,
		Message: message, ErrorCode: errorCode,
	}
	if status == model.WxPusherBindWaiting {
		view.QRURL = session.QRURL
	}
	return view
}

func wxPusherEventTypeList(mask model.WxPusherEventTypes) []model.WxPusherEventType {
	items := make([]model.WxPusherEventType, 0, 4)
	if mask&model.WxPusherEventPileAvailable != 0 {
		items = append(items, model.WxPusherEventTypePileAvailable)
	}
	if mask&model.WxPusherEventCredentialExpired != 0 {
		items = append(items, model.WxPusherEventTypeCredentialExpired)
	}
	if mask&model.WxPusherEventPileOffline != 0 {
		items = append(items, model.WxPusherEventTypePileOffline)
	}
	if mask&model.WxPusherEventPileRecovered != 0 {
		items = append(items, model.WxPusherEventTypePileRecovered)
	}
	return items
}

func maskWxPusherUID(uid string) string {
	runes := []rune(strings.TrimSpace(uid))
	if len(runes) <= 4 {
		return "••••"
	}
	return "••••" + string(runes[len(runes)-4:])
}

func wxPusherPublicErrorCode(err error) string {
	switch wxpusher.CodeOf(err) {
	case wxpusher.ErrorInvalidToken:
		return "invalid_token"
	case wxpusher.ErrorInvalidUID:
		return "invalid_uid"
	case wxpusher.ErrorRecipientRejected:
		return "recipient_rejected"
	case wxpusher.ErrorRateLimited:
		return "rate_limited"
	case wxpusher.ErrorProviderTimeout:
		return "timeout"
	case wxpusher.ErrorInvalidResponse:
		return "invalid_response"
	default:
		return "provider_unavailable"
	}
}
