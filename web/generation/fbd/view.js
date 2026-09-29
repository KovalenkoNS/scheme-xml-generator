/* Настройки FBD выбранного ПЛК: библиотечный шаблон, размещение и независимая инверсия DO. */
import * as _generation_context from "../context.js";
import * as _generation_modules from "../modules.js";
import * as _generation_panel from "../panel.js";
import * as _shared_dom from "../../shared/dom.js";
import * as _shell_preferences from "../../shell/preferences.js";
import { state } from "../../shell/state.js";
import * as _sources_model from "../../sources/model.js";
// Связывает направления IO с реальными ключами библиотеки; показывает переключатель NOT и физические ID DO.
export function fbdPanel() {
  const result = _generation_panel.panel("fbd", "FBD", "Из библиотеки");
  const kinds = _sources_model.kinds(state.sources, state.plc);
  if (!kinds.length) result.append(_shared_dom.el("p", "settings-note", "После выбора ПЛК укажите библиотечные шаблоны для его сигналов."));
  for (const kind of kinds) {
    const row = _shared_dom.el("div", "template-row"), select = _shared_dom.el("select"); select.setAttribute("aria-label", `${kind} · библиотечный шаблон`);
    select.append(_shared_dom.option("", "Выберите шаблон"));
    for (const template of state.catalog.templates.filter(item => item.supported)) select.append(_shared_dom.option(template.key, `${template.ownerName} · ${template.name} · ${template.libraryFile}`));
    select.value = state.templates[kind] || "";
    select.addEventListener("change", () => { state.templates[kind] = select.value; _shell_preferences.store(); });
    row.append(_shared_dom.el("strong", "", kind), select);
    if (kind === "DO") {
      const label = _shared_dom.el("label", "check-label"), checkbox = _shared_dom.el("input"); checkbox.type = "checkbox"; checkbox.checked = !!state.inversion[kind];
      checkbox.addEventListener("change", () => { state.inversion[kind] = checkbox.checked; _shell_preferences.store(); }); label.append(checkbox, document.createTextNode("Инверсия · NOT")); row.append(label);
    }
    result.append(row);
  }
  if (!state.catalog.templates.some(item => item.supported)) {
    const note = _shared_dom.el("p", "settings-note"); note.append(document.createTextNode("Нет поддержанных шаблонов. ")); const link = _shared_dom.el("a", "", "Подключить библиотеку на основной странице"); link.href = "#main"; note.append(link); result.append(note);
  }
  if (kinds.includes("DO")) {
    result.append(_shared_dom.el("p", "settings-note", "Один D32 на модуль. NOT включается между сигналами и входами D32. Аппаратный профиль DO подтверждён для 850."));
    for (const group of _sources_model.fbdGroups(state.sources, state.plc).filter(item => item.kind === "DO")) result.append(_generation_modules.moduleEditor(group));
  }
  result.append(_generation_context.contextEditor("fbd")); return result;
};
