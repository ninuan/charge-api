package runtime

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"charge-dashboard/internal/auth"
	"charge-dashboard/internal/charger"
	"charge-dashboard/internal/model"
	"charge-dashboard/internal/parser"
	"charge-dashboard/internal/persistence"
	"charge-dashboard/internal/store"
)

const (
	stateVersion       = 2
	defaultDeviceLimit = 10
	maxDevicesPerUser  = defaultDeviceLimit
)

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
	mu              sync.Mutex
	store           *store.DashboardStore
	client          *charger.Client
	stats           model.TrafficStats
	lastRemoteFetch time.Time
	minInterval     time.Duration
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
			StatsRetentionDays: 90,
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
		if state.Settings.DefaultDeviceLimit > 0 {
			m.settings = state.Settings
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
		if m.migrated {
			if err := m.Save(); err != nil {
				return nil, err
			}
			if err := persistence.ArchiveMigratedJSON(legacyJSONPath, state); err != nil {
				return nil, err
			}
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
	return m, nil
}

func (m *Manager) InitialAdminPassword() string {
	return m.initialPass
}

func (m *Manager) MigratedLegacyJSON() bool {
	return m.migrated
}

func (m *Manager) Authenticate(username string, password string) (model.CurrentUser, error) {
	username = strings.TrimSpace(username)
	m.mu.RLock()
	var matched model.User
	found := false
	for _, user := range m.users {
		if user.Username != username {
			continue
		}
		matched = user
		found = true
		break
	}
	m.mu.RUnlock()

	if !found {
		return model.CurrentUser{}, fmt.Errorf("用户名或密码错误")
	}
	if !matched.Enabled {
		return model.CurrentUser{}, fmt.Errorf("用户已被禁用")
	}

	valid, needsUpgrade := auth.VerifyPassword(password, matched.PasswordHash)
	if !valid {
		return model.CurrentUser{}, fmt.Errorf("用户名或密码错误")
	}
	if needsUpgrade {
		hash, err := auth.HashPassword(password)
		if err != nil {
			return model.CurrentUser{}, err
		}
		m.mu.Lock()
		current, ok := m.users[matched.ID]
		if ok {
			current.PasswordHash = hash
			current.UpdatedAt = time.Now()
			m.users[matched.ID] = current
			matched = current
		}
		m.mu.Unlock()
		if err := m.Save(); err != nil {
			return model.CurrentUser{}, err
		}
	}
	return publicUser(matched), nil
}

func (m *Manager) User(id string) (model.CurrentUser, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	user, ok := m.users[id]
	if !ok || !user.Enabled {
		return model.CurrentUser{}, false
	}
	return publicUser(user), true
}

func (m *Manager) ListUsers() []model.AdminUserSummary {
	m.mu.RLock()
	userIDs := make([]string, 0, len(m.users))
	for id := range m.users {
		userIDs = append(userIDs, id)
	}
	sort.Slice(userIDs, func(i, j int) bool {
		return m.users[userIDs[i]].Username < m.users[userIDs[j]].Username
	})
	users := make([]model.User, 0, len(userIDs))
	runtimes := make([]*UserRuntime, 0, len(userIDs))
	for _, id := range userIDs {
		users = append(users, m.users[id])
		runtimes = append(runtimes, m.runtimes[id])
	}
	m.mu.RUnlock()

	summaries := make([]model.AdminUserSummary, 0, len(users))
	for i, user := range users {
		runtime := runtimes[i]
		summary := model.AdminUserSummary{User: publicUser(user)}
		if runtime != nil {
			snapshot := runtime.store.Snapshot()
			summary.Stats = runtime.statsSnapshot()
			summary.Dashboard = snapshot.Statistics
			summary.DeviceIDs = runtime.client.DeviceIDs()
			summary.HasCookie = runtime.client.Cookie() != ""
			summary.LastRefresh = snapshot.Refresh
		}
		summaries = append(summaries, summary)
	}
	return summaries
}

func (m *Manager) CreateUser(req model.UserCreateRequest) (model.CurrentUser, error) {
	role := req.Role
	if role == "" {
		role = model.RoleUser
	}
	if role != model.RoleAdmin && role != model.RoleUser {
		return model.CurrentUser{}, fmt.Errorf("invalid role")
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	user, err := newUser(req.Username, req.Password, role, enabled)
	if err != nil {
		return model.CurrentUser{}, err
	}
	m.mu.RLock()
	user.DeviceLimit = m.settings.DefaultDeviceLimit
	user.RefreshEnabled = m.settings.DefaultRefreshEnabled
	m.mu.RUnlock()
	if req.DeviceLimit != nil {
		user.DeviceLimit = *req.DeviceLimit
	}
	if req.RefreshEnabled != nil {
		user.RefreshEnabled = *req.RefreshEnabled
	}
	if user.DeviceLimit < 1 || user.DeviceLimit > 100 {
		return model.CurrentUser{}, fmt.Errorf("设备额度需要在 1 到 100 之间")
	}

	m.mu.Lock()
	for _, existing := range m.users {
		if existing.Username == user.Username {
			m.mu.Unlock()
			return model.CurrentUser{}, fmt.Errorf("username already exists")
		}
	}
	m.users[user.ID] = user
	m.runtimes[user.ID] = newUserRuntime(m.requests, persistence.UserState{}, m.minInterval)
	m.mu.Unlock()

	return publicUser(user), m.Save()
}

func (m *Manager) RegisterUser(username string, password string, inviteCode string) (model.CurrentUser, error) {
	m.mu.Lock()
	if !m.settings.OpenRegistration && !m.settings.InviteRequired {
		m.mu.Unlock()
		return model.CurrentUser{}, fmt.Errorf("当前未开放注册")
	}
	var usedInviteID string
	inviteCode = strings.TrimSpace(inviteCode)
	if inviteCode != "" {
		if !m.settings.InviteRequired {
			m.mu.Unlock()
			return model.CurrentUser{}, fmt.Errorf("当前未开放邀请码注册")
		}
		now := time.Now()
		for id, invite := range m.invites {
			if invite.Enabled && invite.Code == inviteCode &&
				(invite.ExpiresAt == nil || invite.ExpiresAt.After(now)) {
				usedInviteID = id
				break
			}
		}
		if usedInviteID == "" {
			m.mu.Unlock()
			return model.CurrentUser{}, fmt.Errorf("邀请码无效或已过期")
		}
	} else if !m.settings.OpenRegistration {
		m.mu.Unlock()
		return model.CurrentUser{}, fmt.Errorf("请输入有效邀请码")
	}
	m.mu.Unlock()
	user, err := m.CreateUser(model.UserCreateRequest{
		Username: username,
		Password: password,
		Role:     model.RoleUser,
	})
	if err != nil {
		return model.CurrentUser{}, err
	}
	if usedInviteID != "" {
		m.mu.Lock()
		invite := m.invites[usedInviteID]
		invite.UsedCount++
		m.invites[usedInviteID] = invite
		m.mu.Unlock()
		if err := m.Save(); err != nil {
			return model.CurrentUser{}, err
		}
	}
	return user, nil
}

func (m *Manager) UpdateUser(id string, req model.UserUpdateRequest) (model.CurrentUser, error) {
	m.mu.Lock()
	user, ok := m.users[id]
	if !ok {
		m.mu.Unlock()
		return model.CurrentUser{}, fmt.Errorf("user not found")
	}

	if req.Password != nil && strings.TrimSpace(*req.Password) != "" {
		if err := validateNewPassword(*req.Password); err != nil {
			m.mu.Unlock()
			return model.CurrentUser{}, err
		}
		hash, err := auth.HashPassword(*req.Password)
		if err != nil {
			m.mu.Unlock()
			return model.CurrentUser{}, err
		}
		user.PasswordHash = hash
	}
	if req.Role != nil {
		if *req.Role != model.RoleAdmin && *req.Role != model.RoleUser {
			m.mu.Unlock()
			return model.CurrentUser{}, fmt.Errorf("invalid role")
		}
		user.Role = *req.Role
	}
	if req.Enabled != nil {
		user.Enabled = *req.Enabled
	}
	if req.DeviceLimit != nil {
		if *req.DeviceLimit < 1 || *req.DeviceLimit > 100 {
			m.mu.Unlock()
			return model.CurrentUser{}, fmt.Errorf("设备额度需要在 1 到 100 之间")
		}
		user.DeviceLimit = *req.DeviceLimit
	}
	if req.RefreshEnabled != nil {
		user.RefreshEnabled = *req.RefreshEnabled
	}
	activeAdmins := 0
	for userID, existing := range m.users {
		if userID == id {
			existing = user
		}
		if existing.Role == model.RoleAdmin && existing.Enabled {
			activeAdmins++
		}
	}
	if activeAdmins == 0 {
		m.mu.Unlock()
		return model.CurrentUser{}, fmt.Errorf("至少保留一个可用管理员")
	}
	user.UpdatedAt = time.Now()
	m.users[id] = user
	m.mu.Unlock()

	return publicUser(user), m.Save()
}

func (m *Manager) DeleteUser(id string) error {
	m.mu.Lock()
	if _, ok := m.users[id]; !ok {
		m.mu.Unlock()
		return fmt.Errorf("user not found")
	}
	adminCount := 0
	for userID, user := range m.users {
		if userID != id && user.Role == model.RoleAdmin && user.Enabled {
			adminCount++
		}
	}
	if m.users[id].Role == model.RoleAdmin && adminCount == 0 {
		m.mu.Unlock()
		return fmt.Errorf("至少保留一个可用管理员")
	}
	delete(m.users, id)
	delete(m.runtimes, id)
	m.mu.Unlock()

	return m.Save()
}

func (m *Manager) Snapshot(userID string) (model.DashboardSnapshot, error) {
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return model.DashboardSnapshot{}, err
	}
	runtime.recordRequest()
	m.recordMetric(userID, "request")
	return runtime.store.Snapshot(), m.Save()
}

func (m *Manager) AddPile(userID string, req model.PileUpsertRequest) (model.Pile, error) {
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return model.Pile{}, err
	}
	runtime.recordRequest()
	m.recordMetric(userID, "request")
	user, _ := m.User(userID)
	if !user.RefreshEnabled {
		return model.Pile{}, fmt.Errorf("管理员已暂停此账户的远端刷新，暂时无法验证新设备")
	}
	if err := runtime.client.AddDeviceWithLimit(req.ID, user.DeviceLimit); err != nil {
		runtime.recordFailure(false)
		_ = m.Save()
		if charger.IsDeviceLimit(err) {
			return model.Pile{}, fmt.Errorf("当前账户最多添加 %d 台充电桩", user.DeviceLimit)
		}
		return model.Pile{}, err
	}
	if err := runtime.refresh(true); err != nil {
		m.recordMetric(userID, "cookie_error")
		runtime.client.RemoveDevice(req.ID)
		runtime.recordFailure(charger.IsAuthExpired(err))
		_ = m.Save()
		return model.Pile{}, err
	}
	m.recordMetric(userID, "remote")
	m.recordMetric(userID, "remote_ok")
	for _, pile := range runtime.store.Snapshot().Piles {
		if pile.ID == req.ID {
			updated, _ := runtime.store.UpdatePile(req.ID, req.Name, req.Address, pile.SortOrder)
			return updated, m.Save()
		}
	}
	runtime.client.RemoveDevice(req.ID)
	runtime.recordFailure(false)
	_ = m.Save()
	return model.Pile{}, fmt.Errorf("device %s was not returned by remote API", req.ID)
}

