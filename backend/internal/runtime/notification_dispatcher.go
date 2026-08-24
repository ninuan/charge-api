package runtime

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"charge-dashboard/internal/model"
	"charge-dashboard/internal/wxpusher"
)

const (
	defaultNotificationDispatcherPollInterval = 5 * time.Second
	notificationDeliveryLease                 = 2 * time.Minute
	maxNotificationSendAttempts               = 3
	maxNotificationRetryBackoff               = 15 * time.Minute
)

var (
	ErrNotificationDispatcherRunning = errors.New("notification dispatcher already running")
	notificationSendBackoffs         = []time.Duration{time.Minute, 5 * time.Minute}
	notificationStatusDelays         = []time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute}
)

type NotificationDeliveryClient interface {
	Send(context.Context, wxpusher.Message) (wxpusher.SendResult, error)
	QueryMessageStatus(context.Context, string) (wxpusher.MessageStatus, error)
}

type notificationDispatcherCoordinator struct {
	mu            sync.Mutex
	running       bool
	wake          chan struct{}
	done          chan struct{}
	now           func() time.Time
	pollInterval  time.Duration
	client        NotificationDeliveryClient
	publicBaseURL string
}

func (m *Manager) StartNotificationDispatcher(ctx context.Context, client NotificationDeliveryClient, publicBaseURL string) error {
	if ctx == nil || client == nil {
		return fmt.Errorf("notification dispatcher requires context and client")
	}
	coordinator := &m.notificationDispatcher
	coordinator.mu.Lock()
	if coordinator.running {
		coordinator.mu.Unlock()
		return ErrNotificationDispatcherRunning
	}
	if coordinator.now == nil {
		coordinator.now = time.Now
	}
	if coordinator.pollInterval <= 0 {
		coordinator.pollInterval = defaultNotificationDispatcherPollInterval
	}
	coordinator.running = true
	coordinator.client = client
	coordinator.publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	coordinator.wake = make(chan struct{}, 1)
	coordinator.done = make(chan struct{})
	wake, done := coordinator.wake, coordinator.done
	coordinator.mu.Unlock()
	go m.runNotificationDispatcher(ctx, wake, done)
	return nil
}

func (m *Manager) WaitNotificationDispatcher(ctx context.Context) error {
	coordinator := &m.notificationDispatcher
	coordinator.mu.Lock()
	done, running := coordinator.done, coordinator.running
	coordinator.mu.Unlock()
	if !running || done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Manager) wakeNotificationDispatcher() {
	coordinator := &m.notificationDispatcher
	coordinator.mu.Lock()
	wake, running := coordinator.wake, coordinator.running
	coordinator.mu.Unlock()
	if !running || wake == nil {
		return
	}
	select {
	case wake <- struct{}{}:
	default:
	}
}

func (m *Manager) runNotificationDispatcher(ctx context.Context, wake <-chan struct{}, done chan struct{}) {
	defer func() {
		coordinator := &m.notificationDispatcher
		coordinator.mu.Lock()
		coordinator.running = false
		close(done)
		coordinator.mu.Unlock()
	}()
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-wake:
			stopAndDrainTimer(timer)
			timer.Reset(0)
		case <-timer.C:
			if err := m.runNotificationDispatcherOnce(ctx, m.notificationDispatcherNow()); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("notification dispatcher: %v", err)
			}
			timer.Reset(m.notificationDispatcherPollInterval())
		}
	}
}

func (m *Manager) runNotificationDispatcherOnce(ctx context.Context, now time.Time) error {
	staleBefore := now.Add(-notificationDeliveryLease)
	if _, err := m.repository.RecoverStaleSendingDeliveries(staleBefore, now); err != nil {
		return err
	}
	accepted, err := m.repository.ClaimAcceptedNotificationDeliveries(now, staleBefore, 1)
	if err != nil {
		return err
	}
	if len(accepted) == 1 {
		if err := m.confirmNotificationDelivery(ctx, accepted[0], now); err != nil {
			return err
		}
	}
	deliveries, err := m.repository.ClaimNotificationDeliveries(now, staleBefore, 1)
	if err != nil {
		return err
	}
	if len(deliveries) == 1 {
		return m.sendNotificationDelivery(ctx, deliveries[0], now)
	}
	return nil
}

