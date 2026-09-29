/* Поля настроек выпуска: изменение текущего значения и сохранение предпочтений. */
import { el } from "../shared/dom.js";
import * as _shell_preferences from "../shell/preferences.js";
// Создаёт поле текущей настройки и сохраняет изменения через переданный обработчик и локальные предпочтения.
export function input(value, label, onChange, type = "text") {
  const node = el("input"); node.type = type; node.value = value ?? ""; node.setAttribute("aria-label", label);
  if (type === "number") { node.min = "0"; node.max = "2147483647"; node.step = "1"; }
  node.addEventListener("input", () => { onChange(node.value); _shell_preferences.store(); });
  return node;
};
