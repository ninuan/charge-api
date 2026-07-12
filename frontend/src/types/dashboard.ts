export type PortStatus = "idle" | "in_use" | "offline";

export interface Port {
  id: number;
  status: PortStatus;
  powerKw: number;
  energyKwh: number;
  updatedAt: string;
  startedAt?: string;
  sessionMin: number;
  usedSeconds: number;
  usedText?: string;
  remainingText?: string;
}

export interface Pile {
  id: string;
  number: string;
  name: string;
  status: string;
  address: string;
  openNum: number;
  online: boolean;
  createdAt: string;
  updatedAt: string;
  source: string;
  ports: Port[];
  usedPortIds: number[];
  sortOrder?: number;
}

export interface DashboardCounters {
  pileCount: number;
  portCount: number;
  inUsePortCount: number;
  idlePortCount: number;
  offlinePorts: number;
}

export interface RefreshInfo {
  lastRemoteAt?: string;
  nextRemoteAt?: string;
  nextRetryAt?: string;
  minIntervalSeconds: number;
  attemptedDevices: number;
  successfulDevices: number;
  failedDevices: number;
  skippedDevices: number;
  cached: boolean;
  partial: boolean;
  message?: string;
}

export interface DashboardSnapshot {
  piles: Pile[];
  updatedAt: string;
  statistics: DashboardCounters;
  refresh: RefreshInfo;
}

export type UserRole = "admin" | "user";

export interface CurrentUser {
  id: string;
  username: string;
  role: UserRole;
  enabled: boolean;
  createdAt: string;
  deviceLimit: number;
  refreshEnabled: boolean;
  usageGuideAckAt?: string;
}

export interface TrafficStats {
  totalRequests: number;
  refreshRequests: number;
  remoteFetches: number;
  cachedRefreshes: number;
  failedRequests: number;
  authFailures: number;
  lastRequestAt?: string;
  lastRemoteFetchAt?: string;
  lastFailedAt?: string;
  lastAuthFailureAt?: string;
  lastRemoteOkAt?: string;
}

export type CredentialState = "unbound" | "waiting_device" | "healthy" | "sync_failed" | "expired";

export interface CredentialSummary {
  state: CredentialState;
  bound: boolean;
  hasCredential: boolean;
  lastCheckedAt?: string;
}

export interface AdminUserSummary {
  user: CurrentUser;
  stats: TrafficStats;
  dashboard: DashboardCounters;
  deviceIds: string[];
  hasCookie: boolean;
  credential: CredentialSummary;
  snapshotUpdatedAt: string;
  lastRefresh: RefreshInfo;
}

export type AdminAccountFilter = "all" | "enabled" | "disabled";
export type AdminHealthFilter = "all" | "healthy" | "risk";
export type AdminCredentialFilter = "all" | CredentialState;

export interface AdminUserListQuery {
  page: number;
  pageSize: number;
  search: string;
  account: AdminAccountFilter;
  credential: AdminCredentialFilter;
  health: AdminHealthFilter;
}

export interface AdminUserPage {
  items: AdminUserSummary[];
  page: number;
  pageSize: number;
  total: number;
  totalPages: number;
}

export interface RegistrationSettings {
  openRegistration: boolean;
  inviteRequired: boolean;
  defaultDeviceLimit: number;
  defaultRefreshEnabled: boolean;
  statsRetentionDays: number;
}

export interface InviteCode {
  id: string;
  code: string;
  enabled: boolean;
  createdAt: string;
  expiresAt?: string;
  usedCount: number;
}

export interface SessionView {
  id: string;
  createdAt: string;
  expiresAt: string;
  current: boolean;
}

export interface MetricPoint {
  time: string;
  requests: number;
  remote: number;
  cacheHits: number;
  remoteOk: number;
  remoteFailed: number;
  cookieErrors: number;
  activeUsers: number;
}

export interface AdminOverview {
  openIssues: number;
  remoteSuccessRate: number;
  activeUsers: number;
  managedDevices: number;
  offlinePorts: number;
}

export type HealthState = "healthy" | "degraded" | "unavailable";

export interface ServiceHealth {
  state: HealthState;
  message: string;
}

export interface AdminHealth {
  checkedAt: string;
  charge: ServiceHealth;
  database: ServiceHealth;
  yyb: ServiceHealth;
}

export interface SystemException {
  id: string;
  userId: string;
  username: string;
  deviceId?: string;
  type: string;
  level: string;
  message: string;
  time: string;
}

export interface AdminStats {
  overview: AdminOverview;
  users: AdminUserSummary[];
  hourly: MetricPoint[];
  daily: MetricPoint[];
  exceptions: SystemException[];
}