func (m *Manager) DeletePile(userID string, id string) error {
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return err
	}
	runtime.recordRequest()
	m.recordMetric(userID, "request")
	if !runtime.store.DeletePile(id) {
		runtime.recordFailure(false)
		_ = m.Save()
		return fmt.Errorf("pile not found")
	}
	runtime.client.RemoveDevice(id)
	return m.Save()
}

func (m *Manager) UpdatePile(userID, id, name, address string, sortOrder int) (model.Pile, error) {
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return model.Pile{}, err
	}
	runtime.recordRequest()
	m.recordMetric(userID, "request")
	pile, ok := runtime.store.UpdatePile(id, strings.TrimSpace(name), strings.TrimSpace(address), sortOrder)
	if !ok {
		return model.Pile{}, fmt.Errorf("pile not found")
	}
	return pile, m.Save()
}

func (m *Manager) Refresh(userID string, force bool) (model.DashboardSnapshot, error) {
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return model.DashboardSnapshot{}, err
	}
	runtime.recordRequest()
	m.recordMetric(userID, "request")
	user, _ := m.User(userID)
	if !user.RefreshEnabled {
		m.recordMetric(userID, "cache")
		snapshot := runtime.store.Snapshot()
		snapshot.Refresh.Cached = true
		snapshot.Refresh.Message = "管理员已暂停此账户的远端刷新，当前展示缓存数据"
		return snapshot, nil
	}
	if err := runtime.refresh(force); err != nil {
		m.recordMetric(userID, "cookie_error")
		runtime.recordFailure(charger.IsAuthExpired(err))
		_ = m.Save()
		return model.DashboardSnapshot{}, err
	}
	snapshot := runtime.store.Snapshot()
	if snapshot.Refresh.Cached {
		m.recordMetric(userID, "cache")
	} else {
		m.recordMetric(userID, "remote")
		if snapshot.Refresh.SuccessfulDevices > 0 {
			m.recordMetric(userID, "remote_ok")
		}
	}
	return snapshot, m.Save()
}

