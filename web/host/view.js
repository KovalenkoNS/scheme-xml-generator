/* Компактное представление соединения Host; доступность IO для генерации не выдаётся за авторизацию. */
import { $ } from "../shared/dom.js";

let current = { status: "checking", connected: false, message: "Проверка соединения" };

// Применяет снимок текущего соединения к основной странице; значения выводятся только как текст.
export function updateSession(snapshot) { current = snapshot; renderSession(); }

// Восстанавливает фактический статус после переключения двух страниц, сохраняя метку локального режима.
export function renderSession() {
  $("host-title").textContent = current.message;
  $("host-address").textContent = [current.server, current.username].filter(Boolean).join(" · ");
  $("host-address").hidden = !current.server && !current.username;
  $("host-connection").dataset.status = current.connected ? "connected" : "disconnected";
  $("host-note").textContent = current.connected
    ? "Соединение активно. Получение IO-данных для генерации ещё не подключено."
    : "Локальная страница доступна без подключения к серверу.";
  if (!$("remote-source").hidden) $("source-badge").textContent = current.connected ? "Подключено" : current.message;
}
