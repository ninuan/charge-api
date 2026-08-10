package runtime

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"charge-dashboard/internal/model"
)

const (
	defaultBackgroundPileCacheTTL            = 60 * time.Second
	defaultBackgroundCredentialValidationTTL = 24 * time.Hour
)

var (
	ErrWatchRefreshNotEnabled = errors.New("watch refresh not enabled")
	ErrWatchPileNotOwned      = errors.New("watch pile not owned")
)

// WatchedPileRefreshResult describes one logical background refresh. Attempted
// is true only for the caller that performed the remote pile request; cache and
// coalesced consumers never inflate the remote request metric.
type WatchedPileRefreshResult struct {
	Pile                model.Pile
	FetchedAt           time.Time
	Attempted           bool
	Skipped             bool
	Cached              bool
	Coalesced           bool
	CredentialValidated bool
	NextRetryAt         *time.Time
}

type backgroundRefreshCoordinator struct {
	mu                      sync.Mutex
	cache                   map[string]backgroundPileCacheEntry
	flights                 map[string]*backgroundPileFlight
	credentialValidatedAt   map[backgroundCredentialKey]time.Time
	now                     func() time.Time
	cacheTTL                time.Duration
	credentialValidationTTL time.Duration
}

type backgroundPileCacheEntry struct {
	pile      model.Pile
	fetchedAt time.Time
}

type backgroundPileFlight struct {
	done    chan struct{}
	result  WatchedPileRefreshResult
	err     error
	waiters int
}

type backgroundCredentialKey struct {
	userID   string
	deviceID string
}

// RefreshWatchedPile refreshes one whole charging pile for one user. The
// scheduler uses the same capability internally, but this method itself does
// not start a timer, scan rules, or perform interactive credential recovery.
func (m *Manager) RefreshWatchedPile(userID, deviceID string) (WatchedPileRefreshResult, error) {
	return m.refreshWatchedPile(userID, deviceID, nil)
}

func (m *Manager) refreshWatchedPile(userID, deviceID string, beforeRemoteRequest func() error) (WatchedPileRefreshResult, error) {
	deviceID = strings.TrimSpace(deviceID)
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return WatchedPileRefreshResult{}, err
	}
	user, ok := m.User(userID)
	if !ok || !user.RefreshEnabled {
		return WatchedPileRefreshResult{}, ErrWatchRefreshNotEnabled
	}
	if !runtimeOwnsPile(runtime, deviceID) {
		return WatchedPileRefreshResult{}, ErrWatchPileNotOwned
	}
	enabled, err := m.hasEnabledPileReminder(userID, deviceID)
	if err != nil {
		return WatchedPileRefreshResult{}, err
	}
	if !enabled {
		return WatchedPileRefreshResult{}, ErrWatchRefreshNotEnabled
	}

	// Serialize background work with interactive refreshes for this user. Cross-
	// user calls still run concurrently and can share the pile-level flight.
	runtime.refreshMu.Lock()
	defer runtime.refreshMu.Unlock()

	result, err := m.loadWatchedPile(runtime, userID, deviceID, beforeRemoteRequest)
	m.recordWatchedPileMetrics(userID, result, err)
	if err != nil || result.Skipped {
		return result, err
	}

	runtime.store.MergeCapturePiles([]model.Pile{cloneBackgroundPile(result.Pile)})
	if err := m.Save(); err != nil {
		return WatchedPileRefreshResult{}, fmt.Errorf("save watched pile snapshot: %w", err)
	}
	events, err := m.repository.RecordPortStatusTransitions(userID, []model.Pile{result.Pile})
	if err != nil {
		return result, fmt.Errorf("record watched pile status transitions: %w", err)
	}
	if err := m.processPileAvailability(userID, []model.Pile{result.Pile}, events); err != nil {
		return result, fmt.Errorf("deliver watched pile notifications: %w", err)
	}
	return result, nil
}

func (m *Manager) hasEnabledPileReminder(userID, deviceID string) (bool, error) {
	rules, err := m.repository.ListWatchRules(userID)
	if err != nil {
		return false, fmt.Errorf("list watch rules for background refresh: %w", err)
	}
	for _, rule := range rules {
		if rule.DeviceID == deviceID && rule.Enabled {
			return true, nil
		}
	}
	return false, nil
}

func runtimeOwnsPile(runtime *UserRuntime, deviceID string) bool {
	if deviceID == "" {
		return false
	}
	for _, id := range runtime.client.DeviceIDs() {
		if id == deviceID {
			return true
		}
	}
	return false
}