func (m *Manager) UpdateCookie(userID string, cookie string) (model.DashboardSnapshot, error) {
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return model.DashboardSnapshot{}, err
	}
	runtime.recordRequest()
	m.recordMetric(userID, "request")
	previous := runtime.client.Cookie()
	if err := runtime.client.UpdateCookie(cookie); err != nil {
		runtime.recordFailure(false)
		_ = m.Save()
		return model.DashboardSnapshot{}, err
	}
	user, _ := m.User(userID)
	if !user.RefreshEnabled {
		snapshot := runtime.store.Snapshot()
		snapshot.Refresh.Cached = true
		snapshot.Refresh.Message = "Cookie 已保存；远端刷新当前已暂停，继续展示缓存数据"
		runtime.store.SetRefreshInfo(snapshot.Refresh)
		if err := m.Save(); err != nil {
			return model.DashboardSnapshot{}, err
		}
		return snapshot, nil
	}
	if err := runtime.refresh(true); err != nil {
		_ = runtime.client.UpdateCookie(previous)
		runtime.recordFailure(charger.IsAuthExpired(err))
		_ = m.Save()
		return model.DashboardSnapshot{}, err
	}
	m.recordMetric(userID, "remote")
	m.recordMetric(userID, "remote_ok")
	return runtime.store.Snapshot(), m.Save()
}

