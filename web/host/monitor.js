/* Жизненный цикл свежих снимков соединения: опрос, отмена, таймаут и защита от позднего ответа. */

// Создаёт независимый от DOM цикл чтения Host; publish получает только последний завершённый снимок.
// read принимает AbortSignal, а подменяемые часы позволяют проверить реальные гонки без ожидания таймеров.
export function createSessionMonitor({ read, publish, schedule = setTimeout, cancel = clearTimeout, interval = 3000, timeout = 4000 }) {
  let active = false, sequence = 0, polling = null, deadline = null, pending = null;

  // Прерывает старый HTTP-запрос и его часы перед новым действием или закрытием страницы.
  function clearPending() {
    if (polling !== null) cancel(polling);
    if (deadline !== null) cancel(deadline);
    polling = deadline = null;
    pending?.abort(); pending = null;
  }

  // Читает новый снимок; выход/смена адреса, замеченные новым запросом, не затираются старым ответом.
  async function refresh() {
    if (!active) return;
    const current = ++sequence;
    clearPending();
    const controller = new AbortController(); pending = controller;
    deadline = schedule(() => controller.abort(), timeout);
    try {
      const snapshot = await read(controller.signal);
      if (active && current === sequence && !controller.signal.aborted) publish(snapshot);
    } catch {
      if (active && current === sequence) publish({ status: "generator-unavailable", connected: false, available: false, message: "Не удалось обновить соединение" });
    } finally {
      if (active && current === sequence) {
        if (deadline !== null) cancel(deadline);
        deadline = null; pending = null;
        polling = schedule(refresh, interval);
      }
    }
  }

  // Запускает опрос однажды; первый снимок запрашивается сразу после открытия интерфейса.
  function start() { if (!active) { active = true; void refresh(); } }

  // Снимает таймеры и инвалидирует незавершённый ответ при уходе со страницы.
  function stop() { active = false; ++sequence; clearPending(); }

  return { start, stop, refresh };
}
