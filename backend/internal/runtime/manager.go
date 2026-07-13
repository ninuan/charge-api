package runtime

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
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
	"charge-dashboard/internal/yyb"
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
	stateVersion           = 3
	defaultDeviceLimit     = 10
	maxDevicesPerUser      = defaultDeviceLimit
	maxRecoveryDiagnostics = 20
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

func (m *Manager) Ping(ctx context.Context) error {
	return m.repository.Ping(ctx)
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

func (m *Manager) AcknowledgeUsageGuide(userID string) (model.CurrentUser, error) {
	m.mu.Lock()
	user, ok := m.users[userID]
	if !ok || !user.Enabled {
		m.mu.Unlock()
		return model.CurrentUser{}, fmt.Errorf("user not found or disabled")
	}
	if user.UsageGuideAckAt == nil {
		now := time.Now()
		user.UsageGuideAckAt = &now
		user.UpdatedAt = now
		m.users[userID] = user
	}
	m.mu.Unlock()
	return publicUser(user), m.Save()
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
		summary := model.AdminUserSummary{
			User:              publicUser(user),
			DeviceIDs:         []string{},
			Credential:        model.CredentialSummary{State: model.CredentialUnbound},
			SnapshotUpdatedAt: user.CreatedAt,
		}
		if runtime != nil {
			snapshot := runtime.store.Snapshot()
			summary.Stats = runtime.statsSnapshot()
			summary.Dashboard = snapshot.Statistics
			summary.DeviceIDs = runtime.client.DeviceIDs()
			summary.Credential = credentialSummary(runtime, len(summary.DeviceIDs))
			summary.HasCookie = summary.Credential.HasCredential
			summary.SnapshotUpdatedAt = latestSnapshotDataTime(snapshot, user.CreatedAt)
			summary.LastRefresh = snapshot.Refresh
			summary.RecoveryDiagnostics = runtime.recoveryDiagnosticsSnapshot()
		}
		summaries = append(summaries, summary)
	}
	return summaries
}

func (m *Manager) ListUsersPage(query model.AdminUserListQuery) model.AdminUserPage {
	query = normalizeAdminUserListQuery(query)
	items := make([]model.AdminUserSummary, 0)
	for _, summary := range m.ListUsers() {
		if !matchesAdminUserListQuery(summary, query) {
			continue
		}
		items = append(items, summary)
	}

	total := len(items)
	totalPages := int(math.Ceil(float64(total) / float64(query.PageSize)))
	if totalPages == 0 {
		totalPages = 1
	}
	if query.Page > totalPages {
		query.Page = totalPages
	}
	start := (query.Page - 1) * query.PageSize
	end := start + query.PageSize
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	pageItems := items[start:end]
	if pageItems == nil {
		pageItems = []model.AdminUserSummary{}
	}
	return model.AdminUserPage{
		Items:      pageItems,
		Page:       query.Page,
		PageSize:   query.PageSize,
		Total:      total,
		TotalPages: totalPages,
	}
}

func normalizeAdminUserListQuery(query model.AdminUserListQuery) model.AdminUserListQuery {
	if query.Page < 1 {
		query.Page = 1
	}
	if query.PageSize < 1 {
		query.PageSize = 15
	}
	if query.PageSize > 100 {
		query.PageSize = 100
	}
	query.Search = strings.TrimSpace(strings.ToLower(query.Search))
	return query
}

func matchesAdminUserListQuery(summary model.AdminUserSummary, query model.AdminUserListQuery) bool {
	if query.Search != "" && !strings.Contains(strings.ToLower(summary.User.Username), query.Search) {
		return false
	}
	switch query.Account {
	case "enabled":
		if !summary.User.Enabled {
			return false
		}
	case "disabled":
		if summary.User.Enabled {
			return false
		}
	}
	if query.Credential != "" && query.Credential != "all" && string(summary.Credential.State) != query.Credential {
		return false
	}
	switch query.Health {
	case "healthy":
		if hasAdminUserRisk(summary) {
			return false
		}
	case "risk":
		if !hasAdminUserRisk(summary) {
			return false
		}
	}
	return true
}

