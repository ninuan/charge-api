package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"charge-dashboard/internal/charger"
	"charge-dashboard/internal/model"
	"charge-dashboard/internal/parser"
	"charge-dashboard/internal/persistence"
)

const testBackgroundPileID = "2601201412385560201"

func TestRefreshWatchedPileUsesOwnCredentialCacheAndSeparateMetrics(t *testing.T) {
	var requestCount int32
	var cookieMu sync.Mutex
	cookies := make([]string, 0, 3)
	manager, owner, other := newBackgroundRefreshTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		if got := r.Form.Get("id"); got != testBackgroundPileID {
			t.Errorf("request pile id = %q, want %q", got, testBackgroundPileID)
		}
		atomic.AddInt32(&requestCount, 1)
		cookieMu.Lock()
		cookies = append(cookies, r.Header.Get("Cookie"))
		cookieMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":%q,"number":"6201","status":"在线","opennum":10,"used":[2,8]}`, testBackgroundPileID)
	}))

	base := time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC)
	current := base
	manager.backgroundRefresh.mu.Lock()
	manager.backgroundRefresh.now = func() time.Time { return current }
	manager.backgroundRefresh.mu.Unlock()

	first, err := manager.RefreshWatchedPile(owner.ID, testBackgroundPileID)
	if err != nil || !first.Attempted || first.Cached || !first.CredentialValidated || len(first.Pile.Ports) != 10 {
		t.Fatalf("first background refresh = %+v, err %v", first, err)
	}
	second, err := manager.RefreshWatchedPile(owner.ID, testBackgroundPileID)
	if err != nil || !second.Cached || second.Attempted || second.CredentialValidated {
		t.Fatalf("cached background refresh = %+v, err %v", second, err)
	}

	// A fresh shared snapshot cannot hide that another user's credential has
	// never been checked. The second user must perform one request with its own
	// cookie before becoming eligible for shared cache hits.
	otherFirst, err := manager.RefreshWatchedPile(other.ID, testBackgroundPileID)
	if err != nil || !otherFirst.Attempted || !otherFirst.CredentialValidated || otherFirst.Cached {
		t.Fatalf("other-user credential check = %+v, err %v", otherFirst, err)
	}
	if got := atomic.LoadInt32(&requestCount); got != 2 {
		t.Fatalf("remote requests = %d, want 2", got)
	}
	cookieMu.Lock()
	observedCookies := append([]string(nil), cookies...)
	cookieMu.Unlock()
	if len(observedCookies) != 2 || !strings.Contains(observedCookies[0], "sid=owner-secret") ||
		!strings.Contains(observedCookies[1], "sid=other-secret") {
		t.Fatalf("credential checks did not use each user's cookie: %q", observedCookies)
	}

	// Even a cache hit must feed the consumer's own history. Remove the second
	// user's baseline, consume the cache, and verify all ten ports are restored.
	if _, err := manager.repository.DeletePortStatusEvents(other.ID, testBackgroundPileID); err != nil {
		t.Fatalf("DeletePortStatusEvents: %v", err)
	}
	otherCached, err := manager.RefreshWatchedPile(other.ID, testBackgroundPileID)
	if err != nil || !otherCached.Cached {
		t.Fatalf("other-user cache hit = %+v, err %v", otherCached, err)
	}
	events, err := manager.repository.PortStatusEvents(persistence.PortStatusEventQuery{
		UserID: other.ID, DeviceID: testBackgroundPileID, Limit: 20,
	})
	if err != nil || len(events) != 10 {
		t.Fatalf("cached snapshot history events = %d, err %v; want 10", len(events), err)
	}

	// At 24 hours, a fresh-looking shared cache entry is deliberately ignored
	// and the user's own credential is checked again.
	current = base.Add(defaultBackgroundCredentialValidationTTL + time.Second)
	manager.backgroundRefresh.mu.Lock()
	entry := manager.backgroundRefresh.cache[testBackgroundPileID]
	entry.fetchedAt = current
	manager.backgroundRefresh.cache[testBackgroundPileID] = entry
	manager.backgroundRefresh.mu.Unlock()
	revalidated, err := manager.RefreshWatchedPile(owner.ID, testBackgroundPileID)
	if err != nil || !revalidated.Attempted || revalidated.Cached || !revalidated.CredentialValidated {
		t.Fatalf("24-hour credential validation = %+v, err %v", revalidated, err)
	}
	if got := atomic.LoadInt32(&requestCount); got != 3 {
		t.Fatalf("remote requests after revalidation = %d, want 3", got)
	}

	watchRemote, err := manager.repository.MetricKindCount("watch_remote", time.Time{})
	if err != nil || watchRemote != 3 {
		t.Fatalf("watch_remote metric = %d, err %v; want 3", watchRemote, err)
	}
	watchCache, err := manager.repository.MetricKindCount("watch_cache", time.Time{})
	if err != nil || watchCache != 2 {
		t.Fatalf("watch_cache metric = %d, err %v; want 2", watchCache, err)
	}
	interactiveRequests, err := manager.repository.MetricKindCount("request", time.Time{})
	if err != nil || interactiveRequests != 0 {
		t.Fatalf("interactive request metric = %d, err %v; want 0", interactiveRequests, err)
	}
	interactiveRemote, err := manager.repository.MetricKindCount("remote", time.Time{})
	if err != nil || interactiveRemote != 0 {
		t.Fatalf("interactive remote metric = %d, err %v; want 0", interactiveRemote, err)
	}

	manager.backgroundRefresh.mu.Lock()
	cachedPile := cloneBackgroundPile(manager.backgroundRefresh.cache[testBackgroundPileID].pile)
	manager.backgroundRefresh.mu.Unlock()
	encoded, err := json.Marshal(cachedPile)
	if err != nil {
		t.Fatalf("marshal cached pile: %v", err)
	}
	for _, privateValue := range []string{"owner-secret", "other-secret", owner.ID, other.ID} {
		if strings.Contains(string(encoded), privateValue) {
			t.Fatalf("shared pile cache leaked private value %q: %s", privateValue, encoded)
		}
	}
}