func (m *Manager) loadWatchedPile(
	runtime *UserRuntime,
	userID string,
	deviceID string,
	beforeRemoteRequest func() error,
) (WatchedPileRefreshResult, error) {
	coordinator := &m.backgroundRefresh
	coordinator.mu.Lock()
	coordinator.initializeLocked()
	now := coordinator.now()
	credentialKey := backgroundCredentialKey{userID: userID, deviceID: deviceID}
	validatedAt := coordinator.credentialValidatedAt[credentialKey]
	validationDue := validatedAt.IsZero() || now.Sub(validatedAt) >= coordinator.credentialValidationTTL
	if entry, ok := coordinator.cache[deviceID]; !validationDue && ok && now.Sub(entry.fetchedAt) < coordinator.cacheTTL {
		coordinator.mu.Unlock()
		return WatchedPileRefreshResult{
			Pile:      cloneBackgroundPile(entry.pile),
			FetchedAt: entry.fetchedAt,
			Cached:    true,
		}, nil
	}

	flightKey := deviceID
	if validationDue {
		// A due credential check can only merge with another check for the same
		// user. It must never ride on a different user's authenticated request.
		flightKey = userID + "\x00" + deviceID
	}
	if flight, ok := coordinator.flights[flightKey]; ok {
		flight.waiters++
		coordinator.mu.Unlock()
		<-flight.done
		result := cloneWatchedPileRefreshResult(flight.result)
		result.Attempted = false
		result.Coalesced = true
		if !validationDue {
			// The shared flight may have used another user's credential. Its pile
			// status is reusable, but its credential result is not attributable to
			// this waiter.
			result.CredentialValidated = false
		}
		return result, flight.err
	}
	flight := &backgroundPileFlight{done: make(chan struct{})}
	coordinator.flights[flightKey] = flight
	coordinator.mu.Unlock()

	result, fetchErr := fetchWatchedPile(runtime, deviceID, beforeRemoteRequest)
	coordinator.mu.Lock()
	if fetchErr == nil && !result.Skipped {
		result.FetchedAt = coordinator.now()
		result.CredentialValidated = true
		result.Pile = cloneBackgroundPile(result.Pile)
		coordinator.cache[deviceID] = backgroundPileCacheEntry{
			pile:      cloneBackgroundPile(result.Pile),
			fetchedAt: result.FetchedAt,
		}
		coordinator.credentialValidatedAt[credentialKey] = result.FetchedAt
	}
	flight.result = cloneWatchedPileRefreshResult(result)
	flight.err = fetchErr
	delete(coordinator.flights, flightKey)
	close(flight.done)
	coordinator.mu.Unlock()
	return result, fetchErr
}

func fetchWatchedPile(runtime *UserRuntime, deviceID string, beforeRemoteRequest func() error) (WatchedPileRefreshResult, error) {
	fetched := runtime.client.FetchPileWithPermit(deviceID, false, beforeRemoteRequest)
	result := WatchedPileRefreshResult{
		Attempted:   fetched.Attempted > 0,
		Skipped:     fetched.Skipped > 0,
		NextRetryAt: cloneTimePointer(fetched.NextRetryAt),
	}
	if len(fetched.Piles) == 1 {
		result.Pile = cloneBackgroundPile(fetched.Piles[0])
		return result, nil
	}
	if result.Skipped {
		return result, nil
	}
	if err := fetched.FirstError(); err != nil {
		return result, err
	}
	return result, fmt.Errorf("pile %s returned no complete snapshot", deviceID)
}

func (m *Manager) recordWatchedPileMetrics(userID string, result WatchedPileRefreshResult, fetchErr error) {
	if result.Attempted {
		m.recordMetric(userID, "watch_remote")
		if result.CredentialValidated {
			m.recordMetric(userID, "watch_credential_check")
		}
		if fetchErr == nil {
			m.recordMetric(userID, "watch_remote_ok")
		} else {
			m.recordMetric(userID, "watch_remote_failed")
		}
	}
	if result.Cached {
		m.recordMetric(userID, "watch_cache")
	}
	if result.Coalesced {
		m.recordMetric(userID, "watch_coalesced")
	}
	if result.Skipped {
		m.recordMetric(userID, "watch_backoff_skipped")
	}
}

func (c *backgroundRefreshCoordinator) initializeLocked() {
	if c.cache == nil {
		c.cache = make(map[string]backgroundPileCacheEntry)
	}
	if c.flights == nil {
		c.flights = make(map[string]*backgroundPileFlight)
	}
	if c.credentialValidatedAt == nil {
		c.credentialValidatedAt = make(map[backgroundCredentialKey]time.Time)
	}
	if c.now == nil {
		c.now = time.Now
	}
	if c.cacheTTL <= 0 {
		c.cacheTTL = defaultBackgroundPileCacheTTL
	}
	if c.credentialValidationTTL <= 0 {
		c.credentialValidationTTL = defaultBackgroundCredentialValidationTTL
	}
}

func (m *Manager) markBackgroundCredentialsValidated(userID string, piles []model.Pile, at time.Time) {
	if len(piles) == 0 {
		return
	}
	coordinator := &m.backgroundRefresh
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	coordinator.initializeLocked()
	for _, pile := range piles {
		if pile.ID != "" {
			coordinator.credentialValidatedAt[backgroundCredentialKey{userID: userID, deviceID: pile.ID}] = at
		}
	}
}

func (m *Manager) invalidateBackgroundCredentialValidation(userID string) {
	coordinator := &m.backgroundRefresh
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	coordinator.initializeLocked()
	for key := range coordinator.credentialValidatedAt {
		if key.userID == userID {
			delete(coordinator.credentialValidatedAt, key)
		}
	}
}

func (m *Manager) invalidateBackgroundPileValidation(userID, deviceID string) {
	coordinator := &m.backgroundRefresh
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	coordinator.initializeLocked()
	delete(coordinator.credentialValidatedAt, backgroundCredentialKey{userID: userID, deviceID: deviceID})
}

func cloneBackgroundPile(pile model.Pile) model.Pile {
	clone := pile
	clone.Ports = append([]model.Port(nil), pile.Ports...)
	clone.UsedPortIDs = append([]int(nil), pile.UsedPortIDs...)
	return clone
}

func cloneWatchedPileRefreshResult(result WatchedPileRefreshResult) WatchedPileRefreshResult {
	result.Pile = cloneBackgroundPile(result.Pile)
	result.NextRetryAt = cloneTimePointer(result.NextRetryAt)
	return result
}

func cloneTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
