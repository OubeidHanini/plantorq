import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, json } from "@/shared/api/client";
import type { Rule } from "@/shared/api/types";

export const ruleKeys = { all: ["rules"] as const };

export const useRules = () => useQuery({ queryKey: ruleKeys.all, queryFn: () => api<Rule[]>("/rules") });

export type RuleInput = Omit<Rule, "id">;

export function useSaveRule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...rule }: RuleInput & { id?: number }) =>
      id ? api<Rule>(`/rules/${id}`, { method: "PUT", ...json(rule) }) : api<Rule>("/rules", { method: "POST", ...json(rule) }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ruleKeys.all }),
  });
}

export function useDeleteRule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api<void>(`/rules/${id}`, { method: "DELETE" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ruleKeys.all }),
  });
}
