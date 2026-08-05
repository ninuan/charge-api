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

type PortStatusEvent struct {
	ID            int64       `json:"id"`
	UserID        string      `json:"userId"`
	DeviceID      string      `json:"deviceId"`
	PortID        int         `json:"portId"`
	FromStatus    *PortStatus `json:"fromStatus,omitempty"`
	ToStatus      PortStatus  `json:"toStatus"`
	ChangedAt     time.Time   `json:"changedAt"`
	UsedSeconds   int         `json:"usedSeconds"`
	RemainingText string      `json:"remainingText,omitempty"`
	Source        string      `json:"source"`
}

type HistoryWindow struct {
	Range    string    `json:"range"`
	Timezone string    `json:"timezone"`
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
}

type PortHistoryMetrics struct {
	ObservedSeconds       int64    `json:"observedSeconds"`
	GapSeconds            int64    `json:"gapSeconds"`
	IdleSeconds           int64    `json:"idleSeconds"`
	InUseSeconds          int64    `json:"inUseSeconds"`
	OfflineSeconds        int64    `json:"offlineSeconds"`
	OccupancyPercent      *float64 `json:"occupancyPercent"`
	CompletedSessions     int      `json:"completedSessions"`
	AverageSessionSeconds *int64   `json:"averageSessionSeconds"`
	SampleState           string   `json:"sampleState"`
}

type HistoryDailyPoint struct {
	Date    string             `json:"date"`
	Metrics PortHistoryMetrics `json:"metrics"`
}

type HistoryHeatmapCell struct {
	Weekday          int      `json:"weekday"`
	Hour             int      `json:"hour"`
	IdleSeconds      int64    `json:"idleSeconds"`
	InUseSeconds     int64    `json:"inUseSeconds"`
	OfflineSeconds   int64    `json:"offlineSeconds"`
	OccupancyPercent *float64 `json:"occupancyPercent"`
	SampleDates      int      `json:"sampleDates"`
	SampleSufficient bool     `json:"sampleSufficient"`
}

type HistoryHourInsight struct {
	Weekday          int     `json:"weekday"`
	Hour             int     `json:"hour"`
	OccupancyPercent float64 `json:"occupancyPercent"`
	SampleDates      int     `json:"sampleDates"`
}

type PortHistoryTimelineItem struct {
	PortID        int         `json:"portId"`
	FromStatus    *PortStatus `json:"fromStatus,omitempty"`
	ToStatus      PortStatus  `json:"toStatus"`
	ChangedAt     time.Time   `json:"changedAt"`
	UsedSeconds   int         `json:"usedSeconds"`
	RemainingText string      `json:"remainingText,omitempty"`
}

type HistoryDevice struct {
	ID      string `json:"id"`
	Number  string `json:"number"`
	Name    string `json:"name"`
	Address string `json:"address"`
}

type PortHistorySummary struct {
	PortID           int                `json:"portId"`
	CurrentStatus    PortStatus         `json:"currentStatus"`
	HistoryStartedAt *time.Time         `json:"historyStartedAt,omitempty"`
	Metrics          PortHistoryMetrics `json:"metrics"`
}

type DeviceHistoryResponse struct {
	Device           HistoryDevice        `json:"device"`
	Window           HistoryWindow        `json:"window"`
	Metrics          PortHistoryMetrics   `json:"metrics"`
	Daily            []HistoryDailyPoint  `json:"daily"`
	Heatmap          []HistoryHeatmapCell `json:"heatmap"`
	Ports            []PortHistorySummary `json:"ports"`
	BusiestHours     []HistoryHourInsight `json:"busiestHours"`
	QuietSuggestion  *HistoryHourInsight  `json:"quietSuggestion,omitempty"`
	HistoryStartedAt *time.Time           `json:"historyStartedAt,omitempty"`
	HistoryNotice    string               `json:"historyNotice"`
}

