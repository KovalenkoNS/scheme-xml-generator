/* Краткий статус операции рабочей области, без сетевой трассировки. */
import { $ } from "../shared/dom.js";
// Показывает краткий статус операции в рабочей области; принимает текст и признак ошибки.
export function message(text, error = false) { $("status").textContent = text; $("status").style.color = error ? "var(--error)" : ""; };