func hasAdminUserRisk(summary model.AdminUserSummary) bool {
	hasCredentialRisk := len(summary.DeviceIDs) > 0 && (summary.Credential.State == model.CredentialUnbound || summary.Credential.State == model.CredentialSyncFailed || summary.Credential.State == model.CredentialExpired)
	return !summary.User.Enabled || hasCredentialRisk || hasActiveAuthFailure(summary.Stats) || summary.LastRefresh.FailedDevices > 0 || summary.Dashboard.OfflinePorts > 0
}

func credentialSummary(runtime *UserRuntime, deviceCount int) model.CredentialSummary {
	if runtime == nil {
		return model.CredentialSummary{State: model.CredentialUnbound}
	}
	runtime.mu.Lock()
	binding := cloneYYBBinding(runtime.yybBinding)
	runtime.mu.Unlock()

	hasCookie := strings.TrimSpace(runtime.client.Cookie()) != ""
	result := model.CredentialSummary{
		Bound:         binding != nil && binding.Ref != "",
		HasCredential: hasCookie,
	}
	if binding != nil {
		result.LastCheckedAt = binding.LastCheckedAt
	}
	switch {
	case binding != nil && binding.Status == "expired":
		result.State = model.CredentialExpired
	case binding != nil && binding.LastError != "":
		result.State = model.CredentialSyncFailed
	case hasCookie:
		result.State = model.CredentialHealthy
	case binding != nil && deviceCount == 0:
		result.State = model.CredentialWaitingDevice
	case binding != nil:
		result.State = model.CredentialSyncFailed
	default:
		result.State = model.CredentialUnbound
	}
	return result
}