type PortHistoryResponse struct {
	Device            HistoryDevice             `json:"device"`
	PortID            int                       `json:"portId"`
	CurrentStatus     PortStatus                `json:"currentStatus"`
	Window            HistoryWindow             `json:"window"`
	Metrics           PortHistoryMetrics        `json:"metrics"`
	Daily             []HistoryDailyPoint       `json:"daily"`
	Timeline          []PortHistoryTimelineItem `json:"timeline"`
	TimelineTruncated bool                      `json:"timelineTruncated"`
	HistoryStartedAt  *time.Time                `json:"historyStartedAt,omitempty"`
	HistoryNotice     string                    `json:"historyNotice"`
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
	ID                 string     `json:"id"`
	Username           string     `json:"username"`
	PasswordHash       string     `json:"passwordHash,omitempty"`
	Role               UserRole   `json:"role"`
	Enabled            bool       `json:"enabled"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
	DeviceLimit        int        `json:"deviceLimit"`
	RefreshEnabled     bool       `json:"refreshEnabled"`
	MustChangePassword bool       `json:"mustChangePassword"`
	UsageGuideAckAt    *time.Time `json:"usageGuideAckAt,omitempty"`
}

type CurrentUser struct {
	ID                 string     `json:"id"`
	Username           string     `json:"username"`
	Role               UserRole   `json:"role"`
	Enabled            bool       `json:"enabled"`
	CreatedAt          time.Time  `json:"createdAt"`
	DeviceLimit        int        `json:"deviceLimit"`
	RefreshEnabled     bool       `json:"refreshEnabled"`
	MustChangePassword bool       `json:"mustChangePassword"`
	UsageGuideAckAt    *time.Time `json:"usageGuideAckAt,omitempty"`
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
	Role           *UserRole `json:"role,omitempty"`
	Enabled        *bool     `json:"enabled,omitempty"`
	DeviceLimit    *int      `json:"deviceLimit,omitempty"`
	RefreshEnabled *bool     `json:"refreshEnabled,omitempty"`
}

type PasswordChangeRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

type TemporaryPasswordResponse struct {
	TemporaryPassword string `json:"temporaryPassword"`
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

type InviteCodePage struct {
	Items      []InviteCode `json:"items"`
	Page       int          `json:"page"`
	PageSize   int          `json:"pageSize"`
	Total      int          `json:"total"`
	TotalPages int          `json:"totalPages"`
}

type SessionView struct {
	ID           string    `json:"id"`
	CreatedAt    time.Time `json:"createdAt"`
	ExpiresAt    time.Time `json:"expiresAt"`
	LastActiveAt time.Time `json:"lastActiveAt"`
	Browser      string    `json:"browser"`
	OS           string    `json:"os"`
	DeviceType   string    `json:"deviceType"`
	IPLabel      string    `json:"ipLabel"`
	Current      bool      `json:"current"`
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

type AdminTrendWindow struct {
	Range      string    `json:"range"`
	Timezone   string    `json:"timezone"`
	BucketUnit string    `json:"bucketUnit"`
	Start      time.Time `json:"start"`
	End        time.Time `json:"end"`
}

type AdminTrendPoint struct {
	Start             time.Time `json:"start"`
	End               time.Time `json:"end"`
	Requests          int       `json:"requests"`
	RemoteAttempts    int       `json:"remoteAttempts"`
	RemoteSuccesses   int       `json:"remoteSuccesses"`
	RemoteFailures    int       `json:"remoteFailures"`
	RemoteSuccessRate *float64  `json:"remoteSuccessRate"`
	ActiveUsers       int       `json:"activeUsers"`
	OfflinePorts      int       `json:"offlinePorts"`
}

type AdminTrendSummary struct {
	Requests          int      `json:"requests"`
	RemoteAttempts    int      `json:"remoteAttempts"`
	RemoteSuccesses   int      `json:"remoteSuccesses"`
	RemoteFailures    int      `json:"remoteFailures"`
	RemoteSuccessRate *float64 `json:"remoteSuccessRate"`
	ActiveUsers       int      `json:"activeUsers"`
	OfflinePorts      int      `json:"offlinePorts"`
}

type AdminTrends struct {
	Window    AdminTrendWindow  `json:"window"`
	Summary   AdminTrendSummary `json:"summary"`
	Points    []AdminTrendPoint `json:"points"`
	UpdatedAt time.Time         `json:"updatedAt"`
}

type HealthState string

const (
	HealthHealthy     HealthState = "healthy"
	HealthDegraded    HealthState = "degraded"
	HealthUnavailable HealthState = "unavailable"
)

type ServiceHealth struct {
	State               HealthState `json:"state"`
	Message             string      `json:"message"`
	RecoveryAdvice      string      `json:"recoveryAdvice,omitempty"`
	LastRecoveredAt     *time.Time  `json:"lastRecoveredAt,omitempty"`
	Availability24Hours float64     `json:"availability24Hours"`
	ConsecutiveFailures int         `json:"consecutiveFailures"`
}

type AdminHealth struct {
	CheckedAt time.Time     `json:"checkedAt"`
	Charge    ServiceHealth `json:"charge"`
	Database  ServiceHealth `json:"database"`
	YYB       ServiceHealth `json:"yyb"`
}

type SystemException struct {
	ID          string     `json:"id"`
	UserID      string     `json:"userId"`
	Username    string     `json:"username"`
	DeviceID    string     `json:"deviceId,omitempty"`
	Type        string     `json:"type"`
	Level       string     `json:"level"`
	Message     string     `json:"message"`
	Status      string     `json:"status"`
	Note        string     `json:"note,omitempty"`
	Occurrences int        `json:"occurrences"`
	HandledBy   string     `json:"handledBy,omitempty"`
	HandledAt   *time.Time `json:"handledAt,omitempty"`
	FirstSeenAt time.Time  `json:"firstSeenAt"`
	Time        time.Time  `json:"time"`
}

type IncidentUpdateRequest struct {
	Status string `json:"status"`
	Note   string `json:"note"`
}

type AuditEntry struct {
	ID          int64     `json:"id"`
	ActorID     string    `json:"actorId"`
	Actor       string    `json:"actor"`
	Action      string    `json:"action"`
	TargetType  string    `json:"targetType"`
	TargetID    string    `json:"targetId"`
	TargetLabel string    `json:"targetLabel"`
	Result      string    `json:"result"`
	Message     string    `json:"message,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

type AuditPage struct {
	Items      []AuditEntry `json:"items"`
	Page       int          `json:"page"`
	PageSize   int          `json:"pageSize"`
	Total      int          `json:"total"`
	TotalPages int          `json:"totalPages"`
}

type OperationsStatus struct {
	DatabaseSizeBytes   int64      `json:"databaseSizeBytes"`
	MetricRows          int64      `json:"metricRows"`
	MetricRetentionDays int        `json:"metricRetentionDays"`
	IntegrityResult     string     `json:"integrityResult"`
	CheckedAt           time.Time  `json:"checkedAt"`
	LastBackupAt        *time.Time `json:"lastBackupAt,omitempty"`
	LastBackupSizeBytes int64      `json:"lastBackupSizeBytes,omitempty"`
	BackupState         string     `json:"backupState"`
	BackupMessage       string     `json:"backupMessage"`
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

// RecoveryDiagnostic is a deliberately non-sensitive user-operation trace. It
// must never contain cookies, login codes, OpenIDs, refs, or raw upstream
// response bodies.
type RecoveryDiagnostic struct {
	Operation    string    `json:"operation"`
	Code         string    `json:"code"`
	Message      string    `json:"message"`
	DeviceSuffix string    `json:"deviceSuffix,omitempty"`
	StatusCode   int       `json:"statusCode,omitempty"`
	At           time.Time `json:"at"`
}

type AdminUserSummary struct {
	User                CurrentUser          `json:"user"`
	Stats               TrafficStats         `json:"stats"`
	Dashboard           DashboardCounters    `json:"dashboard"`
	DeviceIDs           []string             `json:"deviceIds"`
	HasCookie           bool                 `json:"hasCookie"`
	Credential          CredentialSummary    `json:"credential"`
	SnapshotUpdatedAt   time.Time            `json:"snapshotUpdatedAt"`
	LastRefresh         RefreshInfo          `json:"lastRefresh"`
	RecoveryDiagnostics []RecoveryDiagnostic `json:"recoveryDiagnostics"`
}

type AdminUserDetail struct {
	Summary  AdminUserSummary `json:"summary"`
	Piles    []Pile           `json:"piles"`
	Sessions []SessionView    `json:"sessions"`
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
