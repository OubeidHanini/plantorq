import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/shared/api/client";
import type { Alert } from "@/shared/api/types";

export const alertKeys = {
  open: ["alerts", "open"] as const,
  history: ["alerts", "history"] as const,
};

export const useOpenAlerts = () => useQuery({ queryKey: alertKeys.open, queryFn: () => api<Alert[]>("/alerts?state=open") });

export const useAlertHistory = () => useQuery({ queryKey: alertKeys.history, queryFn: () => api<Alert[]>("/alerts?limit=200") });

export function useAcknowledge() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api<Alert>(`/alerts/${id}/ack`, { method: "POST" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["alerts"] }),
  });
}