func latestSnapshotDataTime(snapshot model.DashboardSnapshot, fallback time.Time) time.Time {
	latest := fallback
	if snapshot.Refresh.LastRemoteAt != nil && snapshot.Refresh.LastRemoteAt.After(latest) {
		latest = *snapshot.Refresh.LastRemoteAt
	}
	for _, pile := range snapshot.Piles {
		if pile.UpdatedAt.After(latest) {
			latest = pile.UpdatedAt
		}
		for _, port := range pile.Ports {
			if port.UpdatedAt.After(latest) {
				latest = port.UpdatedAt
			}
		}
	}
	return latest
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

func (m *Manager) SaveYYBBinding(userID string, account yyb.YYBAccount) (*model.YYBBinding, error) {
	now := time.Now().UTC()
	binding := &model.YYBBinding{
		OpenID:        strings.TrimSpace(account.OpenID),
		Ref:           strings.TrimSpace(account.Ref),
		Nickname:      strings.TrimSpace(account.Nickname),
		Avatar:        strings.TrimSpace(account.Avatar),
		Status:        "alive",
		BoundAt:       now,
		LastCheckedAt: nil,
		LastError:     "",
	}
	if binding.Ref == "" {
		return nil, fmt.Errorf("yyb account ref is required")
	}
	if binding.OpenID == "" {
		return nil, fmt.Errorf("yyb account openid is required")
	}
	if err := m.SetYYBBinding(userID, binding); err != nil {
		return nil, err
	}
	return cloneYYBBinding(binding), nil
}

func (m *Manager) ClearYYBBinding(userID string) error {
	return m.SetYYBBinding(userID, nil)
}

func (m *Manager) SyncCookieFromYYB(userID string, deviceID string, yybClient YYBCodeClient, moceleClient MoceleCookieClient) (model.DashboardSnapshot, error) {
	binding, err := m.YYBBinding(userID)
	if err != nil {
		return model.DashboardSnapshot{}, err
	}
	if binding == nil || binding.Ref == "" {
		m.recordRecoveryDiagnostic(userID, recoveryDiagnostic("binding_missing", deviceID, 0))
		return model.DashboardSnapshot{}, fmt.Errorf("yyb binding is required")
	}
	ctx := context.Background()
	code, err := yybClient.GetCode(ctx, binding.Ref, moceleAppID)
	if err != nil {
		m.recordRecoveryDiagnostic(userID, recoveryDiagnosticWithError("yyb_get_code_failed", deviceID, err))
		if refreshErr := yybClient.RefreshAccount(ctx, binding.Ref); refreshErr != nil {
			m.recordRecoveryDiagnostic(userID, recoveryDiagnosticWithError("yyb_account_refresh_failed", deviceID, refreshErr))
			err = fmt.Errorf("get code failed: %v; refresh failed: %w", err, refreshErr)
			m.markYYBBindingExpired(userID, binding, err)
			return model.DashboardSnapshot{}, err
		}
		m.recordRecoveryDiagnostic(userID, recoveryDiagnostic("yyb_account_refresh_succeeded", deviceID, 0))
		code, err = yybClient.GetCode(ctx, binding.Ref, moceleAppID)
		if err != nil {
			m.recordRecoveryDiagnostic(userID, recoveryDiagnosticWithError("yyb_get_code_retry_failed", deviceID, err))
			m.markYYBBindingExpired(userID, binding, err)
			return model.DashboardSnapshot{}, err
		}
	}
	m.recordRecoveryDiagnostic(userID, recoveryDiagnostic("yyb_get_code_succeeded", deviceID, 0))
	cookieResult, err := moceleClient.ExchangeCode(ctx, deviceID, code)
	if err != nil {
		m.recordRecoveryDiagnostic(userID, recoveryDiagnosticWithError(moceleDiagnosticCode(err), deviceID, err))
		m.markYYBBindingError(userID, binding, err)
		return model.DashboardSnapshot{}, err
	}
	m.recordRecoveryDiagnostic(userID, recoveryDiagnostic("mocele_autologin_succeeded", deviceID, 0))
	now := time.Now().UTC()
	binding.Status = "alive"
	binding.LastError = ""
	binding.LastCheckedAt = &now
	if err := m.SetYYBBinding(userID, binding); err != nil {
		return model.DashboardSnapshot{}, err
	}
	snapshot, err := m.UpdateCookie(userID, cookieResult.Cookie)
	if err != nil {
		m.recordRecoveryDiagnostic(userID, recoveryDiagnosticWithError("new_cookie_validation_failed", deviceID, err))
		m.markYYBBindingError(userID, binding, err)
		return model.DashboardSnapshot{}, err
	}
	return snapshot, nil
}

func (m *Manager) markYYBBindingExpired(userID string, binding *model.YYBBinding, cause error) {
	binding.Status = "expired"
	m.markYYBBindingError(userID, binding, cause)
}

func (m *Manager) markYYBBindingError(userID string, binding *model.YYBBinding, cause error) {
	now := time.Now().UTC()
	binding.LastCheckedAt = &now
	if cause != nil {
		binding.LastError = cause.Error()
	}
	_ = m.SetYYBBinding(userID, binding)
}

func (m *Manager) YYBBinding(userID string) (*model.YYBBinding, error) {
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return nil, err
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return cloneYYBBinding(runtime.yybBinding), nil
}

func (m *Manager) SetYYBBinding(userID string, binding *model.YYBBinding) error {
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return err
	}
	runtime.mu.Lock()
	runtime.yybBinding = cloneYYBBinding(binding)
	runtime.mu.Unlock()
	return m.Save()
}

func cloneYYBBinding(binding *model.YYBBinding) *model.YYBBinding {
	if binding == nil {
		return nil
	}
	clone := *binding
	if binding.LastCheckedAt != nil {
		value := *binding.LastCheckedAt
		clone.LastCheckedAt = &value
	}
	return &clone
}

func (m *Manager) FirstDeviceID(userID string) (string, bool, error) {
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return "", false, err
	}
	ids := runtime.client.DeviceIDs()
	if len(ids) == 0 {
		return "", false, nil
	}
	return ids[0], true, nil
}

func (m *Manager) RecoveryDiagnostics(userID string) ([]model.RecoveryDiagnostic, error) {
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return nil, err
	}
	return runtime.recoveryDiagnosticsSnapshot(), nil
}