func (m *Manager) Subscribe(userID string) (chan model.DashboardSnapshot, error) {
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return nil, err
	}
	runtime.recordRequest()
	return runtime.store.Subscribe(), nil
}

func (m *Manager) Unsubscribe(userID string, ch chan model.DashboardSnapshot) {
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return
	}
	runtime.store.Unsubscribe(ch)
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
		store:       store,
		client:      client,
		stats:       state.Stats,
		minInterval: minInterval,
	}
	if refresh.LastRemoteAt != nil {
		runtime.lastRemoteFetch = *refresh.LastRemoteAt
	}
	return runtime
}

func (r *UserRuntime) refresh(force bool) error {
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
		return nil
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
		return result.FirstError()
	}
	return nil
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

func (r *UserRuntime) statsSnapshot() model.TrafficStats {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stats
}

func (r *UserRuntime) state() persistence.UserState {
	r.mu.Lock()
	defer r.mu.Unlock()
	snapshot := r.store.Snapshot()
	return persistence.UserState{
		Piles:     snapshot.Piles,
		Refresh:   snapshot.Refresh,
		DeviceIDs: r.client.DeviceIDs(),
		Cookie:    r.client.Cookie(),
		Stats:     r.stats,
	}
}

func newUser(username string, password string, role model.UserRole, enabled bool) (model.User, error) {
	username = strings.TrimSpace(username)
	if len(username) < 3 || len(username) > 64 {
		return model.User{}, fmt.Errorf("用户名长度需要在 3 到 64 个字符之间")
	}
	if err := validateNewPassword(password); err != nil {
		return model.User{}, err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return model.User{}, err
	}
	now := time.Now()
	return model.User{
		ID:           randomID("usr"),
		Username:     username,
		PasswordHash: hash,
		Role:         role,
		Enabled:      enabled,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

func validateNewPassword(password string) error {
	if len(password) < 8 || len(password) > 128 {
		return fmt.Errorf("密码长度需要在 8 到 128 个字符之间")
	}
	return nil
}

func publicUser(user model.User) model.CurrentUser {
	return model.CurrentUser{
		ID:             user.ID,
		Username:       user.Username,
		Role:           user.Role,
		Enabled:        user.Enabled,
		CreatedAt:      user.CreatedAt,
		DeviceLimit:    user.DeviceLimit,
		RefreshEnabled: user.RefreshEnabled,
	}
}

func (m *Manager) ChangePassword(userID, currentPassword, newPassword string) error {
	m.mu.Lock()
	user, ok := m.users[userID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("用户不存在")
	}
	valid, _ := auth.VerifyPassword(currentPassword, user.PasswordHash)
	if !valid {
		m.mu.Unlock()
		return fmt.Errorf("当前密码错误")
	}
	if err := validateNewPassword(newPassword); err != nil {
		m.mu.Unlock()
		return err
	}
	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		m.mu.Unlock()
		return err
	}
	user.PasswordHash = hash
	user.UpdatedAt = time.Now()
	m.users[userID] = user
	m.mu.Unlock()
	return m.Save()
}

func (m *Manager) Settings() model.RegistrationSettings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.settings
}

