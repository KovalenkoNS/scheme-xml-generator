/* Список подключённых библиотек и состояние их проверки. */
import * as _generation_settings from "../generation/settings.js";
import * as _library_types from "./types.js";
import * as _shared_dom from "../shared/dom.js";
import { state } from "../shell/state.js";
// Показывает каталог подключённых библиотек и ошибки импорта; обновляет типы и доступные FBD-шаблоны.
export function renderLibrary() {
  _shared_dom.$("library-count").textContent = String(state.catalog.libraries.length);
  _shared_dom.$("library-list").replaceChildren();
  for (const library of state.catalog.libraries) { const row = _shared_dom.el("li"); row.append(_shared_dom.el("span", "", library.file), _shared_dom.el("small", "", `${library.typeCount ?? "—"} типов · ${library.templateCount} шаблонов`)); _shared_dom.$("library-list").append(row); }
  if (!state.catalog.libraries.length) _shared_dom.$("library-list").append(_shared_dom.el("li", "muted", "Библиотека не подключена"));
  for (const error of state.catalog.errors || []) _shared_dom.$("library-list").append(_shared_dom.el("li", "error", `${error.file}: ${error.message}`));
  _library_types.renderTypes(); _generation_settings.renderSettings();
};
