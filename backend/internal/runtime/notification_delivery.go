package runtime

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"charge-dashboard/internal/model"
)

const (
	credentialExpiredDedupeKey = "credential_expired"
	offlineNotificationPrefix  = "pile_offline:"
)

type notificationEventHub struct {
	mu          sync.Mutex
	subscribers map[string]map[chan model.Notification]struct{}
}

func (h *notificationEventHub) subscribe(userID string) chan model.Notification {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subscribers == nil {
		h.subscribers = make(map[string]map[chan model.Notification]struct{})
	}
	if h.subscribers[userID] == nil {
		h.subscribers[userID] = make(map[chan model.Notification]struct{})
	}
	channel := make(chan model.Notification, 8)
	h.subscribers[userID][channel] = struct{}{}
	return channel
}

func (h *notificationEventHub) unsubscribe(userID string, channel chan model.Notification) {
	h.mu.Lock()
	defer h.mu.Unlock()
	listeners := h.subscribers[userID]
	if listeners == nil {
		return
	}
	if _, ok := listeners[channel]; !ok {
		return
	}
	delete(listeners, channel)
	close(channel)
	if len(listeners) == 0 {
		delete(h.subscribers, userID)
	}
}

func (h *notificationEventHub) publish(notification model.Notification) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for channel := range h.subscribers[notification.UserID] {
		select {
		case channel <- notification:
		default:
			// The database is the source of truth. A slow SSE client can reload
			// the inbox instead of blocking refresh and notification delivery.
		}
	}
}

func (m *Manager) SubscribeNotifications(userID string) (chan model.Notification, error) {
	if _, err := m.runtimeFor(userID); err != nil {
		return nil, err
	}
	return m.notificationHub.subscribe(userID), nil
}

func (m *Manager) UnsubscribeNotifications(userID string, channel chan model.Notification) {
	m.notificationHub.unsubscribe(userID, channel)
}

func (m *Manager) recordNotificationOnce(notification model.Notification) (model.Notification, bool, error) {
	m.notificationMu.Lock()
	defer m.notificationMu.Unlock()
	return m.recordNotificationOnceLocked(notification)
}

func (m *Manager) recordNotificationOnceLocked(notification model.Notification) (model.Notification, bool, error) {
	if _, err := m.runtimeFor(notification.UserID); err != nil {
		return model.Notification{}, false, err
	}
	if notification.ID == "" {
		notification.ID = randomID("ntf")
	}
	if notification.CreatedAt.IsZero() {
		notification.CreatedAt = time.Now().UTC().Truncate(time.Second)
	}
	suppressed, err := m.wxPusherQuietAt(notification.UserID, notification.CreatedAt)
	if err != nil {
		return model.Notification{}, false, fmt.Errorf("load notification delivery preference: %w", err)
	}
	inserted, queued, err := m.repository.InsertNotificationWithWxPusherDeliveryIfAbsent(
		notification,
		randomID("ndl"),
		suppressed,
	)
	if err != nil {
		return model.Notification{}, false, fmt.Errorf("record notification once: %w", err)
	}
	if inserted {
		m.notificationHub.publish(notification)
	}
	if queued && !suppressed {
		m.wakeNotificationDispatcher()
	}
	return notification, inserted, nil
}

func (m *Manager) wxPusherQuietAt(userID string, at time.Time) (bool, error) {
	preference, ok, err := m.repository.LoadNotificationPreference(userID)
	if err != nil {
		return false, err
	}
	if !ok {
		preference = defaultNotificationPreference(userID, at)
	}
	if !preference.QuietHoursEnabled || preference.QuietStartMinute == preference.QuietEndMinute {
		return false, nil
	}
	location, err := time.LoadLocation(preference.Timezone)
	if err != nil {
		return false, err
	}
	local := at.In(location)
	minute := local.Hour()*60 + local.Minute()
	if preference.QuietStartMinute < preference.QuietEndMinute {
		return minute >= preference.QuietStartMinute && minute < preference.QuietEndMinute, nil
	}
	return minute >= preference.QuietStartMinute || minute < preference.QuietEndMinute, nil
}

func (m *Manager) resolveNotification(userID, dedupeKey string, at time.Time) (model.Notification, bool, error) {
	m.notificationMu.Lock()
	defer m.notificationMu.Unlock()
	return m.resolveNotificationLocked(userID, dedupeKey, at)
}

func (m *Manager) resolveNotificationLocked(userID, dedupeKey string, at time.Time) (model.Notification, bool, error) {
	notification, resolved, err := m.repository.ResolveActiveNotification(userID, dedupeKey, at)
	if err != nil {
		return model.Notification{}, false, err
	}
	if resolved {
		m.notificationHub.publish(notification)
	}
	return notification, resolved, nil
}