func (m *Manager) UpdateSettings(settings model.RegistrationSettings) error {
	if settings.DefaultDeviceLimit < 1 || settings.DefaultDeviceLimit > 100 {
		return fmt.Errorf("默认设备额度需要在 1 到 100 之间")
	}
	if settings.StatsRetentionDays < 1 || settings.StatsRetentionDays > 365 {
		return fmt.Errorf("统计保留天数需要在 1 到 365 之间")
	}
	m.mu.Lock()
	m.settings = settings
	m.mu.Unlock()
	_ = m.repository.PruneMetrics(time.Now().AddDate(0, 0, -settings.StatsRetentionDays))
	return m.Save()
}

func (m *Manager) InviteCodes() []model.InviteCode {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]model.InviteCode, 0, len(m.invites))
	for _, invite := range m.invites {
		result = append(result, invite)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result
}

func (m *Manager) CreateInvite(code string, expiresAt *time.Time) (model.InviteCode, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		var err error
		code, err = randomInviteCode()
		if err != nil {
			return model.InviteCode{}, err
		}
	}
	if len(code) < 4 || len(code) > 64 {
		return model.InviteCode{}, fmt.Errorf("邀请码长度需要在 4 到 64 个字符之间")
	}
	m.mu.Lock()
	for _, existing := range m.invites {
		if existing.Code == code {
			m.mu.Unlock()
			return model.InviteCode{}, fmt.Errorf("邀请码已存在")
		}
	}
	invite := model.InviteCode{ID: randomID("inv"), Code: code, Enabled: true, CreatedAt: time.Now(), ExpiresAt: expiresAt}
	m.invites[invite.ID] = invite
	m.mu.Unlock()
	return invite, m.Save()
}

