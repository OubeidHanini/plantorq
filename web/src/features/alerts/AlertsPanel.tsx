import { Link } from "react-router-dom";
import { useAuth } from "@/features/auth/AuthContext";
import { Card, Empty, fmt, timeAgo } from "@/shared/ui";
import type { Alert } from "@/shared/api/types";
import { useAcknowledge } from "./api";

export function AlertItem({ alert }: { alert: Alert }) {
  const { can } = useAuth();
  const ack = useAcknowledge();
  const tone = alert.severity === "critical" ? "crit" : "warn";
  return (
    <li className={`alert alert--${tone}`}>
      <span className={`alert__tag alert__tag--${tone}`}>{alert.severity === "critical" ? "CRIT" : "WARN"}</span>
      <div className="alert__body">
        <strong>
          {alert.machine} · {alert.metric} {fmt(alert.value, 2)} {alert.operator} {alert.threshold}
        </strong>
        <span className="muted small">
          {alert.resolved_at ? `Resolved ${timeAgo(alert.resolved_at)}` : `Firing since ${timeAgo(alert.started_at)}`} · rule “{alert.rule_name}”
          {alert.acked_at && " · acknowledged"}
        </span>
      </div>
      {!alert.resolved_at && !alert.acked_at && can("operator") && (
        <button className="button" onClick={() => ack.mutate(alert.id)} disabled={ack.isPending}>
          Acknowledge
        </button>
      )}
    </li>
  );
}

export function AlertsPanel({ alerts }: { alerts: Alert[] }) {
  return (
    <Card title="Live alerts" action={<Link to="/alerts">History</Link>}>
      {alerts.length === 0 ? (
        <Empty>All machines within limits.</Empty>
      ) : (
        <ul className="alert-list">
          {alerts.slice(0, 6).map((a) => (
            <AlertItem key={a.id} alert={a} />
          ))}
        </ul>
      )}
    </Card>
  );
}
