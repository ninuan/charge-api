import { onBeforeUnmount, ref } from "vue";
import type { DashboardSnapshot } from "@/types/dashboard";

export function useDashboardStream(onSnapshot: (snapshot: DashboardSnapshot) => void) {
  const state = ref<"connected" | "connecting" | "offline">("connecting");
  let source: EventSource | null = null;

  function connect() {
    source?.close();
    state.value = "connecting";
    source = new EventSource("/api/stream", { withCredentials: true });
    source.addEventListener("open", () => {
      state.value = "connected";
    });
    source.addEventListener("snapshot", (event) => {
      onSnapshot(JSON.parse((event as MessageEvent).data) as DashboardSnapshot);
    });
    source.addEventListener("error", () => {
      state.value = "offline";
    });
  }

  onBeforeUnmount(() => source?.close());

  return { state, connect };
}
