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
}

export interface AdminUserSummary {
  user: CurrentUser;
  stats: TrafficStats;
  dashboard: DashboardCounters;
  deviceIds: string[];
  hasCookie: boolean;
  lastRefresh: RefreshInfo;
}
