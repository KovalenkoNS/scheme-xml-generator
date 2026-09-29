/* Редактор фактических ModuleID, общий для ST и библиотечного DO. */
import * as _generation_fields from "./fields.js";
import * as _generation_settings from "./settings.js";
import * as _shared_dom from "../shared/dom.js";
import { state } from "../shell/state.js";
import * as _sources_model from "../sources/model.js";
// Редактирует фактические ModuleID для FBD DO и ST; хранит значения по источнику/группе без вывода ID из имени.
export function moduleEditor(group) {
    const key = _sources_model.groupID(group), record = state.modules[key] ||= { count: group.modules.length, ids: [] };
    const details = _shared_dom.el("details", "module-details"), summary = _shared_dom.el("summary");
    summary.textContent = `${group.kind} · ${group.pouName || group.name} · ${record.count} мод. · ${group.source.file.name}`; details.append(summary);
    const head = _shared_dom.el("div", "module-head"); head.append(_shared_dom.el("span", "settings-note", "ModuleID из конфигурации ПЛК"));
    const countInput = _generation_fields.input(record.count, "Количество модулей", value => { record.count = value; }); countInput.type = "number"; countInput.min = group.modules.length; countInput.max = "128";
    countInput.addEventListener("change", () => _generation_settings.renderSettings()); head.append(_shared_dom.field("Модулей", countInput)); details.append(head);
    const grid = _shared_dom.el("div", "module-grid");
    // Обновляет поля группы по сохранённым ModuleID; дополнительные позиции не получают выдуманных ID.
    function drawIDs() {
      grid.replaceChildren(); const count = Number(record.count);
      if (!Number.isInteger(count) || count < group.modules.length || count > 128) { grid.append(_shared_dom.el("p", "error", `Количество: от ${group.modules.length} до 128.`)); return; }
      for (let index = 0; index < count; index++) {
        const label = group.modules[index]?.name || `Дополнительный ${index - group.modules.length + 1}`;
        const node = _generation_fields.input(record.ids[index] ?? "", `${label} · ModuleID`, value => {
          record.ids[index] = value;
          for (const peer of document.querySelectorAll("input[data-module-key]")) if (peer.dataset.moduleKey === key && peer.dataset.channelIndex === String(index)) peer.value = value;
        }, "number");
        node.dataset.moduleKey = key; node.dataset.channelIndex = String(index); node.placeholder = "ID"; grid.append(_shared_dom.field(label, node));
      }
    }
    drawIDs(); details.append(grid); return details;
};
