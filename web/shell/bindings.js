/* Регистрация пользовательских действий оболочки с делегированием предметным компонентам. */
import * as _generation_run from "../generation/run.js";
import * as _generation_settings from "../generation/settings.js";
import * as _library_catalog from "../library/catalog.js";
import * as _library_import from "../library/import.js";
import * as _library_types from "../library/types.js";
import * as _shared_dom from "../shared/dom.js";
import * as _shared_http from "../shared/http.js";
import * as _shell_navigation from "./navigation.js";
import * as _shell_preferences from "./preferences.js";
import { state } from "./state.js";
import * as _sources_local from "../sources/local.js";
// Связывает действия оболочки с предметными компонентами источников, библиотеки и выпуска; инициализирует текущие поля.
export function bind() {
  window.addEventListener("hashchange", _shell_navigation.page);
  _shared_dom.$("source-files").addEventListener("change", () => {
    for (const file of _shared_dom.$("source-files").files) {
      const id = `${file.name}|${file.size}|${file.lastModified}`;
      if (state.sources.some(source => source.id === id)) continue;
      const source = { id, file, abort: new AbortController() }; state.sources.push(source); _sources_local.previewSource(source);
    }
    _shared_dom.$("source-files").value = "";
  });
  _shared_dom.$("plc").addEventListener("change", () => { state.plc = _shared_dom.$("plc").value; _shell_preferences.store(); _generation_settings.renderSettings(); _generation_settings.refreshAction(); });
  _shared_dom.$("cpu").value = state.cpu; _shared_dom.$("cpu").addEventListener("change", () => { state.cpu = _shared_dom.$("cpu").value; _shell_preferences.store(); _generation_settings.renderSettings(); });
  document.querySelectorAll("input[name=output]").forEach(checkbox => { checkbox.checked = state.outputs.includes(checkbox.value); checkbox.addEventListener("change", () => { state.outputs = [...document.querySelectorAll("input[name=output]:checked")].map(item => item.value); _shell_preferences.store(); _generation_settings.renderSettings(); _generation_settings.refreshAction(); }); });
  _shared_dom.$("release-name").value = state.fileName; _shared_dom.$("release-name").addEventListener("input", () => { state.fileName = _shared_dom.$("release-name").value; _shell_preferences.store(); });
  _shared_dom.$("workspace-form").addEventListener("submit", _generation_run.generate);
  _shared_dom.$("library-open").addEventListener("click", () => _shared_dom.$("library-dialog").showModal());
  _shared_dom.$("library-close").addEventListener("click", () => _shared_dom.$("library-dialog").close());
  _shared_dom.$("library-search").addEventListener("input", _library_types.renderTypes);
  _shared_dom.$("library-file").addEventListener("change", _library_import.importLibrary);
  _shared_dom.$("library-refresh").addEventListener("click", async () => { try { state.catalog = await _shared_http.request("/api/refresh", { method: "POST" }); _library_catalog.renderLibrary(); _shared_dom.$("library-status").textContent = "Обновлено"; } catch (error) { _shared_dom.$("library-status").textContent = error.message; } });
};
