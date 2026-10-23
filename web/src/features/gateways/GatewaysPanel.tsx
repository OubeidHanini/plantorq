import { useQuery } from "@tanstack/react-query";
import { api } from "@/shared/api/client";
import type { GatewayStatus } from "@/shared/api/types";
import { Badge, Card, Empty, fmt } from "@/shared/ui";

export const gatewayKeys = { all: ["gateways"] as const };

export const useGateways = () => useQuery({ queryKey: gatewayKeys.all, queryFn: () => api<GatewayStatus[]>("/gateways") });

export function GatewaysPanel({ gateways }: { gateways: GatewayStatus[] }) {
  return (
    <Card title="Edge gateways (Go agent)">
      {gateways.length === 0 ? (
        <Empty>No gateway has reported yet.</Empty>
      ) : (
        gateways.map((g) => (
          <div key={g.gateway} className="gateway">
            <div className="gateway__head">
              <strong>{g.gateway}</strong>
              <Badge tone={g.connected ? "info" : "warn"}>{g.connected ? "connected" : "offline"}</Badge>
            </div>
            <dl className="gateway__metrics">
              <div>
                <dt>Throughput</dt>
                <dd className="mono">{fmt(g.msg_per_sec, 1)} msg/s</dd>
              </div>
              <div>
                <dt>Offline buffer</dt>
                <dd className={`mono ${g.buffered > 0 ? "text--warn" : ""}`}>{g.buffered} pts</dd>
              </div>
              <div>
                <dt>Poll latency p95</dt>
                <dd className="mono">{fmt(g.poll_p95_ms, 1)} ms</dd>
              </div>
              <div>
                <dt>Machines</dt>
                <dd className="mono">
                  {g.machines_online} / {g.machines_total}
                </dd>
              </div>
            </dl>
          </div>
        ))
      )}
    </Card>
  );
}
