/* Связь жизненного цикла web-страницы с read-only снимком серверной сессии Host. */
import { request } from "../shared/http.js";
import { $ } from "../shared/dom.js";
import { createSessionMonitor } from "./monitor.js";
import { updateSession } from "./view.js";

// Включает опрос и обновление после возврата фокуса/ручной команды; не создаёт отдельную авторизацию.
export function startSession() {
  const monitor = createSessionMonitor({ read: signal => request("/api/host/session", { signal }), publish: updateSession });
  $("host-refresh").addEventListener("click", monitor.refresh);
  window.addEventListener("focus", monitor.refresh);
  document.addEventListener("visibilitychange", () => { if (!document.hidden) void monitor.refresh(); });
  window.addEventListener("pagehide", monitor.stop);
  window.addEventListener("pageshow", monitor.start);
  monitor.start();
}
