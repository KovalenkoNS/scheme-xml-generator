/* Состав предметных настроек и доступность выпуска по текущему состоянию. */
import * as _generation_fbd_view from "./fbd/view.js";
import * as _generation_hmi_view from "./hmi/view.js";
import * as _generation_st_view from "./st/view.js";
import * as _shared_dom from "../shared/dom.js";
import { state } from "../shell/state.js";
// Собирает предметные панели только для выбранных FBD/ST/HMI; на основной странице ждёт данных Host.
export function renderSettings() {
  _shared_dom.$("settings").replaceChildren();
  if (!state.loaded) return;
  if (state.page === "main") { _shared_dom.$("settings").append(_shared_dom.el("p", "settings-note", "Настройки выбранного ПЛК будут доступны после подключения данных Host.")); return; }
  for (const kind of state.outputs) _shared_dom.$("settings").append(kind === "fbd" ? _generation_fbd_view.fbdPanel() : kind === "st" ? _generation_st_view.stPanel() : _generation_hmi_view.hmiPanel());
};

// Согласует доступность выпуска с чтением файлов, ПЛК и активной операцией; обновляет краткий статус действия.
export function refreshAction() {
  const loading = state.sources.some(source => source.loading), ready = state.page === "local" && state.plc && state.outputs.length && !loading && state.loaded;
  _shared_dom.$("generate").disabled = state.busy || !ready;
  _shared_dom.$("cpu").disabled = state.busy || state.page !== "local";
  _shared_dom.$("source-files").disabled = state.busy;
  _shared_dom.$("action-summary").textContent = state.busy ? "Формирование…" : state.page === "main" ? "Ожидает IO-данных Host" : loading ? "Чтение источников…" : state.plc ? `${state.plc} · ${state.outputs.map(kind => kind.toUpperCase()).join(" + ") || "выберите файлы"}` : "Выберите источник данных";
};
