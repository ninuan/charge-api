package runtime

import (
	"crypto/rand"
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

const stateVersion = 2

type Manager struct {
	mu          sync.RWMutex
	statePath   string
	requests    []parser.CaptureRequest
	minInterval time.Duration
	users       map[string]model.User
	runtimes    map[string]*UserRuntime
	initialPass string
}

type UserRuntime struct {
	mu              sync.Mutex
	store           *store.DashboardStore
	client          *charger.Client
	stats           model.TrafficStats
	lastRemoteFetch time.Time
	minInterval     time.Duration
}

func NewManager(statePath string, requests []parser.CaptureRequest, adminPassword string, minInterval time.Duration) (*Manager, error) {
	m := &Manager{
		statePath:   statePath,
		requests:    requests,
		minInterval: minInterval,
		users:       make(map[string]model.User),
		runtimes:    make(map[string]*UserRuntime),
	}

	state, hasState, err := persistence.Load(statePath)
	if err != nil {
		return nil, err
	}

	if hasState && len(state.Users) > 0 {
		for _, user := range state.Users {
			m.users[user.ID] = user
			userState := state.UserStates[user.ID]
			m.runtimes[user.ID] = newUserRuntime(requests, userState, minInterval)
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
	return m, nil
}

func (m *Manager) InitialAdminPassword() string {
	return m.initialPass
}

func (m *Manager) Authenticate(username string, password string) (model.CurrentUser, error) {
	username = strings.TrimSpace(username)
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, user := range m.users {
		if user.Username != username {
			continue
		}
		if !user.Enabled {
			return model.CurrentUser{}, fmt.Errorf("用户已被禁用")
		}
		if !auth.CheckPassword(password, user.PasswordHash) {
			return model.CurrentUser{}, fmt.Errorf("用户名或密码错误")
		}
		return publicUser(user), nil
	}
	return model.CurrentUser{}, fmt.Errorf("用户名或密码错误")
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

func (m *Manager) RegisterUser(username string, password string) (model.CurrentUser, error) {
	return m.CreateUser(model.UserCreateRequest{
		Username: username,
		Password: password,
		Role:     model.RoleUser,
	})
}

func (m *Manager) UpdateUser(id string, req model.UserUpdateRequest) (model.CurrentUser, error) {
	m.mu.Lock()
	user, ok := m.users[id]
	if !ok {
		m.mu.Unlock()
		return model.CurrentUser{}, fmt.Errorf("user not found")
	}

	if req.Password != nil && strings.TrimSpace(*req.Password) != "" {
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
	return runtime.store.Snapshot(), m.Save()
}

func (m *Manager) AddPile(userID string, req model.PileUpsertRequest) (model.Pile, error) {
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return model.Pile{}, err
	}
	runtime.recordRequest()
	if err := runtime.client.AddDevice(req.ID); err != nil {
		runtime.recordFailure(false)
		_ = m.Save()
		return model.Pile{}, err
	}
	if err := runtime.refresh(true); err != nil {
		runtime.client.RemoveDevice(req.ID)
		runtime.recordFailure(charger.IsAuthExpired(err))
		_ = m.Save()
		return model.Pile{}, err
	}
	for _, pile := range runtime.store.Snapshot().Piles {
		if pile.ID == req.ID {
			return pile, m.Save()
		}
	}
	return model.Pile{}, fmt.Errorf("device %s was not returned by remote API", req.ID)
}

func (m *Manager) DeletePile(userID string, id string) error {
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return err
	}
	runtime.recordRequest()
	if !runtime.store.DeletePile(id) {
		runtime.recordFailure(false)
		_ = m.Save()
		return fmt.Errorf("pile not found")
	}
	runtime.client.RemoveDevice(id)
	return m.Save()
}

func (m *Manager) Refresh(userID string, force bool) (model.DashboardSnapshot, error) {
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return model.DashboardSnapshot{}, err
	}
	runtime.recordRequest()
	if err := runtime.refresh(force); err != nil {
		runtime.recordFailure(charger.IsAuthExpired(err))
		_ = m.Save()
		return model.DashboardSnapshot{}, err
	}
	return runtime.store.Snapshot(), m.Save()
}

func (m *Manager) UpdateCookie(userID string, cookie string) (model.DashboardSnapshot, error) {
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return model.DashboardSnapshot{}, err
	}
	runtime.recordRequest()
	previous := runtime.client.Cookie()
	if err := runtime.client.UpdateCookie(cookie); err != nil {
		runtime.recordFailure(false)
		_ = m.Save()
		return model.DashboardSnapshot{}, err
	}
	if err := runtime.refresh(true); err != nil {
		_ = runtime.client.UpdateCookie(previous)
		runtime.recordFailure(charger.IsAuthExpired(err))
		_ = m.Save()
		return model.DashboardSnapshot{}, err
	}
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
	m.mu.RLock()
	users := make([]model.User, 0, len(m.users))
	userStates := make(map[string]persistence.UserState, len(m.runtimes))
	for _, user := range m.users {
		users = append(users, user)
	}
	sort.Slice(users, func(i, j int) bool {
		return users[i].Username < users[j].Username
	})
	for userID, runtime := range m.runtimes {
		userStates[userID] = runtime.state()
	}
	m.mu.RUnlock()

	return persistence.Save(m.statePath, persistence.State{
		Version:    stateVersion,
		Users:      users,
		UserStates: userStates,
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

	piles, err := r.client.FetchPiles()
	if err != nil {
		if charger.IsAuthExpired(err) {
			r.store.SetRefreshInfo(model.RefreshInfo{
				MinIntervalSeconds: int(r.minInterval.Seconds()),
				Cached:             true,
				Message:            "Cookie 可能已过期，请更新 Cookie 后重试",
			})
		}
		return err
	}

	r.store.ReplaceCapturePiles(piles)
	r.lastRemoteFetch = time.Now()
	r.stats.RemoteFetches++
	r.stats.LastRemoteFetchAt = &r.lastRemoteFetch
	next := r.lastRemoteFetch.Add(r.minInterval)
	r.store.SetRefreshInfo(model.RefreshInfo{
		LastRemoteAt:       &r.lastRemoteFetch,
		NextRemoteAt:       &next,
		MinIntervalSeconds: int(r.minInterval.Seconds()),
		Cached:             false,
		Message:            "已请求远端接口",
	})
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
	if username == "" {
		return model.User{}, fmt.Errorf("username is required")
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

func publicUser(user model.User) model.CurrentUser {
	return model.CurrentUser{
		ID:        user.ID,
		Username:  user.Username,
		Role:      user.Role,
		Enabled:   user.Enabled,
		CreatedAt: user.CreatedAt,
	}
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