func randomInviteCode() (string, error) {
	token := make([]byte, 8)
	if _, err := rand.Read(token); err != nil {
		return "", fmt.Errorf("generate invite code: %w", err)
	}
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(token)
	return "CHG-" + encoded, nil
}

func (m *Manager) DeleteInvite(id string) error {
	m.mu.Lock()
	if _, ok := m.invites[id]; !ok {
		m.mu.Unlock()
		return fmt.Errorf("邀请码不存在")
	}
	delete(m.invites, id)
	m.mu.Unlock()
	return m.Save()
}

func (m *Manager) AdminStats() model.AdminStats {
	hourly, _ := m.repository.MetricSeries(time.Now().Add(-24*time.Hour), 3600)
	daily, _ := m.repository.MetricSeries(time.Now().AddDate(0, 0, -30), 86400)
	users := m.ListUsers()
	var exceptions []model.SystemException
	now := time.Now()
	for _, summary := range users {
		if summary.User.Role != model.RoleUser {
			continue
		}
		if !summary.HasCookie {
			exceptions = append(exceptions, model.SystemException{ID: "cookie-" + summary.User.ID, UserID: summary.User.ID, Username: summary.User.Username, Type: "cookie", Level: "warning", Message: "尚未配置 Cookie", Time: now})
		}
		if summary.Stats.AuthFailures > 0 {
			at := now
			if summary.Stats.LastAuthFailureAt != nil {
				at = *summary.Stats.LastAuthFailureAt
			}
			exceptions = append(exceptions, model.SystemException{ID: "auth-" + summary.User.ID, UserID: summary.User.ID, Username: summary.User.Username, Type: "cookie_expired", Level: "critical", Message: "远端鉴权失败，Cookie 可能已失效", Time: at})
		}
		if summary.LastRefresh.FailedDevices > 0 {
			exceptions = append(exceptions, model.SystemException{ID: "refresh-" + summary.User.ID, UserID: summary.User.ID, Username: summary.User.Username, Type: "refresh", Level: "warning", Message: fmt.Sprintf("%d 台设备刷新失败", summary.LastRefresh.FailedDevices), Time: now})
		}
		if summary.LastRefresh.LastRemoteAt != nil && now.Sub(*summary.LastRefresh.LastRemoteAt) > 24*time.Hour {
			exceptions = append(exceptions, model.SystemException{ID: "stale-" + summary.User.ID, UserID: summary.User.ID, Username: summary.User.Username, Type: "stale", Level: "warning", Message: "设备数据已超过 24 小时未更新", Time: *summary.LastRefresh.LastRemoteAt})
		}
		if summary.Dashboard.OfflinePorts > 0 {
			exceptions = append(exceptions, model.SystemException{ID: "offline-" + summary.User.ID, UserID: summary.User.ID, Username: summary.User.Username, Type: "offline", Level: "warning", Message: fmt.Sprintf("%d 个充电口处于离线状态", summary.Dashboard.OfflinePorts), Time: now})
		}
	}
	return model.AdminStats{Users: users, Hourly: hourly, Daily: daily, Exceptions: exceptions}
}

func (m *Manager) recordMetric(userID, kind string) {
	_ = m.repository.RecordMetric(userID, kind, time.Now())
}

func randomID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}

func randomPassword() (string, error) {
	token := make([]byte, 18)
	if _, err := rand.Read(token); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(token), nil
}
