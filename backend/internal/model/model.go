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

type UserRole string

const (
	RoleAdmin UserRole = "admin"
	RoleUser  UserRole = "user"
)

type User struct {
	ID             string    `json:"id"`
	Username       string    `json:"username"`
	PasswordHash   string    `json:"passwordHash,omitempty"`
	Role           UserRole  `json:"role"`
	Enabled        bool      `json:"enabled"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
	DeviceLimit    int       `json:"deviceLimit"`
	RefreshEnabled bool      `json:"refreshEnabled"`
}

type CurrentUser struct {
	ID             string    `json:"id"`
	Username       string    `json:"username"`
	Role           UserRole  `json:"role"`
	Enabled        bool      `json:"enabled"`
	CreatedAt      time.Time `json:"createdAt"`
	DeviceLimit    int       `json:"deviceLimit"`
	RefreshEnabled bool      `json:"refreshEnabled"`
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
	CookieErrors int       `json:"cookieErrors"`
	ActiveUsers  int       `json:"activeUsers"`
}

type AdminStats struct {
	Users      []AdminUserSummary `json:"users"`
	Hourly     []MetricPoint      `json:"hourly"`
	Daily      []MetricPoint      `json:"daily"`
	Exceptions []SystemException  `json:"exceptions"`
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
}

type AdminUserSummary struct {
	User        CurrentUser       `json:"user"`
	Stats       TrafficStats      `json:"stats"`
	Dashboard   DashboardCounters `json:"dashboard"`
	DeviceIDs   []string          `json:"deviceIds"`
	HasCookie   bool              `json:"hasCookie"`
	LastRefresh RefreshInfo       `json:"lastRefresh"`
}