func TestRefreshWatchedPileCoalescesValidatedUsersByWholePile(t *testing.T) {
	var requestCount int32
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	var startedOnce sync.Once
	manager, owner, other := newBackgroundRefreshTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		startedOnce.Do(func() { close(requestStarted) })
		<-releaseRequest
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":%q,"number":"6201","status":"在线","opennum":10,"used":[4]}`, testBackgroundPileID)
	}))

	now := time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC)
	manager.backgroundRefresh.mu.Lock()
	manager.backgroundRefresh.now = func() time.Time { return now }
	manager.backgroundRefresh.mu.Unlock()
	manager.markBackgroundCredentialsValidated(owner.ID, []model.Pile{{ID: testBackgroundPileID}}, now)
	manager.markBackgroundCredentialsValidated(other.ID, []model.Pile{{ID: testBackgroundPileID}}, now)

	type outcome struct {
		result WatchedPileRefreshResult
		err    error
	}
	ownerOutcome := make(chan outcome, 1)
	go func() {
		result, err := manager.RefreshWatchedPile(owner.ID, testBackgroundPileID)
		ownerOutcome <- outcome{result: result, err: err}
	}()
	<-requestStarted
	otherOutcome := make(chan outcome, 1)
	go func() {
		result, err := manager.RefreshWatchedPile(other.ID, testBackgroundPileID)
		otherOutcome <- outcome{result: result, err: err}
	}()
	deadline := time.Now().Add(time.Second)
	for {
		manager.backgroundRefresh.mu.Lock()
		flight := manager.backgroundRefresh.flights[testBackgroundPileID]
		joined := flight != nil && flight.waiters == 1
		manager.backgroundRefresh.mu.Unlock()
		if joined {
			break
		}
		if time.Now().After(deadline) {
			close(releaseRequest)
			t.Fatal("second user did not join the pile-level flight")
		}
		time.Sleep(time.Millisecond)
	}
	close(releaseRequest)
	second := <-otherOutcome
	otherResult, otherErr := second.result, second.err
	first := <-ownerOutcome
	if first.err != nil || otherErr != nil {
		t.Fatalf("coalesced refresh errors: owner=%v other=%v", first.err, otherErr)
	}
	if got := atomic.LoadInt32(&requestCount); got != 1 {
		t.Fatalf("same-pile concurrent requests = %d, want 1", got)
	}
	if !first.result.Attempted || !otherResult.Coalesced || otherResult.Attempted || otherResult.CredentialValidated {
		t.Fatalf("unexpected coalesced results: owner=%+v other=%+v", first.result, otherResult)
	}
	if len(first.result.Pile.Ports) != 10 || len(otherResult.Pile.Ports) != 10 {
		t.Fatalf("whole-pile result was split: owner=%d other=%d", len(first.result.Pile.Ports), len(otherResult.Pile.Ports))
	}
	coalesced, err := manager.repository.MetricKindCount("watch_coalesced", time.Time{})
	if err != nil || coalesced != 1 {
		t.Fatalf("watch_coalesced metric = %d, err %v; want 1", coalesced, err)
	}
	for _, userID := range []string{owner.ID, other.ID} {
		events, err := manager.repository.PortStatusEvents(persistence.PortStatusEventQuery{
			UserID: userID, DeviceID: testBackgroundPileID, Limit: 20,
		})
		if err != nil || len(events) != 10 {
			t.Fatalf("user %s history events = %d, err %v; want 10", userID, len(events), err)
		}
	}
}

