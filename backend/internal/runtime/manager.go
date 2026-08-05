package runtime

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"charge-dashboard/internal/auth"
	"charge-dashboard/internal/charger"
	"charge-dashboard/internal/mocele"
	"charge-dashboard/internal/model"
	"charge-dashboard/internal/parser"
	"charge-dashboard/internal/persistence"
	"charge-dashboard/internal/store"
)

const moceleAppID = "wx9cbffc15d3cb7739"

var ErrYYBBindingRequired = errors.New("yyb binding required")

type YYBCodeClient interface {
	GetCode(ctx context.Context, ref string, appID string) (string, error)
	RefreshAccount(ctx context.Context, ref string) error
}

type MoceleCookieClient interface {
	ExchangeCode(ctx context.Context, deviceID string, code string) (mocele.CookieResult, error)
}

const (
	stateVersion                = 3
	defaultDeviceLimit          = 10
	maxDevicesPerUser           = defaultDeviceLimit
	defaultStatsRetentionDays   = 90
	defaultHistoryRetentionDays = 90
	maxRecoveryDiagnostics      = 20
	diagnosticOperationRecovery = "credential_recovery"
	diagnosticOperationAddPile  = "add_pile"
	diagnosticOperationRefresh  = "refresh"
	diagnosticOperationCookie   = "update_cookie"
	diagnosticOperationScan     = "scan_login"
	diagnosticOperationSync     = "sync_cookie"
	diagnosticOperationAuth     = "auth_protection"
)

var recoveryStatusCodePattern = regexp.MustCompile(`(?:status=|returned\s+)([1-5][0-9]{2})`)

type Manager struct {
	mu          sync.RWMutex
	saveMu      sync.Mutex
	repository  *persistence.Store
	requests    []parser.CaptureRequest
	minInterval time.Duration
	users       map[string]model.User
	runtimes    map[string]*UserRuntime
	initialPass string
	migrated    bool
	settings    model.RegistrationSettings
	invites     map[string]model.InviteCode
}

type UserRuntime struct {
	mu                  sync.Mutex
	refreshMu           sync.Mutex
	store               *store.DashboardStore
	client              *charger.Client
	stats               model.TrafficStats
	lastRemoteFetch     time.Time
	minInterval         time.Duration
	yybBinding          *model.YYBBinding
	recoveryDiagnostics []model.RecoveryDiagnostic
}

func NewManager(
	repository *persistence.Store,
	legacyJSONPath string,
	requests []parser.CaptureRequest,
	adminPassword string,
	minInterval time.Duration,
) (*Manager, error) {
	m := &Manager{
		repository:  repository,
		requests:    requests,
		minInterval: minInterval,
		users:       make(map[string]model.User),
		runtimes:    make(map[string]*UserRuntime),
		invites:     make(map[string]model.InviteCode),
		settings: model.RegistrationSettings{
			OpenRegistration: true, InviteRequired: true,
			DefaultDeviceLimit: defaultDeviceLimit, DefaultRefreshEnabled: true,
			StatsRetentionDays:       defaultStatsRetentionDays,
			PortHistoryRetentionDays: defaultHistoryRetentionDays,
		},
	}

	state, hasState, err := repository.Load()
	if err != nil {
		return nil, err
	}
	if hasState && legacyJSONPath != "" {
		legacyState, exists, err := persistence.LoadJSON(legacyJSONPath)
		if err != nil {
			return nil, err
		}
		if exists {
			if err := persistence.ArchiveMigratedJSON(legacyJSONPath, legacyState); err != nil {
				return nil, err
			}
		}
	}
	if !hasState && legacyJSONPath != "" {
		legacyState, exists, err := persistence.LoadJSON(legacyJSONPath)
		if err != nil {
			return nil, err
		}
		if exists {
			state = legacyState
			hasState = true
			m.migrated = true
		}
	}

	if hasState && len(state.Users) > 0 {
		settingsUpdated := false
		if state.Settings.DefaultDeviceLimit > 0 {
			m.settings = normalizeRegistrationSettings(state.Settings)
			settingsUpdated = m.settings != state.Settings
		}
		for _, invite := range state.Invites {
			m.invites[invite.ID] = invite
		}
		for _, user := range state.Users {
			if user.DeviceLimit <= 0 {
				user.DeviceLimit = m.settings.DefaultDeviceLimit
			}
			m.users[user.ID] = user
			userState := state.UserStates[user.ID]
			m.runtimes[user.ID] = newUserRuntime(requests, userState, minInterval)
		}
		// 遗留的 sha256$ 弱哈希在启动时整体套上 argon2id（无需明文），
		// 不必等用户登录才升级——从不登录的账户会永远留着弱哈希。
		wrappedLegacy := false
		for id, user := range m.users {
			if !auth.IsLegacySHA256(user.PasswordHash) {
				continue
			}
			upgraded, err := auth.WrapLegacySHA256(user.PasswordHash)
			if err != nil {
				continue
			}
			user.PasswordHash = upgraded
			user.UpdatedAt = time.Now()
			m.users[id] = user
			wrappedLegacy = true
		}
		if m.migrated || wrappedLegacy || settingsUpdated {
			if err := m.Save(); err != nil {
				return nil, err
			}
		}
		if m.migrated {
			if err := persistence.ArchiveMigratedJSON(legacyJSONPath, state); err != nil {
				return nil, err
			}
		}
		if _, _, err := m.runRetentionMaintenance(time.Now()); err != nil {
			return nil, fmt.Errorf("run startup retention maintenance: %w", err)
		}
		return m, nil
	}

	password := strings.TrimSpace(adminPassword)
	if password == "" {
		generated, err := randomPassword()
		if err != nil {
			return nil, err
		}
		password = generated
		m.initialPass = generated
	}
	admin, err := newUser("admin", password, model.RoleAdmin, true)
	if err != nil {
		return nil, err
	}
	admin.DeviceLimit = m.settings.DefaultDeviceLimit
	admin.RefreshEnabled = m.settings.DefaultRefreshEnabled
	m.users[admin.ID] = admin

	initialState := persistence.UserState{}
	if hasState {
		initialState = persistence.UserState{
			Piles:     state.Piles,
			Refresh:   state.Refresh,
			DeviceIDs: state.DeviceIDs,
			Cookie:    state.Cookie,
		}
	}
	m.runtimes[admin.ID] = newUserRuntime(requests, initialState, minInterval)

	if err := m.Save(); err != nil {
		return nil, err
	}
	if m.migrated {
		if err := persistence.ArchiveMigratedJSON(legacyJSONPath, state); err != nil {
			return nil, err
		}
	}
	if _, _, err := m.runRetentionMaintenance(time.Now()); err != nil {
		return nil, fmt.Errorf("run startup retention maintenance: %w", err)
	}
	return m, nil
}

