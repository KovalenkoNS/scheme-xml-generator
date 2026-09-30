/* Настройки диагностического HMI по инвентарю выбранного ПЛК и поддержанному экспортному профилю. */
import * as _equipment_controllers from "../../equipment/controllers.js";
import * as _generation_context from "../context.js";
import * as _generation_panel from "../panel.js";
import * as _shared_dom from "../../shared/dom.js";
import { state } from "../../shell/state.js";
// Показывает контекст HMI и границы подтверждённого CPU-профиля; не изображает поддержку неподтверждённого 850.
export function hmiPanel() {
  const result = _generation_panel.panel("hmi", "HMI", "Кадры диагностики");
  result.append(_shared_dom.el("p", "settings-note", state.cpu === _equipment_controllers.CPU715 ? "Диагностике нужны исходный инвентарь FCS или карта AO. Связь прочитанного SCS-листа с кадрами ещё не реализована." : "Образец 850_DIAG.xml получен. Профиль HMI 850 ещё не реализован; FBD и ST можно сформировать отдельно."));
  result.append(_generation_context.contextEditor("hmi")); return result;
};
