package model

import "time"

type PortStatus string

const (
	PortIdle    PortStatus = "idle"
	PortInUse   PortStatus = "in_use"
	PortOffline PortStatus = "offline"
)

type Port struct {
	ID            int        `json:"id"`
	Status        PortStatus `json:"status"`
	PowerKW       float64    `json:"powerKw"`
	EnergyKWh     float64    `json:"energyKwh"`
	UpdatedAt     time.Time  `json:"updatedAt"`
	StartedAt     *time.Time `json:"startedAt,omitempty"`
	SessionMin    int        `json:"sessionMin"`
	UsedSeconds   int        `json:"usedSeconds"`
	UsedText      string     `json:"usedText,omitempty"`
	RemainingText string     `json:"remainingText,omitempty"`
}

type Pile struct {
	ID          string    `json:"id"`
	Number      string    `json:"number"`
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	Address     string    `json:"address"`
	OpenNum     int       `json:"openNum"`
	Online      bool      `json:"online"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	Source      string    `json:"source"`
	Ports       []Port    `json:"ports"`
	UsedPortIDs []int     `json:"usedPortIds"`
	SortOrder   int       `json:"sortOrder"`
}

type DashboardSnapshot struct {
	Piles      []Pile            `json:"piles"`
	UpdatedAt  time.Time         `json:"updatedAt"`
	Statistics DashboardCounters `json:"statistics"`
	Refresh    RefreshInfo       `json:"refresh"`
}

type DashboardCounters struct {
	PileCount      int `json:"pileCount"`
	PortCount      int `json:"portCount"`
	InUsePortCount int `json:"inUsePortCount"`
	IdlePortCount  int `json:"idlePortCount"`
	OfflinePorts   int `json:"offlinePorts"`
}

type RefreshInfo struct {
	LastRemoteAt       *time.Time `json:"lastRemoteAt,omitempty"`
	NextRemoteAt       *time.Time `json:"nextRemoteAt,omitempty"`
	NextRetryAt        *time.Time `json:"nextRetryAt,omitempty"`
	MinIntervalSeconds int        `json:"minIntervalSeconds"`
	AttemptedDevices   int        `json:"attemptedDevices"`
	SuccessfulDevices  int        `json:"successfulDevices"`
	FailedDevices      int        `json:"failedDevices"`
	SkippedDevices     int        `json:"skippedDevices"`
	Cached             bool       `json:"cached"`
	Partial            bool       `json:"partial"`
	Message            string     `json:"message,omitempty"`
}

type PileUpsertRequest struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Number  string `json:"number"`
	OpenNum int    `json:"openNum"`
	Status  string `json:"status"`
	Address string `json:"address"`
}

type YYBBinding struct {
	OpenID        string     `json:"openid"`
	Ref           string     `json:"ref"`
	Nickname      string     `json:"nickname"`
	Avatar        string     `json:"avatar"`
	Status        string     `json:"status"`
	BoundAt       time.Time  `json:"boundAt"`
	LastCheckedAt *time.Time `json:"lastCheckedAt,omitempty"`
	LastError     string     `json:"lastError,omitempty"`
}

type UserRole string

const (
	RoleAdmin UserRole = "admin"
	RoleUser  UserRole = "user"
)

type User struct {
	ID              string     `json:"id"`
	Username        string     `json:"username"`
	PasswordHash    string     `json:"passwordHash,omitempty"`
	Role            UserRole   `json:"role"`
	Enabled         bool       `json:"enabled"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
	DeviceLimit     int        `json:"deviceLimit"`
	RefreshEnabled  bool       `json:"refreshEnabled"`
	UsageGuideAckAt *time.Time `json:"usageGuideAckAt,omitempty"`
}

type CurrentUser struct {
	ID              string     `json:"id"`
	Username        string     `json:"username"`
	Role            UserRole   `json:"role"`
	Enabled         bool       `json:"enabled"`
	CreatedAt       time.Time  `json:"createdAt"`
	DeviceLimit     int        `json:"deviceLimit"`
	RefreshEnabled  bool       `json:"refreshEnabled"`
	UsageGuideAckAt *time.Time `json:"usageGuideAckAt,omitempty"`
}

type LoginRequest struct {
	Username      string `json:"username"`
	Password      string `json:"password"`
	CaptchaToken  string `json:"captchaToken"`
	CaptchaID     string `json:"captchaId,omitempty"`
	CaptchaAnswer string `json:"captchaAnswer,omitempty"`
	InviteCode    string `json:"inviteCode,omitempty"`
}

type UserCreateRequest struct {
	Username       string   `json:"username"`
	Password       string   `json:"password"`
	Role           UserRole `json:"role"`
	Enabled        *bool    `json:"enabled,omitempty"`
	DeviceLimit    *int     `json:"deviceLimit,omitempty"`
	RefreshEnabled *bool    `json:"refreshEnabled,omitempty"`
}

type UserUpdateRequest struct {
	Password       *string   `json:"password,omitempty"`
	Role           *UserRole `json:"role,omitempty"`
	Enabled        *bool     `json:"enabled,omitempty"`
	DeviceLimit    *int      `json:"deviceLimit,omitempty"`
	RefreshEnabled *bool     `json:"refreshEnabled,omitempty"`
}

type PasswordChangeRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

type RegistrationSettings struct {
	OpenRegistration      bool `json:"openRegistration"`
	InviteRequired        bool `json:"inviteRequired"`
	DefaultDeviceLimit    int  `json:"defaultDeviceLimit"`
	DefaultRefreshEnabled bool `json:"defaultRefreshEnabled"`
	StatsRetentionDays    int  `json:"statsRetentionDays"`
}

type InviteCode struct {
	ID        string     `json:"id"`
	Code      string     `json:"code"`
	Enabled   bool       `json:"enabled"`
	CreatedAt time.Time  `json:"createdAt"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	UsedCount int        `json:"usedCount"`
}

type SessionView struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
	Current   bool      `json:"current"`
}

type MetricPoint struct {
	Time         time.Time `json:"time"`
	Requests     int       `json:"requests"`
	Remote       int       `json:"remote"`
	CacheHits    int       `json:"cacheHits"`
	RemoteOK     int       `json:"remoteOk"`
	RemoteFailed int       `json:"remoteFailed"`
	CookieErrors int       `json:"cookieErrors"`
	ActiveUsers  int       `json:"activeUsers"`
}

type AdminOverview struct {
	OpenIssues        int     `json:"openIssues"`
	RemoteSuccessRate float64 `json:"remoteSuccessRate"`
	ActiveUsers       int     `json:"activeUsers"`
	ManagedDevices    int     `json:"managedDevices"`
	OfflinePorts      int     `json:"offlinePorts"`
}

type AdminStats struct {
	Overview   AdminOverview      `json:"overview"`
	Users      []AdminUserSummary `json:"users"`
	Hourly     []MetricPoint      `json:"hourly"`
	Daily      []MetricPoint      `json:"daily"`
	Exceptions []SystemException  `json:"exceptions"`
}

type HealthState string

const (
	HealthHealthy     HealthState = "healthy"
	HealthDegraded    HealthState = "degraded"
	HealthUnavailable HealthState = "unavailable"
)

type ServiceHealth struct {
	State   HealthState `json:"state"`
	Message string      `json:"message"`
}

type AdminHealth struct {
	CheckedAt time.Time     `json:"checkedAt"`
	Charge    ServiceHealth `json:"charge"`
	Database  ServiceHealth `json:"database"`
	YYB       ServiceHealth `json:"yyb"`
}

type SystemException struct {
	ID       string    `json:"id"`
	UserID   string    `json:"userId"`
	Username string    `json:"username"`
	DeviceID string    `json:"deviceId,omitempty"`
	Type     string    `json:"type"`
	Level    string    `json:"level"`
	Message  string    `json:"message"`
	Time     time.Time `json:"time"`
}

type TrafficStats struct {
	TotalRequests     int        `json:"totalRequests"`
	RefreshRequests   int        `json:"refreshRequests"`
	RemoteFetches     int        `json:"remoteFetches"`
	CachedRefreshes   int        `json:"cachedRefreshes"`
	FailedRequests    int        `json:"failedRequests"`
	AuthFailures      int        `json:"authFailures"`
	LastRequestAt     *time.Time `json:"lastRequestAt,omitempty"`
	LastRemoteFetchAt *time.Time `json:"lastRemoteFetchAt,omitempty"`
	LastFailedAt      *time.Time `json:"lastFailedAt,omitempty"`
	LastAuthFailureAt *time.Time `json:"lastAuthFailureAt,omitempty"`
	LastRemoteOKAt    *time.Time `json:"lastRemoteOkAt,omitempty"`
}

type CredentialState string

const (
	CredentialUnbound       CredentialState = "unbound"
	CredentialWaitingDevice CredentialState = "waiting_device"
	CredentialHealthy       CredentialState = "healthy"
	CredentialSyncFailed    CredentialState = "sync_failed"
	CredentialExpired       CredentialState = "expired"
)

type CredentialSummary struct {
	State         CredentialState `json:"state"`
	Bound         bool            `json:"bound"`
	HasCredential bool            `json:"hasCredential"`
	LastCheckedAt *time.Time      `json:"lastCheckedAt,omitempty"`
}

type AdminUserSummary struct {
	User              CurrentUser       `json:"user"`
	Stats             TrafficStats      `json:"stats"`
	Dashboard         DashboardCounters `json:"dashboard"`
	DeviceIDs         []string          `json:"deviceIds"`
	HasCookie         bool              `json:"hasCookie"`
	Credential        CredentialSummary `json:"credential"`
	SnapshotUpdatedAt time.Time         `json:"snapshotUpdatedAt"`
	LastRefresh       RefreshInfo       `json:"lastRefresh"`
}

type AdminUserListQuery struct {
	Page       int
	PageSize   int
	Search     string
	Account    string
	Credential string
	Health     string
}

type AdminUserPage struct {
	Items      []AdminUserSummary `json:"items"`
	Page       int                `json:"page"`
	PageSize   int                `json:"pageSize"`
	Total      int                `json:"total"`
	TotalPages int                `json:"totalPages"`
}
