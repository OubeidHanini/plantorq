import { useState } from "react";
import { AlertsPanel } from "@/features/alerts/AlertsPanel";
import { useOpenAlerts } from "@/features/alerts/api";
import { GatewaysPanel, useGateways } from "@/features/gateways/GatewaysPanel";
import { useMachines } from "@/features/machines/api";
import { MachinesTable } from "@/features/machines/MachinesTable";
import { useRules } from "@/features/rules/api";
import { Stat, fmt } from "@/shared/ui";
import { TrendChart } from "./TrendChart";

export function OverviewPage() {
  const { data: machines = [] } = useMachines();
  const { data: alerts = [] } = useOpenAlerts();
  const { data: gateways = [] } = useGateways();
  const { data: rules = [] } = useRules();
  const [selected, setSelected] = useState("cnc-02");
  const [metric, setMetric] = useState("temperature");
  const [range, setRange] = useState("15m");

  const online = machines.filter((m) => m.status !== "offline");
  const offline = machines.filter((m) => m.status === "offline");
  const power = online.reduce((sum, m) => sum + (m.values.power_kw ?? 0), 0);
  const throughput = gateways.reduce((sum, g) => sum + g.msg_per_sec, 0);
  const critical = alerts.filter((a) => a.severity === "critical").length;

  return (
    <>
      <h1 className="page-title">Overview</h1>
      <section className="grid grid--stats" aria-label="Key metrics">
        <Stat
          label="Machines online"
          value={online.length}
          unit={`/ ${machines.length}`}
          hint={offline.length ? <span className="text--warn">{offline.map((m) => m.name).join(", ")} offline</span> : "All reachable"}
        />
        <Stat label="Active power" value={fmt(power)} unit="kW" hint="Sum of running machines" />
        <Stat label="Ingest throughput" value={fmt(throughput)} unit="msg/s" hint={`${gateways.length} gateway(s)`} />
        <Stat
          label="Open alerts"
          value={alerts.length}
          tone={alerts.length ? "crit" : undefined}
          hint={`${critical} critical · ${alerts.length - critical} warning`}
        />
      </section>

      <TrendChart machine={selected} metric={metric} range={range} rules={rules} onMetric={setMetric} onRange={setRange} />

      <div className="grid grid--split">
        <MachinesTable machines={machines} selected={selected} onSelect={setSelected} />
        <div className="stack">
          <AlertsPanel alerts={alerts} />
          <GatewaysPanel gateways={gateways} />
        </div>
      </div>
    </>
  );
}