func (m *Manager) processPileAvailability(userID string, piles []model.Pile, events []model.PortStatusEvent) error {
	if len(piles) == 0 {
		return nil
	}
	rules, err := m.repository.ListWatchRules(userID)
	if err != nil {
		return fmt.Errorf("list pile availability rules: %w", err)
	}
	rulesByPile := make(map[string]model.WatchRule, len(rules))
	for _, rule := range rules {
		if rule.Enabled {
			rulesByPile[rule.DeviceID] = rule
		}
	}
	eventsByPile := make(map[string][]model.PortStatusEvent)
	for _, event := range events {
		if event.UserID == userID {
			eventsByPile[event.DeviceID] = append(eventsByPile[event.DeviceID], event)
		}
	}
	for _, pile := range piles {
		rule, ok := rulesByPile[pile.ID]
		if !ok {
			continue
		}
		observedAt := pile.UpdatedAt.UTC()
		if observedAt.IsZero() {
			observedAt = time.Now().UTC().Truncate(time.Second)
		}
		active, _, err := reminderRuleActiveAt(rule, observedAt)
		if err != nil {
			return err
		}
		if !active || rule.CreatedAt.After(observedAt) {
			continue
		}
		if err := m.processOnePileAvailability(userID, rule, pile, eventsByPile[pile.ID], observedAt); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) processOnePileAvailability(
	userID string,
	rule model.WatchRule,
	pile model.Pile,
	events []model.PortStatusEvent,
	observedAt time.Time,
) error {
	idlePortIDs := pileIdlePortIDs(pile)
	hasIdlePort := len(idlePortIDs) > 0
	availabilityEventID := latestPortStatusEventID(events)

	m.notificationMu.Lock()
	defer m.notificationMu.Unlock()
	state, found, err := m.repository.LoadWatchRefreshState(userID, pile.ID)
	if err != nil {
		return fmt.Errorf("load pile availability state: %w", err)
	}
	if !found || !state.AvailabilityKnown {
		if hasIdlePort && rule.Mode == model.WatchRuleTemporary {
			if err := m.recordTemporaryPileAvailabilityLocked(userID, rule, pile, idlePortIDs, events, observedAt); err != nil {
				return err
			}
		}
		return m.repository.SavePileAvailabilityState(userID, pile.ID, true, hasIdlePort, availabilityEventID, observedAt)
	}
	availabilityEventID = max(state.AvailabilityEventID, availabilityEventID)
	if state.HadIdlePort == hasIdlePort {
		if availabilityEventID > state.AvailabilityEventID {
			return m.repository.SavePileAvailabilityState(userID, pile.ID, true, hasIdlePort, availabilityEventID, observedAt)
		}
		return nil
	}
	if !hasIdlePort {
		return m.repository.SavePileAvailabilityState(userID, pile.ID, true, false, availabilityEventID, observedAt)
	}

	event, ok, err := eligiblePileAvailabilityEvent(rule, events)
	if err != nil {
		return err
	}
	if !ok {
		// A pile that becomes available outside the configured active window, or
		// comes back after the nightly power cut, establishes a new baseline but
		// must not create a delayed notification.
		return m.repository.SavePileAvailabilityState(userID, pile.ID, true, true, availabilityEventID, observedAt)
	}
	portID := event.PortID
	sourceEventID := event.ID
	_, _, err = m.recordNotificationOnceLocked(model.Notification{
		UserID: userID, Type: model.NotificationPileAvailable, Severity: "info",
		Title:    "充电桩有空闲口",
		Message:  pileAvailabilityMessage(m.notificationPileLabel(userID, pile.ID), idlePortIDs),
		DeviceID: pile.ID, PortID: &portID, SourceEventID: &sourceEventID,
		DedupeKey: fmt.Sprintf("pile_available:%d", event.ID), CreatedAt: event.ChangedAt,
	})
	if err != nil {
		return err
	}
	if rule.Mode == model.WatchRuleTemporary {
		if _, err := m.repository.CompleteWatchRule(userID, rule.ID, model.WatchCompletionNotified, observedAt); err != nil {
			return fmt.Errorf("complete notified temporary watch rule: %w", err)
		}
	}
	return m.repository.SavePileAvailabilityState(userID, pile.ID, true, true, availabilityEventID, observedAt)
}

func (m *Manager) recordTemporaryPileAvailabilityLocked(
	userID string,
	rule model.WatchRule,
	pile model.Pile,
	idlePortIDs []int,
	events []model.PortStatusEvent,
	observedAt time.Time,
) error {
	portID := idlePortIDs[0]
	var sourceEventID *int64
	if latest := latestPortStatusEventID(events); latest > 0 {
		sourceEventID = &latest
	}
	_, _, err := m.recordNotificationOnceLocked(model.Notification{
		UserID: userID, Type: model.NotificationPileAvailable, Severity: "info",
		Title:    "充电桩有空闲口",
		Message:  pileAvailabilityMessage(m.notificationPileLabel(userID, pile.ID), idlePortIDs),
		DeviceID: pile.ID, PortID: &portID, SourceEventID: sourceEventID,
		DedupeKey: "pile_available_rule:" + rule.ID, CreatedAt: observedAt,
	})
	if err != nil {
		return err
	}
	if _, err := m.repository.CompleteWatchRule(userID, rule.ID, model.WatchCompletionNotified, observedAt); err != nil {
		return fmt.Errorf("complete initially available temporary watch rule: %w", err)
	}
	return nil
}

func latestPortStatusEventID(events []model.PortStatusEvent) int64 {
	var latest int64
	for _, event := range events {
		latest = max(latest, event.ID)
	}
	return latest
}

func eligiblePileAvailabilityEvent(rule model.WatchRule, events []model.PortStatusEvent) (model.PortStatusEvent, bool, error) {
	sort.Slice(events, func(i, j int) bool { return events[i].ID < events[j].ID })
	for _, event := range events {
		if event.FromStatus == nil || *event.FromStatus != model.PortInUse || event.ToStatus != model.PortIdle ||
			rule.CreatedAt.After(event.ChangedAt) {
			continue
		}
		active, _, err := reminderRuleActiveAt(rule, event.ChangedAt)
		if err != nil {
			return model.PortStatusEvent{}, false, err
		}
		if active {
			return event, true, nil
		}
	}
	return model.PortStatusEvent{}, false, nil
}

func pileIdlePortIDs(pile model.Pile) []int {
	ids := make([]int, 0, len(pile.Ports))
	if !pile.Online {
		return ids
	}
	for _, port := range pile.Ports {
		if port.Status == model.PortIdle {
			ids = append(ids, port.ID)
		}
	}
	sort.Ints(ids)
	return ids
}

func pileAvailabilityMessage(pileLabel string, idlePortIDs []int) string {
	ports := make([]string, 0, len(idlePortIDs))
	for _, portID := range idlePortIDs {
		ports = append(ports, strconv.Itoa(portID)+" 号")
	}
	return fmt.Sprintf("%s目前有 %d 个空闲充电口：%s。", strings.TrimSpace(pileLabel), len(ports), strings.Join(ports, "、"))
}

func (m *Manager) recoverPendingPileAvailabilityNotifications(limit int) error {
	events, err := m.repository.UnnotifiedIdleTransitions(limit)
	if err != nil {
		return err
	}
	byUserAndPile := make(map[string]map[string][]model.PortStatusEvent)
	for _, event := range events {
		if byUserAndPile[event.UserID] == nil {
			byUserAndPile[event.UserID] = make(map[string][]model.PortStatusEvent)
		}
		byUserAndPile[event.UserID][event.DeviceID] = append(byUserAndPile[event.UserID][event.DeviceID], event)
	}
	for userID, byPile := range byUserAndPile {
		runtime, err := m.runtimeFor(userID)
		if err != nil {
			continue
		}
		for _, pile := range runtime.store.Snapshot().Piles {
			pileEvents, ok := byPile[pile.ID]
			if !ok {
				continue
			}
			if err := m.processPileAvailability(userID, []model.Pile{pile}, pileEvents); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *Manager) notificationPileLabel(userID, deviceID string) string {
	runtime, err := m.runtimeFor(userID)
	if err == nil {
		for _, pile := range runtime.store.Snapshot().Piles {
			if pile.ID != deviceID {
				continue
			}
			if name := strings.TrimSpace(pile.Name); name != "" {
				return name + " "
			}
			if number := strings.TrimSpace(pile.Number); number != "" {
				return "充电桩 " + number + " "
			}
		}
	}
	return "充电桩 " + deviceID + " "
}

func (m *Manager) notifyCredentialExpired(userID string, at time.Time) error {
	m.notificationMu.Lock()
	defer m.notificationMu.Unlock()
	if _, _, err := m.recordNotificationOnceLocked(model.Notification{
		UserID: userID, Type: model.NotificationCredentialExpired, Severity: "warning",
		Title: "登录凭据已失效", Message: "后台刷新已暂停，请重新扫码或更新登录凭据后恢复提醒。",
		DedupeKey: credentialExpiredDedupeKey, CreatedAt: at,
	}); err != nil {
		return err
	}
	states, err := m.repository.ListWatchRefreshStates(userID)
	if err != nil {
		return err
	}
	for _, state := range states {
		state.PausedReason = watchPauseCredentialExpired
		state.ConsecutiveFailures = 0
		state.NextAttemptAt = at.UTC()
		state.UpdatedAt = at.UTC()
		if err := m.repository.SaveWatchRefreshState(state); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) resolveCredentialExpired(userID string, at time.Time) error {
	m.notificationMu.Lock()
	defer m.notificationMu.Unlock()
	if _, _, err := m.resolveNotificationLocked(userID, credentialExpiredDedupeKey, at); err != nil {
		return err
	}
	states, err := m.repository.ListWatchRefreshStates(userID)
	if err != nil {
		return err
	}
	for _, state := range states {
		if state.PausedReason != watchPauseCredentialExpired {
			continue
		}
		state.PausedReason = ""
		state.ConsecutiveFailures = 0
		state.NextAttemptAt = at.UTC()
		state.UpdatedAt = at.UTC()
		if err := m.repository.SaveWatchRefreshState(state); err != nil {
			return err
		}
	}
	m.wakeReminderScheduler()
	return nil
}

func offlineNotificationDedupeKey(deviceID string) string {
	return offlineNotificationPrefix + deviceID
}