func (m *Manager) InitialAdminPassword() string {
	return m.initialPass
}

func (m *Manager) Ping(ctx context.Context) error {
	return m.repository.Ping(ctx)
}

func (m *Manager) MigratedLegacyJSON() bool {
	return m.migrated
}

func (m *Manager) Save() error {
	m.saveMu.Lock()
	defer m.saveMu.Unlock()

	m.mu.RLock()
	users := make([]model.User, 0, len(m.users))
	userStates := make(map[string]persistence.UserState, len(m.runtimes))
	invites := make([]model.InviteCode, 0, len(m.invites))
	settings := m.settings
	for _, user := range m.users {
		users = append(users, user)
	}
	sort.Slice(users, func(i, j int) bool {
		return users[i].Username < users[j].Username
	})
	for userID, runtime := range m.runtimes {
		userStates[userID] = runtime.state()
	}
	for _, invite := range m.invites {
		invites = append(invites, invite)
	}
	m.mu.RUnlock()
	sort.Slice(invites, func(i, j int) bool { return invites[i].CreatedAt.After(invites[j].CreatedAt) })

	return m.repository.Save(persistence.State{
		Version:    stateVersion,
		Users:      users,
		UserStates: userStates,
		Settings:   settings,
		Invites:    invites,
	})
}

func (m *Manager) runtimeFor(userID string) (*UserRuntime, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	user, ok := m.users[userID]
	if !ok || !user.Enabled {
		return nil, fmt.Errorf("user not found or disabled")
	}
	runtime, ok := m.runtimes[userID]
	if !ok {
		return nil, fmt.Errorf("user runtime not found")
	}
	return runtime, nil
}

func newUserRuntime(requests []parser.CaptureRequest, state persistence.UserState, minInterval time.Duration) *UserRuntime {
	client := charger.NewClientTemplateOnly(requests)
	if state.Cookie != "" {
		if err := client.UpdateCookie(state.Cookie); err != nil {
			// Keep runtime usable; the next refresh will surface the real auth error.
		}
	}
	client.RestoreDevices(state.DeviceIDs)

	refresh := state.Refresh
	if refresh.MinIntervalSeconds == 0 {
		refresh.MinIntervalSeconds = int(minInterval.Seconds())
	}
	store := store.NewDashboardStore(nil)
	if len(state.Piles) > 0 || refresh.Message != "" {
		store.Restore(state.Piles, refresh)
	} else {
		store.SetRefreshInfo(refresh)
	}

	runtime := &UserRuntime{
		store:               store,
		client:              client,
		stats:               state.Stats,
		minInterval:         minInterval,
		yybBinding:          cloneYYBBinding(state.YYBBinding),
		recoveryDiagnostics: cloneRecoveryDiagnostics(state.RecoveryDiagnostics),
	}
	if refresh.LastRemoteAt != nil {
		runtime.lastRemoteFetch = *refresh.LastRemoteAt
	}
	return runtime
}

