import { CategoryScale, Chart as ChartJS, Filler, LinearScale, LineElement, PointElement, Tooltip } from "chart.js";
import { Line } from "react-chartjs-2";
import { useHistory } from "@/features/machines/api";
import type { Rule } from "@/shared/api/types";
import { Card, Empty } from "@/shared/ui";

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Filler, Tooltip);

const METRICS = [
  { key: "temperature", label: "Temperature", unit: "°C" },
  { key: "vibration", label: "Vibration", unit: "mm/s" },
  { key: "power_kw", label: "Power", unit: "kW" },
  { key: "anomaly_score", label: "Anomaly score", unit: "" },
] as const;
const RANGES = ["15m", "1h", "6h", "24h"] as const;

interface Props {
  machine: string;
  metric: string;
  range: string;
  rules: Rule[];
  onMetric(m: string): void;
  onRange(r: string): void;
}

export function TrendChart({ machine, metric, range, rules, onMetric, onRange }: Props) {
  const { data = [], isLoading } = useHistory(machine, metric, range);
  const meta = METRICS.find((m) => m.key === metric) ?? METRICS[0];
  // Draw the threshold of the first enabled rule that watches this metric.
  const rule = rules.find((r) => r.enabled && r.metric === metric && (r.machine === "" || r.machine === machine));

  const labels = data.map((s) => new Date(s.t).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }));
  const chartData = {
    labels,
    datasets: [
      { data: data.map((s) => s.v), borderColor: "#4ea1ff", backgroundColor: "rgba(78,161,255,0.12)", fill: true, pointRadius: 0, borderWidth: 2, tension: 0.25 },
      ...(rule ? [{ data: data.map(() => rule.threshold), borderColor: "#ff8f8f", borderDash: [6, 5], pointRadius: 0, borderWidth: 1.5, fill: false }] : []),
    ],
  };

  return (
    <Card
      title={`${meta.label} · ${machine}`}
      action={
        <div className="toolbar">
          <div className="segmented" role="group" aria-label="Metric">
            {METRICS.map((m) => (
              <button key={m.key} aria-pressed={m.key === metric} onClick={() => onMetric(m.key)}>
                {m.label}
              </button>
            ))}
          </div>
          <div className="segmented" role="group" aria-label="Time range">
            {RANGES.map((r) => (
              <button key={r} aria-pressed={r === range} onClick={() => onRange(r)}>
                {r}
              </button>
            ))}
          </div>
        </div>
      }
    >
      <div className="chart">
        {isLoading ? (
          <Empty>Loading…</Empty>
        ) : data.length === 0 ? (
          <Empty>No data in this range yet.</Empty>
        ) : (
          <Line
            data={chartData}
            options={{
              animation: false,
              maintainAspectRatio: false,
              plugins: { tooltip: { mode: "index", intersect: false }, legend: { display: false } },
              scales: {
                x: { ticks: { color: "#9aa7b6", maxTicksLimit: 8 }, grid: { color: "#222b36" } },
                y: { ticks: { color: "#9aa7b6" }, grid: { color: "#222b36" }, title: { display: !!meta.unit, text: meta.unit, color: "#9aa7b6" } },
              },
            }}
          />
        )}
      </div>
    </Card>
  );
}