func (m *Manager) recordRecoveryDiagnostic(userID string, diagnostic model.RecoveryDiagnostic) {
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return
	}
	// Callers may carry an upstream error. Normalize every displayable field here
	// so a future call site cannot accidentally persist a cookie, code, ref, or
	// raw response body.
	diagnostic.Message = recoveryDiagnosticMessage(diagnostic.Code)
	diagnostic.DeviceSuffix = deviceSuffix(diagnostic.DeviceSuffix)
	if diagnostic.StatusCode < 100 || diagnostic.StatusCode > 599 {
		diagnostic.StatusCode = 0
	}
	if diagnostic.At.IsZero() {
		diagnostic.At = time.Now().UTC()
	}
	runtime.mu.Lock()
	runtime.recoveryDiagnostics = append(runtime.recoveryDiagnostics, diagnostic)
	if overflow := len(runtime.recoveryDiagnostics) - maxRecoveryDiagnostics; overflow > 0 {
		runtime.recoveryDiagnostics = append([]model.RecoveryDiagnostic(nil), runtime.recoveryDiagnostics[overflow:]...)
	}
	runtime.mu.Unlock()
	_ = m.Save()
}

func recoveryDiagnostic(code string, deviceID string, statusCode int) model.RecoveryDiagnostic {
	return model.RecoveryDiagnostic{
		Code: code, Message: recoveryDiagnosticMessage(code), DeviceSuffix: deviceSuffix(deviceID), StatusCode: statusCode,
	}
}

func recoveryDiagnosticWithError(code string, deviceID string, err error) model.RecoveryDiagnostic {
	return recoveryDiagnostic(code, deviceID, diagnosticStatusCode(err))
}

func recoveryDiagnosticMessage(code string) string {
	messages := map[string]string{
		"remote_auth_rejected":              "远端拒绝原登录凭据，开始自动恢复",
		"recovery_unavailable":              "无法自动恢复：缺少已绑定的账号或设备",
		"binding_missing":                   "无法同步凭据：尚未完成扫码登录绑定",
		"yyb_get_code_failed":               "扫码服务未能生成临时登录凭据",
		"yyb_account_refresh_failed":        "扫码服务刷新已绑定账号失败",
		"yyb_account_refresh_succeeded":     "扫码服务已刷新已绑定账号",
		"yyb_get_code_retry_failed":         "刷新账号后仍无法生成临时登录凭据",
		"yyb_get_code_succeeded":            "扫码服务已生成临时登录凭据",
		"mocele_autologin_missing_info":     "自动登录未返回必要的 info 凭据",
		"mocele_autologin_missing_wxopenid": "自动登录未返回必要的 wxopenid 凭据",
		"mocele_autologin_failed":           "自动登录服务请求失败",
		"mocele_autologin_succeeded":        "自动登录服务已生成新凭据",
		"new_cookie_validation_failed":      "新凭据校验设备接口失败",
		"recovery_succeeded":                "登录凭据已自动恢复并校验成功",
		"recovery_failed":                   "登录凭据自动恢复失败",
	}
	if message, ok := messages[code]; ok {
		return message
	}
	return "登录凭据恢复过程发生未知错误"
}

func moceleDiagnosticCode(err error) string {
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "missing info cookie"):
		return "mocele_autologin_missing_info"
	case strings.Contains(message, "missing wxopenid cookie"):
		return "mocele_autologin_missing_wxopenid"
	default:
		return "mocele_autologin_failed"
	}
}

func diagnosticStatusCode(err error) int {
	if err == nil {
		return 0
	}
	match := recoveryStatusCodePattern.FindStringSubmatch(err.Error())
	if len(match) != 2 {
		return 0
	}
	var statusCode int
	_, _ = fmt.Sscanf(match[1], "%d", &statusCode)
	return statusCode
}

