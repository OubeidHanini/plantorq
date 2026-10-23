import { useQuery, type QueryClient } from "@tanstack/react-query";
import { api } from "@/shared/api/client";
import type { Machine, Reading, Sample } from "@/shared/api/types";

export const machineKeys = {
  all: ["machines"] as const,
  history: (machine: string, metric: string, range: string) => ["history", machine, metric, range] as const,
};

export function useMachines() {
  // Live values arrive over WebSocket; the periodic refetch only refreshes
  // derived status (offline detection, alert severity).
  return useQuery({ queryKey: machineKeys.all, queryFn: () => api<Machine[]>("/machines"), refetchInterval: 10_000 });
}

export function useHistory(machine: string, metric: string, range: string) {
  return useQuery({
    queryKey: machineKeys.history(machine, metric, range),
    queryFn: () => api<Sample[]>(`/machines/${machine}/history?metric=${metric}&range=${range}`),
    refetchInterval: 15_000,
    enabled: !!machine,
  });
}

/** Merges a live reading into the cached machine list (no refetch needed). */
export function applyReading(qc: QueryClient, r: Reading) {
  qc.setQueryData<Machine[]>(machineKeys.all, (machines) =>
    machines?.map((m) =>
      m.id === r.machine
        ? { ...m, values: { ...m.values, ...r.values }, last_seen: r.source === "edge" ? r.ts : m.last_seen, status: m.status === "offline" && r.source === "edge" ? "running" : m.status }
        : m,
    ),
  );
}