func TestRefreshWatchedPileRejectsFavoritesAndUnownedPilesWithoutTraffic(t *testing.T) {
	var requestCount int32
	manager, owner, _ := newBackgroundRefreshTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	rules, err := manager.WatchRules(owner.ID)
	if err != nil {
		t.Fatalf("WatchRules: %v", err)
	}
	for _, rule := range rules {
		if err := manager.DeleteWatchRule(owner.ID, rule.ID); err != nil {
			t.Fatalf("DeleteWatchRule: %v", err)
		}
	}
	portID := 1
	if _, err := manager.CreateWatchRule(owner.ID, model.WatchRuleCreateRequest{
		DeviceID: testBackgroundPileID, PortID: &portID, NotifyIdle: false,
	}); err != nil {
		t.Fatalf("CreateWatchRule favorite: %v", err)
	}
	if _, err := manager.RefreshWatchedPile(owner.ID, testBackgroundPileID); !errors.Is(err, ErrWatchRefreshNotEnabled) {
		t.Fatalf("favorite-only refresh error = %v", err)
	}
	if _, err := manager.RefreshWatchedPile(owner.ID, "2601201412385560999"); !errors.Is(err, ErrWatchPileNotOwned) {
		t.Fatalf("unowned-pile refresh error = %v", err)
	}
	if got := atomic.LoadInt32(&requestCount); got != 0 {
		t.Fatalf("rejected background refresh produced %d requests", got)
	}
}

func TestRefreshWatchedPileDoesNotCacheOrShareAuthenticationFailures(t *testing.T) {
	var requestCount int32
	manager, owner, other := newBackgroundRefreshTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		http.Error(w, "expired", http.StatusUnauthorized)
	}))
	for _, userID := range []string{owner.ID, other.ID} {
		result, err := manager.RefreshWatchedPile(userID, testBackgroundPileID)
		if !charger.IsAuthExpired(err) || !result.Attempted || result.Cached {
			t.Fatalf("auth failure for %s = %+v, err %v", userID, result, err)
		}
	}
	if got := atomic.LoadInt32(&requestCount); got != 2 {
		t.Fatalf("cross-user auth failures produced %d requests, want 2 independent checks", got)
	}
	manager.backgroundRefresh.mu.Lock()
	_, cached := manager.backgroundRefresh.cache[testBackgroundPileID]
	manager.backgroundRefresh.mu.Unlock()
	if cached {
		t.Fatal("authentication failure entered shared pile cache")
	}
	failed, err := manager.repository.MetricKindCount("watch_remote_failed", time.Time{})
	if err != nil || failed != 2 {
		t.Fatalf("watch_remote_failed metric = %d, err %v; want 2", failed, err)
	}
}

func newBackgroundRefreshTestManager(t *testing.T, handler http.Handler) (*Manager, model.User, model.User) {
	t.Helper()
	server := newIPv4TestServer(t, handler)
	repository := testRepository(t)
	now := time.Now().UTC().Truncate(time.Second)
	owner := model.User{
		ID: "background-owner", Username: "background-owner", PasswordHash: "hash",
		Role: model.RoleUser, Enabled: true, DeviceLimit: 10, RefreshEnabled: true,
		CreatedAt: now, UpdatedAt: now,
	}
	other := model.User{
		ID: "background-other", Username: "background-other", PasswordHash: "hash",
		Role: model.RoleUser, Enabled: true, DeviceLimit: 10, RefreshEnabled: true,
		CreatedAt: now, UpdatedAt: now,
	}
	ports := make([]model.Port, 0, 10)
	for id := 1; id <= 10; id++ {
		ports = append(ports, model.Port{ID: id, Status: model.PortOffline, UpdatedAt: now})
	}
	pile := model.Pile{
		ID: testBackgroundPileID, Number: "6201", Status: "离线", OpenNum: 10,
		Online: false, UpdatedAt: now, Ports: ports,
	}
	settings := normalizeRegistrationSettings(model.RegistrationSettings{
		DefaultDeviceLimit: 10, DefaultRefreshEnabled: true,
		StatsRetentionDays: 90, PortHistoryRetentionDays: 90,
	})
	if err := repository.Save(persistence.State{
		Version: stateVersion, Users: []model.User{owner, other}, Settings: settings,
		UserStates: map[string]persistence.UserState{
			owner.ID: {Piles: []model.Pile{pile}, DeviceIDs: []string{testBackgroundPileID}, Cookie: "sid=owner-secret"},
			other.ID: {Piles: []model.Pile{pile}, DeviceIDs: []string{testBackgroundPileID}, Cookie: "sid=other-secret"},
		},
	}); err != nil {
		t.Fatalf("save background refresh fixture: %v", err)
	}
	manager, err := NewManager(repository, "", []parser.CaptureRequest{{
		Name: "template", URL: server.URL, Method: http.MethodPost, Body: "id=template",
		Headers: map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
	}}, "", 30*time.Second)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	for _, user := range []model.User{owner, other} {
		portID := 1
		if _, err := manager.CreateWatchRule(user.ID, model.WatchRuleCreateRequest{
			DeviceID: testBackgroundPileID, PortID: &portID, NotifyIdle: true,
		}); err != nil {
			t.Fatalf("CreateWatchRule for %s: %v", user.ID, err)
		}
	}
	return manager, owner, other
}