func deviceSuffix(deviceID string) string {
	deviceID = strings.TrimSpace(deviceID)
	if len(deviceID) <= 4 {
		return deviceID
	}
	return deviceID[len(deviceID)-4:]
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
	req.ID = strings.TrimSpace(req.ID)
	req.Number = strings.TrimSpace(req.Number)
	req.Name = strings.TrimSpace(req.Name)
	req.Address = strings.TrimSpace(req.Address)
	runtime.recordRequest()
	m.recordMetric(userID, "request")
	user, _ := m.User(userID)
	if !user.RefreshEnabled {
		return model.Pile{}, fmt.Errorf("管理员已暂停此账户的远端刷新，暂时无法验证新设备")
	}
	if req.ID == "" {
		if req.Number == "" {
			return model.Pile{}, fmt.Errorf("请输入桩号或设备长ID")
		}
		if user.DeviceLimit > 0 && len(runtime.client.DeviceIDs()) >= user.DeviceLimit {
			return model.Pile{}, fmt.Errorf("当前账户最多添加 %d 台充电桩", user.DeviceLimit)
		}
		resolvedID, err := runtime.client.ResolveDeviceIDByNumber(req.Number)
		if err != nil {
			runtime.recordFailure(false)
			_ = m.Save()
			return model.Pile{}, err
		}
		req.ID = resolvedID
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
		info := runtime.store.Snapshot().Refresh
		m.recordRefreshMetrics(userID, info)
		m.recordMetric(userID, "cookie_error")
		runtime.client.RemoveDevice(req.ID)
		runtime.recordFailure(charger.IsAuthExpired(err))
		_ = m.Save()
		return model.Pile{}, err
	}
	snapshot := runtime.store.Snapshot()
	m.recordRefreshMetrics(userID, snapshot.Refresh)
	for _, pile := range snapshot.Piles {
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

func (m *Manager) AddPileWithYYB(userID string, req model.PileUpsertRequest, yybClient YYBCodeClient, moceleClient MoceleCookieClient) (model.Pile, error) {
	runtime, err := m.runtimeFor(userID)
	if err != nil {
		return model.Pile{}, err
	}
	req.ID = strings.TrimSpace(req.ID)
	req.Number = strings.TrimSpace(req.Number)
	if req.ID == "" && req.Number != "" {
		resolvedID, err := runtime.client.ResolveDeviceIDByNumber(req.Number)
		if err != nil {
			return model.Pile{}, err
		}
		req.ID = resolvedID
	}

	pile, err := m.AddPile(userID, req)
	if err == nil {
		return pile, nil
	}
	if !charger.IsAuthExpired(err) {
		return model.Pile{}, err
	}
	if req.ID == "" {
		return model.Pile{}, err
	}
	binding, bindingErr := m.YYBBinding(userID)
	if bindingErr != nil {
		return model.Pile{}, bindingErr
	}
	if binding == nil || binding.Ref == "" {
		return model.Pile{}, ErrYYBBindingRequired
	}
	if _, syncErr := m.SyncCookieFromYYB(userID, req.ID, yybClient, moceleClient); syncErr != nil {
		return model.Pile{}, fmt.Errorf("自动更新登录凭据失败: %w", syncErr)
	}
	return m.AddPile(userID, req)
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
		info := runtime.store.Snapshot().Refresh
		m.recordRefreshMetrics(userID, info)
		m.recordMetric(userID, "cookie_error")
		runtime.recordFailure(charger.IsAuthExpired(err))
		_ = m.Save()
		return model.DashboardSnapshot{}, err
	}
	snapshot := runtime.store.Snapshot()
	if snapshot.Refresh.Cached {
		m.recordMetric(userID, "cache")
	} else {
		m.recordRefreshMetrics(userID, snapshot.Refresh)
	}
	return snapshot, m.Save()
}

func (m *Manager) RefreshWithYYB(userID string, force bool, yybClient YYBCodeClient, moceleClient MoceleCookieClient) (model.DashboardSnapshot, error) {
	snapshot, err := m.Refresh(userID, force)
	if err == nil || !charger.IsAuthExpired(err) {
		return snapshot, err
	}
	m.recordRecoveryDiagnostic(userID, recoveryDiagnosticWithError("remote_auth_rejected", "", err))
	if yybClient == nil || moceleClient == nil {
		m.recordRecoveryDiagnostic(userID, recoveryDiagnostic("recovery_unavailable", "", 0))
		return model.DashboardSnapshot{}, err
	}
	return m.RecoverRefreshWithYYB(userID, yybClient, moceleClient)
}

func (m *Manager) RecoverRefreshWithYYB(userID string, yybClient YYBCodeClient, moceleClient MoceleCookieClient) (model.DashboardSnapshot, error) {
	deviceID, ok, deviceErr := m.FirstDeviceID(userID)
	if deviceErr != nil || !ok {
		if deviceErr != nil {
			return model.DashboardSnapshot{}, deviceErr
		}
		m.recordRecoveryDiagnostic(userID, recoveryDiagnostic("recovery_unavailable", "", 0))
		return model.DashboardSnapshot{}, fmt.Errorf("no device is available for automatic credential recovery")
	}
	snapshot, syncErr := m.SyncCookieFromYYB(userID, deviceID, yybClient, moceleClient)
	if syncErr != nil {
		m.recordRecoveryDiagnostic(userID, recoveryDiagnosticWithError("recovery_failed", deviceID, syncErr))
		return model.DashboardSnapshot{}, fmt.Errorf("自动更新登录凭据失败: %w", syncErr)
	}
	m.recordRecoveryDiagnostic(userID, recoveryDiagnostic("recovery_succeeded", deviceID, 0))
	return snapshot, nil
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
		info := runtime.store.Snapshot().Refresh
		m.recordRefreshMetrics(userID, info)
		_ = runtime.client.UpdateCookie(previous)
		runtime.recordFailure(charger.IsAuthExpired(err))
		_ = m.Save()
		return model.DashboardSnapshot{}, err
	}
	snapshot := runtime.store.Snapshot()
	m.recordRefreshMetrics(userID, snapshot.Refresh)
	return snapshot, m.Save()
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
		ID:              user.ID,
		Username:        user.Username,
		Role:            user.Role,
		Enabled:         user.Enabled,
		CreatedAt:       user.CreatedAt,
		DeviceLimit:     user.DeviceLimit,
		RefreshEnabled:  user.RefreshEnabled,
		UsageGuideAckAt: user.UsageGuideAckAt,
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
	exceptions := make([]model.SystemException, 0)
	now := time.Now()
	for _, summary := range users {
		if summary.User.Role != model.RoleUser {
			continue
		}
		if len(summary.DeviceIDs) > 0 && summary.Credential.State != model.CredentialHealthy {
			exceptions = append(exceptions, model.SystemException{
				ID:       "credential-" + summary.User.ID,
				UserID:   summary.User.ID,
				Username: summary.User.Username,
				Type:     "credential",
				Level:    credentialIssueLevel(summary.Credential.State),
				Message:  credentialIssueMessage(summary.Credential.State),
				Time:     summary.SnapshotUpdatedAt,
			})
		}
		if hasActiveAuthFailure(summary.Stats) {
			at := *summary.Stats.LastAuthFailureAt
			exceptions = append(exceptions, model.SystemException{ID: "auth-" + summary.User.ID, UserID: summary.User.ID, Username: summary.User.Username, Type: "cookie_expired", Level: "critical", Message: "远端鉴权失败，Cookie 可能已失效", Time: at})
		}
		if summary.LastRefresh.FailedDevices > 0 {
			exceptions = append(exceptions, model.SystemException{ID: "refresh-" + summary.User.ID, UserID: summary.User.ID, Username: summary.User.Username, Type: "refresh", Level: "warning", Message: fmt.Sprintf("%d 台设备刷新失败", summary.LastRefresh.FailedDevices), Time: issueTime(summary)})
		}
		if summary.LastRefresh.LastRemoteAt != nil && now.Sub(*summary.LastRefresh.LastRemoteAt) > 24*time.Hour {
			exceptions = append(exceptions, model.SystemException{ID: "stale-" + summary.User.ID, UserID: summary.User.ID, Username: summary.User.Username, Type: "stale", Level: "warning", Message: "设备数据已超过 24 小时未更新", Time: *summary.LastRefresh.LastRemoteAt})
		}
		if summary.Dashboard.OfflinePorts > 0 {
			exceptions = append(exceptions, model.SystemException{ID: "offline-" + summary.User.ID, UserID: summary.User.ID, Username: summary.User.Username, Type: "offline", Level: "warning", Message: fmt.Sprintf("%d 个充电口处于离线状态", summary.Dashboard.OfflinePorts), Time: summary.SnapshotUpdatedAt})
		}
	}
	sort.SliceStable(exceptions, func(i, j int) bool {
		if exceptions[i].Level != exceptions[j].Level {
			return exceptions[i].Level == "critical"
		}
		return exceptions[i].Time.After(exceptions[j].Time)
	})
	if hourly == nil {
		hourly = []model.MetricPoint{}
	}
	if daily == nil {
		daily = []model.MetricPoint{}
	}
	if users == nil {
		users = []model.AdminUserSummary{}
	}
	overview := adminOverview(users, hourly, len(exceptions))
	return model.AdminStats{Overview: overview, Users: users, Hourly: hourly, Daily: daily, Exceptions: exceptions}
}

