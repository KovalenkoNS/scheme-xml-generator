/* Загрузка локальных таблиц в адаптеры API, состояние разбора и список выбранных исходников. */
import * as _generation_settings from "../generation/settings.js";
import * as _shared_dom from "../shared/dom.js";
import * as _shared_http from "../shared/http.js";
import * as _shell_navigation from "../shell/navigation.js";
import { state } from "../shell/state.js";
import * as _sources_model from "./model.js";
// Отправляет локальную таблицу подходящим IO-парсерам; сохраняет ПЛК, группы и замечания в модели источника.
export async function previewSource(source) {
  source.loading = true; renderSources();
  const paths = /\.(txt|tsv)$/i.test(source.file.name)
    ? [["ao", "/api/temporary/ao/preview"]]
    : [["assignments", "/api/mappings/preview"], ["io", "/api/temporary/diagnostic/preview"]];
  const results = await Promise.allSettled(paths.map(([, url]) => _shared_http.request(url, { method: "POST", body: _shared_http.uploadBody(source.file), signal: source.abort.signal })));
  if (!state.sources.includes(source)) return;
  const errors = [];
  results.forEach((result, index) => { if (result.status === "fulfilled") source[paths[index][0]] = result.value; else errors.push(result.reason.message); });
  source.loading = false;
  if (!source.ao && !source.assignments && !source.io) source.error = [...new Set(errors)].join(" ");
  source.warnings = [...new Set([...(source.ao?.warnings || []), ...(source.assignments?.warnings || []), ...(source.io?.warnings || [])])];
  renderSources(); _shell_navigation.updateControllers();
};

// Показывает загруженные локальные файлы, найденные ПЛК и ошибки; удаление отменяет незавершённое чтение.
export function renderSources() {
  _shared_dom.$("source-list").replaceChildren(); _shared_dom.$("source-empty").hidden = state.sources.length > 0;
  for (const source of state.sources) {
    const row = _shared_dom.el("li"), info = _shared_dom.el("div", "file-info");
    info.append(_shared_dom.el("strong", "", source.file.name));
    const names = _sources_model.controllers([source]);
    info.append(_shared_dom.el("small", source.error ? "file-error" : "", source.loading ? "Чтение…" : source.error || `${names.length} ПЛК · ${names.join(", ")}`));
    if (source.warnings?.length) {
      const details = _shared_dom.el("details", "settings-note"); details.append(_shared_dom.el("summary", "", `Замечания: ${source.warnings.length}`));
      const list = _shared_dom.el("ul"); source.warnings.forEach(warning => list.append(_shared_dom.el("li", "", warning))); details.append(list); info.append(details);
    }
    const remove = _shared_dom.el("button", "icon-button", "×"); remove.type = "button"; remove.disabled = state.busy; remove.setAttribute("aria-label", `Убрать ${source.file.name}`);
    remove.addEventListener("click", () => { source.abort.abort(); state.sources = state.sources.filter(item => item !== source); renderSources(); _shell_navigation.updateControllers(); });
    row.append(_shared_dom.el("span", "file-icon", /\.xlsx$/i.test(source.file.name) ? "XLSX" : "TXT"), info, remove); _shared_dom.$("source-list").append(row);
  }
  _generation_settings.refreshAction();
};
