/* Переключение двух страниц и текущего ПЛК по прочитанным источникам. */
import * as _generation_settings from "../generation/settings.js";
import * as _shared_dom from "../shared/dom.js";
import * as _shell_preferences from "./preferences.js";
import { state } from "./state.js";
import * as _sources_model from "../sources/model.js";
// Переключает две страницы по URL; основная показывает фактическое отсутствие контракта данных Host.
export function page() {
  state.page = location.hash === "#local" ? "local" : "main";
  const local = state.page === "local";
  for (const link of document.querySelectorAll("[data-page]")) {
    if (link.dataset.page === state.page) link.setAttribute("aria-current", "page"); else link.removeAttribute("aria-current");
  }
  _shared_dom.$("remote-source").hidden = local; _shared_dom.$("local-source").hidden = !local; _shared_dom.$("library-open").hidden = local;
  _shared_dom.$("page-source").textContent = local ? "ЛОКАЛЬНЫЕ ДАННЫЕ" : "ДАННЫЕ HOST";
  _shared_dom.$("source-badge").textContent = local ? "Временный режим" : "Нет подключения";
  _shared_dom.$("validation").hidden = true; updateControllers();
};

// Обновляет выбор ПЛК по прочитанным источникам и согласует настройки областей с текущим выбором.
export function updateControllers() {
  const names = _sources_model.controllers(state.sources);
  if (!names.includes(state.plc)) state.plc = names[0] || "";
  _shared_dom.$("plc").replaceChildren();
  if (!names.length || state.page === "main") _shared_dom.$("plc").append(_shared_dom.option("", "Нет данных"));
  else names.forEach(name => _shared_dom.$("plc").append(_shared_dom.option(name, name)));
  _shared_dom.$("plc").value = state.page === "local" ? state.plc : "";
  _shared_dom.$("plc").disabled = state.page !== "local" || !names.length || state.busy;
  _shell_preferences.store(); _generation_settings.renderSettings(); _generation_settings.refreshAction();
};
