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
  const errors = [];
  for (const [family, url] of paths) {
    try {
      source[family] = await _shared_http.request(url, { method: "POST", body: _shared_http.uploadBody(source.file), signal: source.abort.signal });
      // Формат SCS уже прочитан целиком: обработчик FCS не является вторым источником его сигналов.
      if (source.assignments?.source) break;
    } catch (error) { errors.push(error.message); }
    if (!state.sources.includes(source)) return;
  }
  if (!state.sources.includes(source)) return;
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
    if (source.assignments?.source) renderReadReport(info, source.assignments.source);
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

// Показывает состав фактически прочитанного IO: исходные строки отдельно от резервных размещений, без придуманных SCADA-имён.
// Подробности раскрываются по запросу; поиск и страницы сохраняют читаемость большой книги.
function renderReadReport(parent, report) {
  const records = report.records || [], excluded = report.excluded || [];
  const counts = Object.fromEntries(["DI", "DO", "AI", "AO"].map(kind => [kind, records.filter(record => record.kind === kind).length]));
  parent.append(_shared_dom.el("small", "io-counts", Object.entries(counts).map(([kind, count]) => `${kind} ${count}`).join(" · ")));
  const details = _shared_dom.el("details", "source-records"), summary = _shared_dom.el("summary", "", "Прочитанные строки");
  details.append(summary);
  const controls = _shared_dom.el("div", "source-record-controls"), search = _shared_dom.el("input"), kind = _shared_dom.el("select");
  search.type = "search"; search.placeholder = "Тег, ПЛК или модуль"; search.setAttribute("aria-label", "Найти исходную строку IO");
  kind.setAttribute("aria-label", "Направление IO"); kind.append(_shared_dom.option("", "Все IO"));
  for (const name of ["DI", "DO", "AI", "AO"]) kind.append(_shared_dom.option(name, `${name} · ${counts[name]}`));
  const previous = _shared_dom.el("button", "", "←"), next = _shared_dom.el("button", "", "→"), page = _shared_dom.el("span");
  previous.type = next.type = "button"; previous.setAttribute("aria-label", "Предыдущие строки"); next.setAttribute("aria-label", "Следующие строки");
  controls.append(search, kind, previous, page, next); details.append(controls);
  const viewport = _shared_dom.el("div", "source-record-viewport"), table = _shared_dom.el("table"), head = _shared_dom.el("thead"), header = _shared_dom.el("tr"), body = _shared_dom.el("tbody");
  for (const label of ["Строка", "IO / ПЛК", "Tag No / Loop", "Модули / канал", "Шкалы"]) { const cell = _shared_dom.el("th", "", label); cell.scope = "col"; header.append(cell); }
  head.append(header); table.append(head, body); viewport.append(table); details.append(viewport);
  if (excluded.length) {
    const reasons = new Map();
    for (const row of excluded) reasons.set(row.reason, (reasons.get(row.reason) || 0) + 1);
    details.append(_shared_dom.el("p", "settings-note", `${excluded.length} строк без физического IO: ${[...reasons].map(([reason, count]) => `${reason} — ${count}`).join("; ")}. Они не входят в счётчики сигналов.`));
  }
  let offset = 0;
  // Фильтрует только уже прочитанные записи и рисует до25 строк; значения попадают в textContent, не в HTML.
  function draw() {
    const needle = search.value.trim().toLowerCase();
    const selected = records.filter(record => (!kind.value || record.kind === kind.value) && (!needle || [record.signalName, record.loop, record.controllerName, record.description, ...record.placements.map(item => item.module)].join(" ").toLowerCase().includes(needle)));
    offset = Math.min(offset, Math.max(0, Math.floor((selected.length - 1) / 25) * 25)); body.replaceChildren();
    for (const record of selected.slice(offset, offset + 25)) {
      const row = _shared_dom.el("tr");
      const values = [`${record.sheet}:${record.row}`, `${record.kind}\n${record.controllerName}`, `${record.signalName}${record.reserve ? " · резерв" : ""}\n${record.loop || "—"}`, record.placements.map(item => `${item.module} / ${item.channel} (${item.role})`).join("\n"), (record.ranges || []).map((range, index) => `${index + 1}: ${range.minimum}…${range.maximum} ${range.unit}`).join("\n") || "—"];
      for (const value of values) row.append(_shared_dom.el("td", "", value));
      body.append(row);
    }
    page.textContent = selected.length ? `${offset + 1}–${Math.min(offset + 25, selected.length)} / ${selected.length}` : "Нет строк";
    previous.disabled = offset === 0; next.disabled = offset + 25 >= selected.length;
  }
  search.addEventListener("input", () => { offset = 0; draw(); }); kind.addEventListener("change", () => { offset = 0; draw(); });
  previous.addEventListener("click", () => { offset -= 25; draw(); }); next.addEventListener("click", () => { offset += 25; draw(); });
  details.addEventListener("toggle", () => { if (details.open) draw(); }); parent.append(details);
}
