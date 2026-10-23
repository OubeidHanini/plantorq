import { useQueryClient } from "@tanstack/react-query";
import { useCallback } from "react";
import { useAuth } from "@/features/auth/AuthContext";
import { gatewayKeys } from "@/features/gateways/GatewaysPanel";
import { applyReading, machineKeys } from "@/features/machines/api";
import type { GatewayStatus, LiveEvent } from "@/shared/api/types";
import { useLiveEvents, type ConnectionState } from "@/shared/realtime/useLiveEvents";

/**
 * Bridges server push and the query cache: WebSocket events update cached
 * data in place, so every component reading that data re-renders without
 * knowing the WebSocket exists.
 */
export function useLiveSync(): ConnectionState {
  const { session } = useAuth();
  const qc = useQueryClient();

  const onEvent = useCallback(
    (e: LiveEvent) => {
      switch (e.type) {
        case "reading":
          applyReading(qc, e.data);
          break;
        case "gateway":
          qc.setQueryData<GatewayStatus[]>(gatewayKeys.all, (list = []) => [...list.filter((g) => g.gateway !== e.data.gateway), e.data]);
          break;
        case "alert":
          void qc.invalidateQueries({ queryKey: ["alerts"] });
          void qc.invalidateQueries({ queryKey: machineKeys.all }); // status depends on alerts
          break;
      }
    },
    [qc],
  );

  return useLiveEvents(session?.token ?? null, onEvent);
}