func (r *UserRuntime) refresh(force bool) ([]model.Pile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.stats.RefreshRequests++
	now := time.Now()
	if !force && !r.lastRemoteFetch.IsZero() && now.Sub(r.lastRemoteFetch) < r.minInterval {
		next := r.lastRemoteFetch.Add(r.minInterval)
		r.stats.CachedRefreshes++
		r.store.SetRefreshInfo(model.RefreshInfo{
			LastRemoteAt:       &r.lastRemoteFetch,
			NextRemoteAt:       &next,
			MinIntervalSeconds: int(r.minInterval.Seconds()),
			Cached:             true,
			Message:            "刷新间隔内，已返回缓存数据",
		})
		return nil, nil
	}

	result := r.client.FetchPiles(force)
	if result.Attempted > 0 {
		r.lastRemoteFetch = time.Now()
		r.stats.RemoteFetches += result.Attempted
		r.stats.LastRemoteFetchAt = &r.lastRemoteFetch
	}
	if len(result.Piles) > 0 {
		r.store.MergeCapturePiles(result.Piles)
	}

	failed := 0
	for _, failure := range result.Failures {
		if !failure.Skipped {
			failed++
		}
	}
	info := model.RefreshInfo{
		NextRetryAt:        result.NextRetryAt,
		MinIntervalSeconds: int(r.minInterval.Seconds()),
		AttemptedDevices:   result.Attempted,
		SuccessfulDevices:  len(result.Piles),
		FailedDevices:      failed,
		SkippedDevices:     result.Skipped,
		Cached:             len(result.Piles) == 0,
		Partial:            len(result.Piles) > 0 && (failed > 0 || result.Skipped > 0),
	}
	if !r.lastRemoteFetch.IsZero() {
		lastRemoteAt := r.lastRemoteFetch
		nextRemoteAt := lastRemoteAt.Add(r.minInterval)
		info.LastRemoteAt = &lastRemoteAt
		info.NextRemoteAt = &nextRemoteAt
	}
	switch {
	case result.AuthExpired() && len(result.Piles) == 0:
		info.Message = "Cookie 可能已过期，请更新 Cookie 后重试"
	case len(result.Piles) > 0 && (failed > 0 || result.Skipped > 0):
		info.Message = fmt.Sprintf("已更新 %d 台，%d 台失败，%d 台退避中；失败设备保留上次数据", len(result.Piles), failed, result.Skipped)
	case len(result.Piles) > 0:
		info.Message = fmt.Sprintf("已更新 %d 台充电桩", len(result.Piles))
	case result.Skipped > 0 && failed == 0:
		info.Message = fmt.Sprintf("%d 台设备处于请求退避期，已返回缓存数据", result.Skipped)
	case failed > 0:
		info.Message = fmt.Sprintf("%d 台设备请求失败，已保留上次数据", failed)
	default:
		info.Message = "没有需要刷新的充电桩"
	}
	r.store.SetRefreshInfo(model.RefreshInfo{
		LastRemoteAt:       info.LastRemoteAt,
		NextRemoteAt:       info.NextRemoteAt,
		NextRetryAt:        info.NextRetryAt,
		MinIntervalSeconds: info.MinIntervalSeconds,
		AttemptedDevices:   info.AttemptedDevices,
		SuccessfulDevices:  info.SuccessfulDevices,
		FailedDevices:      info.FailedDevices,
		SkippedDevices:     info.SkippedDevices,
		Cached:             info.Cached,
		Partial:            info.Partial,
		Message:            info.Message,
	})
	if len(result.Piles) == 0 && failed > 0 {
		return nil, result.FirstError()
	}
	return append([]model.Pile(nil), result.Piles...), nil
}

func (r *UserRuntime) recordRequest() {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	r.stats.TotalRequests++
	r.stats.LastRequestAt = &now
}

func (r *UserRuntime) recordFailure(authFailure bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	r.stats.FailedRequests++
	r.stats.LastFailedAt = &now
	if authFailure {
		r.stats.AuthFailures++
		r.stats.LastAuthFailureAt = &now
	}
}

func (r *UserRuntime) recordRemoteOK(at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stats.LastRemoteOKAt = &at
}

func (r *UserRuntime) statsSnapshot() model.TrafficStats {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stats
}

func (r *UserRuntime) recoveryDiagnosticsSnapshot() []model.RecoveryDiagnostic {
	r.mu.Lock()
	defer r.mu.Unlock()
	return cloneRecoveryDiagnostics(r.recoveryDiagnostics)
}

func cloneRecoveryDiagnostics(items []model.RecoveryDiagnostic) []model.RecoveryDiagnostic {
	if len(items) == 0 {
		return []model.RecoveryDiagnostic{}
	}
	return append([]model.RecoveryDiagnostic(nil), items...)
}

func (r *UserRuntime) state() persistence.UserState {
	r.mu.Lock()
	defer r.mu.Unlock()
	snapshot := r.store.Snapshot()
	return persistence.UserState{
		Piles:               snapshot.Piles,
		Refresh:             snapshot.Refresh,
		DeviceIDs:           r.client.DeviceIDs(),
		Cookie:              r.client.Cookie(),
		Stats:               r.stats,
		YYBBinding:          cloneYYBBinding(r.yybBinding),
		RecoveryDiagnostics: cloneRecoveryDiagnostics(r.recoveryDiagnostics),
	}
}

func randomID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}
