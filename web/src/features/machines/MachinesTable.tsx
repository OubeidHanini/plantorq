import { Badge, Card, Empty, fmt, type Tone } from "@/shared/ui";
import type { Machine, MachineStatus } from "@/shared/api/types";

const statusTone: Record<MachineStatus, Tone> = { running: "info", warning: "warn", critical: "crit", offline: "muted" };

export function MachinesTable({ machines, selected, onSelect }: { machines: Machine[]; selected?: string; onSelect(id: string): void }) {
  return (
    <Card title="Machines">
      {machines.length === 0 ? (
        <Empty>No machines registered.</Empty>
      ) : (
        <div className="table-scroll">
          <table className="table">
            <thead>
              <tr>
                <th>Machine</th>
                <th>Protocol</th>
                <th>Status</th>
                <th className="num">Temp</th>
                <th className="num">Vibration</th>
                <th className="num">Power</th>
              </tr>
            </thead>
            <tbody>
              {machines.map((m) => {
                const offline = m.status === "offline";
                return (
                  <tr key={m.id} className={m.id === selected ? "is-selected" : ""}>
                    <td>
                      <button className="link-button" onClick={() => onSelect(m.id)} aria-pressed={m.id === selected}>
                        {m.name}
                      </button>
                    </td>
                    <td className="muted">{m.protocol}</td>
                    <td>
                      <Badge tone={statusTone[m.status]}>{m.status}</Badge>
                    </td>
                    <td className={`num mono ${m.status === "critical" ? "text--crit" : ""}`}>{offline ? "—" : `${fmt(m.values.temperature)} °C`}</td>
                    <td className="num mono">{offline ? "—" : `${fmt(m.values.vibration, 2)} mm/s`}</td>
                    <td className="num mono">{offline ? "—" : `${fmt(m.values.power_kw)} kW`}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </Card>
  );
}
