import { useState, type FormEvent } from "react";
import { useAuth } from "@/features/auth/AuthContext";
import { useMachines } from "@/features/machines/api";
import { ApiError } from "@/shared/api/client";
import type { Rule } from "@/shared/api/types";
import { Badge, Card, Empty } from "@/shared/ui";
import { useDeleteRule, useRules, useSaveRule, type RuleInput } from "./api";

const EMPTY: RuleInput = { name: "", machine: "", metric: "temperature", operator: ">", threshold: 0, for_seconds: 0, severity: "warning", enabled: true };

export function RulesPage() {
  const { can } = useAuth();
  const { data: rules = [], isLoading } = useRules();
  const del = useDeleteRule();
  const [editing, setEditing] = useState<(RuleInput & { id?: number }) | null>(null);
  const admin = can("admin");

  return (
    <>
      <div className="page-head">
        <h1 className="page-title">Alert rules</h1>
        {admin && (
          <button className="button button--primary" onClick={() => setEditing(EMPTY)}>
            + New rule
          </button>
        )}
      </div>
      {!admin && <p className="muted">Only administrators can change rules.</p>}
      {editing && <RuleForm initial={editing} onDone={() => setEditing(null)} />}
      <Card>
        {isLoading ? (
          <Empty>Loading…</Empty>
        ) : rules.length === 0 ? (
          <Empty>No rules defined.</Empty>
        ) : (
          <div className="table-scroll">
            <table className="table">
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Condition</th>
                  <th>Scope</th>
                  <th>Severity</th>
                  <th>State</th>
                  {admin && <th />}
                </tr>
              </thead>
              <tbody>
                {rules.map((r: Rule) => (
                  <tr key={r.id}>
                    <td>{r.name}</td>
                    <td className="mono">
                      {r.metric} {r.operator} {r.threshold}
                      {r.for_seconds > 0 && ` for ${r.for_seconds}s`}
                    </td>
                    <td className="muted">{r.machine || "all machines"}</td>
                    <td>
                      <Badge tone={r.severity === "critical" ? "crit" : r.severity === "warning" ? "warn" : "info"}>{r.severity}</Badge>
                    </td>
                    <td>{r.enabled ? "enabled" : <span className="muted">disabled</span>}</td>
                    {admin && (
                      <td className="actions">
                        <button className="button" onClick={() => setEditing(r)}>
                          Edit
                        </button>
                        <button className="button button--danger" onClick={() => confirm(`Delete rule “${r.name}”?`) && del.mutate(r.id)}>
                          Delete
                        </button>
                      </td>
                    )}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>
    </>
  );
}

function RuleForm({ initial, onDone }: { initial: RuleInput & { id?: number }; onDone(): void }) {
  const [rule, setRule] = useState(initial);
  const save = useSaveRule();
  const { data: machines = [] } = useMachines();
  const set = <K extends keyof RuleInput>(k: K, v: RuleInput[K]) => setRule((r) => ({ ...r, [k]: v }));

  async function submit(e: FormEvent) {
    e.preventDefault();
    await save.mutateAsync(rule).then(onDone, () => undefined);
  }

  return (
    <Card title={rule.id ? "Edit rule" : "New rule"}>
      <form className="form-grid" onSubmit={submit}>
        <label className="field">
          Name
          <input value={rule.name} onChange={(e) => set("name", e.target.value)} required maxLength={120} />
        </label>
        <label className="field">
          Machine
          <select value={rule.machine} onChange={(e) => set("machine", e.target.value)}>
            <option value="">All machines</option>
            {machines.map((m) => (
              <option key={m.id} value={m.id}>
                {m.name}
              </option>
            ))}
          </select>
        </label>
        <label className="field">
          Metric
          <input value={rule.metric} onChange={(e) => set("metric", e.target.value)} required />
        </label>
        <label className="field">
          Operator
          <select value={rule.operator} onChange={(e) => set("operator", e.target.value as Rule["operator"])}>
            {([">", ">=", "<", "<="] as const).map((o) => (
              <option key={o}>{o}</option>
            ))}
          </select>
        </label>
        <label className="field">
          Threshold
          <input type="number" step="any" value={rule.threshold} onChange={(e) => set("threshold", Number(e.target.value))} required />
        </label>
        <label className="field">
          For (seconds)
          <input type="number" min={0} max={86400} value={rule.for_seconds} onChange={(e) => set("for_seconds", Number(e.target.value))} />
        </label>
        <label className="field">
          Severity
          <select value={rule.severity} onChange={(e) => set("severity", e.target.value as Rule["severity"])}>
            <option value="info">info</option>
            <option value="warning">warning</option>
            <option value="critical">critical</option>
          </select>
        </label>
        <label className="field field--inline">
          <input type="checkbox" checked={rule.enabled} onChange={(e) => set("enabled", e.target.checked)} />
          Enabled
        </label>
        {save.error && <p className="text--crit form-grid__full">{save.error instanceof ApiError ? save.error.message : "Save failed"}</p>}
        <div className="form-grid__full actions">
          <button className="button button--primary" disabled={save.isPending}>
            Save
          </button>
          <button type="button" className="button" onClick={onDone}>
            Cancel
          </button>
        </div>
      </form>
    </Card>
  );
}
