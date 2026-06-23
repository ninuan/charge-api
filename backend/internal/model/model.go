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
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"passwordHash,omitempty"`
	Role         UserRole  `json:"role"`
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type CurrentUser struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Role      UserRole  `json:"role"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"createdAt"`
}

type LoginRequest struct {
	Username      string `json:"username"`
	Password      string `json:"password"`
	CaptchaToken  string `json:"captchaToken"`
	CaptchaID     string `json:"captchaId,omitempty"`
	CaptchaAnswer string `json:"captchaAnswer,omitempty"`
}

type UserCreateRequest struct {
	Username string   `json:"username"`
	Password string   `json:"password"`
	Role     UserRole `json:"role"`
	Enabled  *bool    `json:"enabled,omitempty"`
}

type UserUpdateRequest struct {
	Password *string   `json:"password,omitempty"`
	Role     *UserRole `json:"role,omitempty"`
	Enabled  *bool     `json:"enabled,omitempty"`
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