func (m *Manager) sendNotificationDelivery(ctx context.Context, delivery model.NotificationDelivery, now time.Time) error {
	if delivery.ProviderRecordID != "" {
		return m.finishDelivery(delivery, model.NotificationDeliveryUncertain, "record_id_reentered_send_queue", now)
	}
	binding, found, err := m.repository.LoadWxPusherBinding(delivery.UserID)
	if err != nil {
		return err
	}
	if !found || !binding.Enabled {
		return m.finishDelivery(delivery, model.NotificationDeliveryCancelled, "channel_disabled", now)
	}
	client, publicBaseURL := m.notificationDispatcherConfig()
	message := wxpusher.Message{
		UID: binding.UID, ContentType: wxpusher.ContentTypeText,
	}
	if delivery.IsTest {
		message.Summary = "Charge Console 测试消息"
		message.Content = "微信提醒已连接。之后开启的空闲提醒和账户异常会发送到这里。"
		message.URL = dashboardURL(publicBaseURL)
	} else {
		if delivery.NotificationID == nil {
			return m.finishDelivery(delivery, model.NotificationDeliveryFailed, "notification_missing", now)
		}
		notification, found, err := m.repository.LoadNotification(delivery.UserID, *delivery.NotificationID)
		if err != nil {
			return err
		}
		if !found {
			return m.finishDelivery(delivery, model.NotificationDeliveryCancelled, "notification_missing", now)
		}
		if binding.EventTypes&runtimeWxPusherEventMask(notification.Type) == 0 {
			return m.finishDelivery(delivery, model.NotificationDeliveryCancelled, "event_type_disabled", now)
		}
		quiet, err := m.wxPusherQuietAt(delivery.UserID, now)
		if err != nil {
			return err
		}
		if quiet {
			return m.finishDelivery(delivery, model.NotificationDeliverySuppressed, "quiet_hours", now)
		}
		message.Summary = truncateRunes(notification.Title, wxpusher.MaxSummaryLength)
		message.Content = notification.Message
		message.URL = notificationURL(publicBaseURL, notification.ID)
	}
	requestCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 12*time.Second)
	defer cancel()
	result, sendErr := client.Send(requestCtx, message)
	if sendErr != nil {
		return m.handleNotificationSendError(binding, delivery, sendErr, now)
	}
	delivery.Status = model.NotificationDeliveryAccepted
	delivery.ProviderRecordID = result.SendRecordID
	delivery.ProviderMessageContentID = result.MessageContentID
	delivery.LastErrorCode, delivery.LastErrorMessage = "", ""
	delivery.ClaimedAt = nil
	delivery.AcceptedAt = timePointer(now)
	next := now.Add(notificationStatusDelays[0])
	delivery.NextAttemptAt = &next
	delivery.UpdatedAt = now
	if err := m.repository.SaveNotificationDelivery(delivery); err != nil {
		return err
	}
	return m.repository.RecordWxPusherDeliveryAccepted(binding.UserID, now)
}

func (m *Manager) handleNotificationSendError(binding model.WxPusherBinding, delivery model.NotificationDelivery, sendErr error, now time.Time) error {
	code := string(wxpusher.CodeOf(sendErr))
	if code == "" {
		code = "wxpusher_unknown"
	}
	if err := m.repository.RecordWxPusherDeliveryError(binding.UserID, code, now); err != nil {
		return err
	}
	status := model.NotificationDeliveryFailed
	if notificationSendOutcomeAmbiguous(sendErr) {
		status = model.NotificationDeliveryUncertain
	} else if wxpusher.IsRetryable(sendErr) && delivery.AttemptCount < maxNotificationSendAttempts {
		status = model.NotificationDeliveryRetryWait
		backoff := notificationSendBackoffs[delivery.AttemptCount-1]
		if retryAfter := wxpusher.RetryAfterOf(sendErr); retryAfter > backoff {
			backoff = retryAfter
		}
		if backoff > maxNotificationRetryBackoff {
			backoff = maxNotificationRetryBackoff
		}
		next := now.Add(backoff)
		delivery.NextAttemptAt = &next
	}
	delivery.Status = status
	delivery.ClaimedAt = nil
	if status != model.NotificationDeliveryRetryWait {
		delivery.NextAttemptAt = nil
	}
	delivery.LastErrorCode, delivery.LastErrorMessage, delivery.UpdatedAt = code, "", now
	return m.repository.SaveNotificationDelivery(delivery)
}

