/* Редактор текущих транспортных констант выбранной области XML. */
import * as _generation_fields from "./fields.js";
import * as _shared_dom from "../shared/dom.js";
import { state, contextFields } from "../shell/state.js";
// Показывает текущие константы XML выбранной области; изменения относятся только к будущему запросу выпуска.
export function contextEditor(kind) {
  const context = state.contexts[kind] || {};
  const details = _shared_dom.el("details", "context"), summary = _shared_dom.el("summary");
  // Показывает текущие постоянные ID выбранного результата даже при закрытой панели настроек.
  const updateSummary = () => { summary.textContent = kind === "hmi" ? `Контекст · ресурс ${context.resourceNumber ?? "—"}` : `Контекст · GroupID ${context.groupId ?? "—"} · POUNum ${context.pouNumber ?? "—"}`; };
  updateSummary(); details.append(summary);
  const grid = _shared_dom.el("div", "context-grid");
  const fields = kind === "hmi" ? [["version", "Версия XML"], ["resourceNumber", "Номер ресурса"], ["project", "Проект в SCADA"]] : contextFields;
  for (const [key, label] of fields) {
    const node = _generation_fields.input(context[key], label, value => { context[key] = value; updateSummary(); }, key === "project" ? "text" : "number");
    grid.append(_shared_dom.field(label, node, key === "project"));
  }
  details.append(grid); return details;
};