func credentialIssueLevel(state model.CredentialState) string {
	if state == model.CredentialExpired {
		return "critical"
	}
	return "warning"
}

func credentialIssueMessage(state model.CredentialState) string {
	switch state {
	case model.CredentialExpired:
		return "扫码登录绑定已失效"
	case model.CredentialSyncFailed:
		return "扫码登录凭据同步失败"
	default:
		return "尚未完成扫码登录绑定"
	}
}

func issueTime(summary model.AdminUserSummary) time.Time {
	if summary.LastRefresh.LastRemoteAt != nil {
		return *summary.LastRefresh.LastRemoteAt
	}
	return summary.SnapshotUpdatedAt
}

func hasActiveAuthFailure(stats model.TrafficStats) bool {
	if stats.AuthFailures == 0 || stats.LastAuthFailureAt == nil {
		return false
	}
	if stats.LastRemoteOKAt == nil {
		return true
	}
	return stats.LastAuthFailureAt.After(*stats.LastRemoteOKAt)
}

func adminOverview(users []model.AdminUserSummary, hourly []model.MetricPoint, issueCount int) model.AdminOverview {
	remote := 0
	remoteOK := 0
	active := map[string]struct{}{}
	managed := 0
	offline := 0
	for _, point := range hourly {
		remote += point.Remote
		remoteOK += point.RemoteOK
	}
	for _, summary := range users {
		managed += len(summary.DeviceIDs)
		offline += summary.Dashboard.OfflinePorts
		if summary.User.Role == model.RoleUser && summary.Stats.LastRequestAt != nil &&
			time.Since(*summary.Stats.LastRequestAt) <= 24*time.Hour {
			active[summary.User.ID] = struct{}{}
		}
	}
	rate := 0.0
	if remote > 0 {
		rate = math.Round(float64(remoteOK)/float64(remote)*1000) / 10
	}
	return model.AdminOverview{
		OpenIssues: issueCount, RemoteSuccessRate: rate,
		ActiveUsers: len(active), ManagedDevices: managed, OfflinePorts: offline,
	}
}

func (m *Manager) recordMetric(userID, kind string) {
	m.recordMetricCount(userID, kind, 1)
}

func (m *Manager) recordMetricCount(userID, kind string, count int) {
	if count <= 0 {
		return
	}
	now := time.Now()
	if kind == "remote_ok" {
		if runtime, err := m.runtimeFor(userID); err == nil {
			runtime.recordRemoteOK(now)
		}
	}
	for i := 0; i < count; i++ {
		_ = m.repository.RecordMetric(userID, kind, now)
	}
}

func (m *Manager) recordRefreshMetrics(userID string, info model.RefreshInfo) {
	m.recordMetricCount(userID, "remote", info.AttemptedDevices)
	m.recordMetricCount(userID, "remote_ok", info.SuccessfulDevices)
	m.recordMetricCount(userID, "remote_failed", info.FailedDevices)
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