func (m *Manager) confirmNotificationDelivery(ctx context.Context, delivery model.NotificationDelivery, now time.Time) error {
	client, _ := m.notificationDispatcherConfig()
	requestCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 12*time.Second)
	defer cancel()
	status, queryErr := client.QueryMessageStatus(requestCtx, delivery.ProviderRecordID)
	if queryErr != nil {
		code := string(wxpusher.CodeOf(queryErr))
		if code == "" {
			code = "wxpusher_unknown"
		}
		delivery.LastErrorCode, delivery.LastErrorMessage = code, ""
		return m.rescheduleAcceptedDelivery(delivery, now)
	}
	if status.Succeeded {
		delivery.Status = model.NotificationDeliveryProviderSucceeded
		delivery.NextAttemptAt, delivery.ClaimedAt = nil, nil
		delivery.LastErrorCode, delivery.LastErrorMessage = "", ""
		delivery.ProviderSucceededAt, delivery.UpdatedAt = timePointer(now), now
		if err := m.repository.SaveNotificationDelivery(delivery); err != nil {
			return err
		}
		return m.repository.RecordWxPusherProviderSuccess(delivery.UserID, now)
	}
	if status.Failed {
		return m.finishDelivery(delivery, model.NotificationDeliveryFailed, "provider_delivery_failed", now)
	}
	return m.rescheduleAcceptedDelivery(delivery, now)
}

func (m *Manager) rescheduleAcceptedDelivery(delivery model.NotificationDelivery, now time.Time) error {
	if delivery.AcceptedAt == nil {
		return m.finishDelivery(delivery, model.NotificationDeliveryUncertain, "accepted_time_missing", now)
	}
	elapsed := now.Sub(*delivery.AcceptedAt)
	var next time.Time
	switch {
	case elapsed < notificationStatusDelays[1]:
		next = delivery.AcceptedAt.Add(notificationStatusDelays[1])
	case elapsed < notificationStatusDelays[2]:
		next = delivery.AcceptedAt.Add(notificationStatusDelays[2])
	default:
		return m.finishDelivery(delivery, model.NotificationDeliveryUncertain, "provider_status_unknown", now)
	}
	delivery.Status, delivery.NextAttemptAt, delivery.ClaimedAt, delivery.UpdatedAt = model.NotificationDeliveryAccepted, &next, nil, now
	return m.repository.SaveNotificationDelivery(delivery)
}

func (m *Manager) finishDelivery(delivery model.NotificationDelivery, status model.NotificationDeliveryStatus, code string, now time.Time) error {
	delivery.Status, delivery.LastErrorCode, delivery.LastErrorMessage = status, code, ""
	delivery.NextAttemptAt, delivery.ClaimedAt, delivery.UpdatedAt = nil, nil, now
	return m.repository.SaveNotificationDelivery(delivery)
}

func (m *Manager) notificationDispatcherConfig() (NotificationDeliveryClient, string) {
	coordinator := &m.notificationDispatcher
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	return coordinator.client, coordinator.publicBaseURL
}

func (m *Manager) notificationDispatcherNow() time.Time {
	coordinator := &m.notificationDispatcher
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	return coordinator.now().UTC().Truncate(time.Second)
}

func (m *Manager) notificationDispatcherPollInterval() time.Duration {
	coordinator := &m.notificationDispatcher
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	return coordinator.pollInterval
}

func notificationSendOutcomeAmbiguous(err error) bool {
	code := wxpusher.CodeOf(err)
	return code == wxpusher.ErrorProviderTimeout || code == wxpusher.ErrorInvalidResponse ||
		(code == wxpusher.ErrorProviderUnavailable && wxpusher.HTTPStatusOf(err) == 0)
}

func runtimeWxPusherEventMask(notificationType model.NotificationType) model.WxPusherEventTypes {
	switch notificationType {
	case model.NotificationPileAvailable:
		return model.WxPusherEventPileAvailable
	case model.NotificationCredentialExpired:
		return model.WxPusherEventCredentialExpired
	case model.NotificationPileOffline:
		return model.WxPusherEventPileOffline
	case model.NotificationPileRecovered:
		return model.WxPusherEventPileRecovered
	default:
		return 0
	}
}

func notificationURL(baseURL, notificationID string) string {
	if baseURL == "" {
		return ""
	}
	return baseURL + "/dashboard?notification=" + url.QueryEscape(notificationID)
}

func dashboardURL(baseURL string) string {
	if baseURL == "" {
		return ""
	}
	return baseURL + "/dashboard"
}

func truncateRunes(value string, limit int) string {
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit])
}

func timePointer(value time.Time) *time.Time {
	value = value.UTC().Truncate(time.Second)
	return &value
}
