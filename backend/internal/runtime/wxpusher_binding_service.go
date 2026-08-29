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
	wxPusherTestDayMax      = 5
	wxPusherDeliveryNotice  = "消息发出后，请在 WxPusher App 或微信中确认是否收到。页面无法确认是否已经阅读。"
)

var (
	ErrWxPusherAlreadyBound        = errors.New("wxpusher already bound")
	ErrWxPusherBindSessionActive   = errors.New("wxpusher bind session active")
	ErrWxPusherBindSessionMissing  = errors.New("wxpusher bind session missing")
	ErrWxPusherUIDConflict         = errors.New("wxpusher uid conflict")
	ErrWxPusherRateLimited         = errors.New("wxpusher bind rate limited")
	ErrWxPusherNotConfigured       = errors.New("wxpusher not configured")
	ErrWxPusherNotBound            = errors.New("wxpusher not bound")
	ErrWxPusherChannelDisabled     = errors.New("wxpusher channel disabled")
	ErrWxPusherPreferenceInvalid   = errors.New("wxpusher preference invalid")
	ErrWxPusherTestDeliveryMissing = errors.New("wxpusher test delivery missing")
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
	latestDelivery, deliveryFound, err := m.repository.LoadLatestNotificationDelivery(userID, false)
	if err != nil {
		return model.WxPusherChannelState{}, err
	}
	if deliveryFound {
		summary := wxPusherDeliverySummary(latestDelivery)
		state.LastDelivery = &summary
	}
	latestTestDelivery, testDeliveryFound, err := m.repository.LoadLatestNotificationDelivery(userID, true)
	if err != nil {
		return model.WxPusherChannelState{}, err
	}
	if testDeliveryFound {
		summary := wxPusherDeliverySummary(latestTestDelivery)
		state.LastTestDelivery = &summary
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

func (m *Manager) UpdateWxPusherChannel(
	userID string,
	request model.WxPusherChannelUpdateRequest,
	configured bool,
) (model.WxPusherChannelState, error) {
	if !configured {
		return model.WxPusherChannelState{}, ErrWxPusherNotConfigured
	}
	if request.Enabled == nil && request.EventTypes == nil {
		return model.WxPusherChannelState{}, ErrWxPusherPreferenceInvalid
	}
	if _, ok := m.User(userID); !ok {
		return model.WxPusherChannelState{}, fmt.Errorf("user not found or disabled")
	}
	m.wxPusherMu.Lock()
	defer m.wxPusherMu.Unlock()
	binding, found, err := m.repository.LoadWxPusherBinding(userID)
	if err != nil {
		return model.WxPusherChannelState{}, err
	}
	if !found {
		return model.WxPusherChannelState{}, ErrWxPusherNotBound
	}
	if request.Enabled != nil {
		binding.Enabled = *request.Enabled
	}
	if request.EventTypes != nil {
		mask, err := wxPusherEventTypeMask(*request.EventTypes)
		if err != nil {
			return model.WxPusherChannelState{}, ErrWxPusherPreferenceInvalid
		}
		binding.EventTypes = mask
	}
	binding.UpdatedAt = m.reminderSchedulerNow().UTC().Truncate(time.Second)
	if err := m.repository.SaveWxPusherBinding(binding); err != nil {
		return model.WxPusherChannelState{}, err
	}
	return m.WxPusherChannelState(userID, configured)
}

func (m *Manager) CreateWxPusherTestDelivery(userID string, configured bool) (model.NotificationDeliverySummary, error) {
	if !configured {
		return model.NotificationDeliverySummary{}, ErrWxPusherNotConfigured
	}
	if _, ok := m.User(userID); !ok {
		return model.NotificationDeliverySummary{}, fmt.Errorf("user not found or disabled")
	}
	now := m.reminderSchedulerNow().UTC().Truncate(time.Second)
	m.wxPusherMu.Lock()
	binding, found, err := m.repository.LoadWxPusherBinding(userID)
	if err != nil {
		m.wxPusherMu.Unlock()
		return model.NotificationDeliverySummary{}, err
	}
	if !found {
		m.wxPusherMu.Unlock()
		return model.NotificationDeliverySummary{}, ErrWxPusherNotBound
	}
	if !binding.Enabled {
		m.wxPusherMu.Unlock()
		return model.NotificationDeliverySummary{}, ErrWxPusherChannelDisabled
	}
	if binding.LastTestAt != nil && now.Sub(*binding.LastTestAt) < time.Minute {
		retryAfter := time.Minute - now.Sub(*binding.LastTestAt)
		m.wxPusherMu.Unlock()
		return model.NotificationDeliverySummary{}, WxPusherRateLimitError{RetryAfter: retryAfter}
	}
	count, err := m.repository.CountTestNotificationDeliveriesSince(userID, now.Add(-24*time.Hour))
	if err != nil {
		m.wxPusherMu.Unlock()
		return model.NotificationDeliverySummary{}, err
	}
	if count >= wxPusherTestDayMax {
		m.wxPusherMu.Unlock()
		return model.NotificationDeliverySummary{}, WxPusherRateLimitError{RetryAfter: time.Hour}
	}
	deliveryID, err := secureWxPusherDeliveryID()
	if err != nil {
		m.wxPusherMu.Unlock()
		return model.NotificationDeliverySummary{}, fmt.Errorf("generate wxpusher test delivery: %w", err)
	}
	delivery := model.NotificationDelivery{
		ID: deliveryID, UserID: userID, Channel: "wxpusher",
		Status: model.NotificationDeliveryPending, IsTest: true,
		NextAttemptAt: &now, CreatedAt: now, UpdatedAt: now,
	}
	if err := m.repository.CreateWxPusherTestDelivery(delivery); err != nil {
		m.wxPusherMu.Unlock()
		if errors.Is(err, persistence.ErrWxPusherBindingNotEnabled) {
			return model.NotificationDeliverySummary{}, ErrWxPusherChannelDisabled
		}
		return model.NotificationDeliverySummary{}, err
	}
	m.wxPusherMu.Unlock()
	m.wakeNotificationDispatcher()
	return wxPusherDeliverySummary(delivery), nil
}

// RecheckWxPusherTestDelivery queries the provider record belonging to the
// current user's latest test delivery. It never creates or resends a message.
func (m *Manager) RecheckWxPusherTestDelivery(ctx context.Context, userID string) (model.NotificationDeliverySummary, error) {
	if _, ok := m.User(userID); !ok {
		return model.NotificationDeliverySummary{}, fmt.Errorf("user not found or disabled")
	}
	delivery, found, err := m.repository.LoadLatestNotificationDelivery(userID, true)
	if err != nil {
		return model.NotificationDeliverySummary{}, err
	}
	if !found {
		return model.NotificationDeliverySummary{}, ErrWxPusherTestDeliveryMissing
	}
	if (delivery.Status != model.NotificationDeliveryAccepted && delivery.Status != model.NotificationDeliveryUncertain) ||
		strings.TrimSpace(delivery.ProviderRecordID) == "" {
		return wxPusherDeliverySummary(delivery), nil
	}
	client, _ := m.notificationDispatcherConfig()
	if client == nil {
		return model.NotificationDeliverySummary{}, ErrWxPusherNotConfigured
	}

	requestCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	providerStatus, queryErr := client.QueryMessageStatus(requestCtx, delivery.ProviderRecordID)
	now := m.reminderSchedulerNow().UTC().Truncate(time.Second)

	// The background dispatcher may have completed the same delivery while the
	// provider request was in flight. Reload before saving so a manual check can
	// never move a newer state backwards.
	current, currentFound, err := m.repository.LoadLatestNotificationDelivery(userID, true)
	if err != nil {
		return model.NotificationDeliverySummary{}, err
	}
	if !currentFound {
		return model.NotificationDeliverySummary{}, ErrWxPusherTestDeliveryMissing
	}
	if current.ID != delivery.ID ||
		(current.Status != model.NotificationDeliveryAccepted && current.Status != model.NotificationDeliveryUncertain) {
		return wxPusherDeliverySummary(current), nil
	}

	if queryErr != nil {
		code := string(wxpusher.CodeOf(queryErr))
		if code == "" {
			code = "wxpusher_unknown"
		}
		current.LastErrorCode, current.LastErrorMessage, current.UpdatedAt = code, "", now
		if err := m.repository.SaveNotificationDelivery(current); err != nil {
			return model.NotificationDeliverySummary{}, err
		}
		return wxPusherDeliverySummary(current), nil
	}
	if providerStatus.Succeeded {
		current.Status = model.NotificationDeliveryProviderSucceeded
		current.NextAttemptAt, current.ClaimedAt = nil, nil
		current.LastErrorCode, current.LastErrorMessage = "", ""
		current.ProviderSucceededAt, current.UpdatedAt = timePointer(now), now
		if err := m.repository.SaveNotificationDelivery(current); err != nil {
			return model.NotificationDeliverySummary{}, err
		}
		if err := m.repository.RecordWxPusherProviderSuccess(userID, now); err != nil {
			return model.NotificationDeliverySummary{}, err
		}
		return wxPusherDeliverySummary(current), nil
	}
	if providerStatus.Failed {
		if err := m.finishDelivery(current, model.NotificationDeliveryFailed, "provider_delivery_failed", now); err != nil {
			return model.NotificationDeliverySummary{}, err
		}
		current.Status = model.NotificationDeliveryFailed
		current.LastErrorCode, current.UpdatedAt = "provider_delivery_failed", now
		return wxPusherDeliverySummary(current), nil
	}

	if current.Status == model.NotificationDeliveryAccepted {
		current.LastErrorCode, current.LastErrorMessage = "", ""
	}
	current.UpdatedAt = now
	if err := m.repository.SaveNotificationDelivery(current); err != nil {
		return model.NotificationDeliverySummary{}, err
	}
	return wxPusherDeliverySummary(current), nil
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

func secureWxPusherDeliveryID() (string, error) {
	raw := make([]byte, 18)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "ndl_" + base64.RawURLEncoding.EncodeToString(raw), nil
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

func wxPusherEventTypeMask(items []model.WxPusherEventType) (model.WxPusherEventTypes, error) {
	var mask model.WxPusherEventTypes
	seen := make(map[model.WxPusherEventType]struct{}, len(items))
	for _, item := range items {
		if _, exists := seen[item]; exists {
			return 0, ErrWxPusherPreferenceInvalid
		}
		seen[item] = struct{}{}
		switch item {
		case model.WxPusherEventTypePileAvailable:
			mask |= model.WxPusherEventPileAvailable
		case model.WxPusherEventTypeCredentialExpired:
			mask |= model.WxPusherEventCredentialExpired
		case model.WxPusherEventTypePileOffline:
			mask |= model.WxPusherEventPileOffline
		case model.WxPusherEventTypePileRecovered:
			mask |= model.WxPusherEventPileRecovered
		default:
			return 0, ErrWxPusherPreferenceInvalid
		}
	}
	return mask, nil
}

func wxPusherDeliverySummary(delivery model.NotificationDelivery) model.NotificationDeliverySummary {
	userState := delivery.UserState()
	return model.NotificationDeliverySummary{
		ID: delivery.ID, Status: delivery.Status, UserState: userState, IsTest: delivery.IsTest,
		AcceptedAt: delivery.AcceptedAt, ProviderSucceededAt: delivery.ProviderSucceededAt,
		CreatedAt: delivery.CreatedAt, UpdatedAt: delivery.UpdatedAt,
		Message:   wxPusherDeliveryMessage(userState),
		ErrorCode: wxPusherDeliveryErrorCode(delivery.LastErrorCode),
	}
}

func wxPusherDeliveryMessage(state model.NotificationDeliveryUserState) string {
	switch state {
	case model.NotificationDeliveryUserQueued:
		return "等待发送"
	case model.NotificationDeliveryUserSubmitted:
		return "已发送，请检查手机"
	case model.NotificationDeliveryUserProcessed:
		return "已发送，请检查手机"
	case model.NotificationDeliveryUserSuppressed:
		return "免打扰时段未发送"
	case model.NotificationDeliveryUserFailed:
		return "发送失败"
	case model.NotificationDeliveryUserCancelled:
		return "已取消"
	default:
		return "发送结果暂时无法确认"
	}
}

func wxPusherDeliveryErrorCode(code string) string {
	code = strings.TrimSpace(code)
	switch code {
	case "wxpusher_rate_limited":
		return "rate_limited"
	case "wxpusher_invalid_token":
		return "invalid_token"
	case "wxpusher_invalid_uid":
		return "invalid_uid"
	case "wxpusher_recipient_rejected":
		return "recipient_rejected"
	case "wxpusher_provider_timeout":
		return "timeout"
	case "wxpusher_invalid_response":
		return "invalid_response"
	case "send_outcome_unknown", "provider_status_unknown", "record_id_reentered_send_queue":
		return "ambiguous_result"
	case "wxpusher_provider_unavailable", "provider_delivery_failed":
		return "provider_unavailable"
	default:
		return ""
	}
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
