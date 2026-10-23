// Wire types of the Go API (internal/transport/http). Kept in one place so a
// contract change is a single, type-checked edit.

export type Role = "admin" | "operator" | "viewer";
export type Severity = "info" | "warning" | "critical";
export type MachineStatus = "running" | "warning" | "critical" | "offline";

export interface Principal {
  uid: number;
  email: string;
  name: string;
  tid: number;
  tenant: string;
  role: Role;
}

export interface Session {
  token: string;
  expires_at: string;
  user: Principal;
}

export interface Machine {
  id: string;
  name: string;
  kind: string;
  protocol: string;
  status: MachineStatus;
  last_seen?: number;
  values: Record<string, number>;
}

export interface Reading {
  tenant: string;
  machine: string;
  gateway?: string;
  source: "edge" | "analytics";
  ts: number;
  values: Record<string, number>;
}

export interface Sample {
  t: number;
  v: number;
}

export interface GatewayStatus {
  tenant: string;
  gateway: string;
  ts: number;
  msg_per_sec: number;
  buffered: number;
  poll_p95_ms: number;
  machines_online: number;
  machines_total: number;
  connected: boolean;
}

export interface Alert {
  id: number;
  rule_id: number;
  rule_name: string;
  machine: string;
  metric: string;
  operator: string;
  threshold: number;
  value: number;
  severity: Severity;
  started_at: string;
  resolved_at?: string;
  acked_at?: string;
}

export interface Rule {
  id: number;
  name: string;
  machine: string;
  metric: string;
  operator: ">" | ">=" | "<" | "<=";
  threshold: number;
  for_seconds: number;
  severity: Severity;
  enabled: boolean;
}

export type LiveEvent =
  | { type: "reading"; data: Reading }
  | { type: "gateway"; data: GatewayStatus }
  | { type: "alert"; data: Alert & { state: "firing" | "resolved" } };
