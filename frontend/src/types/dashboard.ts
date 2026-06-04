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
  minIntervalSeconds: number;
  cached: boolean;
  message?: string;
}

export interface DashboardSnapshot {
  piles: Pile[];
  updatedAt: string;
  statistics: DashboardCounters;
  refresh: RefreshInfo;
}
