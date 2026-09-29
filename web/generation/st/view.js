/* Настройки ST выбранного ПЛК: физические ID модулей, профиль адресации и контекст XML. */
import * as _equipment_controllers from "../../equipment/controllers.js";
import * as _generation_context from "../context.js";
import * as _generation_modules from "../modules.js";
import * as _generation_panel from "../panel.js";
import * as _shared_dom from "../../shared/dom.js";
import * as _shell_preferences from "../../shell/preferences.js";
import { state } from "../../shell/state.js";
import * as _sources_model from "../../sources/model.js";
// Показывает настройки адресации ST и физические модули выбранного ПЛК; значения используются HTTP-планировщиком.
export function stPanel() {
  const result = _generation_panel.panel("st", "ST", "Физические назначения");
  const profile = _shared_dom.el("select"); profile.id = "physical-profile";
  profile.append(_shared_dom.option("", `Авто · ${state.cpu === _equipment_controllers.CPU715 ? "IU / QU" : "Measurement / Quality"}`), _shared_dom.option("legacy-iu-qu", "IU / QU"), _shared_dom.option("measurement-quality", "Measurement / Quality")); profile.value = state.profile;
  profile.addEventListener("change", () => { state.profile = profile.value; _shell_preferences.store(); });
  result.append(_shared_dom.field("Адресация", profile));
  const groups = _sources_model.groups(state.sources, state.plc);
  if (!groups.length) result.append(_shared_dom.el("p", "settings-note", "Нужны подготовленные перекладки или назначения DI/DO в IO-листе."));
  for (const group of groups) result.append(_generation_modules.moduleEditor(group));
  result.append(_generation_context.contextEditor("st")); return result;
};
