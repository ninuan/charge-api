export function useAdminAutoRefresh(refresh: () => Promise<void>, intervalMs = 60_000) {
  let timer: number | undefined;

  function clear() {
    if (timer !== undefined) window.clearInterval(timer);
    timer = undefined;
  }

  function schedule() {
    clear();
    if (document.visibilityState === "visible") {
      timer = window.setInterval(() => void refresh(), intervalMs);
    }
  }

  function onVisibilityChange() {
    if (document.visibilityState === "visible") {
      void refresh();
      schedule();
    } else {
      clear();
    }
  }

  function start() {
    document.addEventListener("visibilitychange", onVisibilityChange);
    schedule();
  }

  function stop() {
    clear();
    document.removeEventListener("visibilitychange", onVisibilityChange);
  }

  return { start, stop };
}
