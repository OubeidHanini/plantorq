import { Card, Empty } from "@/shared/ui";
import { useAlertHistory } from "./api";
import { AlertItem } from "./AlertsPanel";

export function AlertsPage() {
  const { data = [], isLoading } = useAlertHistory();
  return (
    <>
      <h1 className="page-title">Alert history</h1>
      <Card>
        {isLoading ? (
          <Empty>Loading…</Empty>
        ) : data.length === 0 ? (
          <Empty>No alerts yet.</Empty>
        ) : (
          <ul className="alert-list">
            {data.map((a) => (
              <AlertItem key={a.id} alert={a} />
            ))}
          </ul>
        )}
      </Card>
    </>
  );
}
