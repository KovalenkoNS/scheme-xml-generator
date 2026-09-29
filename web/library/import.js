/* Действие подключения выбранного XML через проверяющий серверный API. */
import * as _library_catalog from "./catalog.js";
import * as _shared_dom from "../shared/dom.js";
import * as _shared_http from "../shared/http.js";
import { state } from "../shell/state.js";
// Загружает выбранную XML-библиотеку через проверяющий серверный API; обновляет каталог после успешного подключения.
export async function importLibrary() {
  const file = _shared_dom.$("library-file").files[0]; if (!file) return;
  _shared_dom.$("library-file").disabled = true; _shared_dom.$("library-refresh").disabled = true; _shared_dom.$("library-status").textContent = "Проверка и подключение…";
  try { const result = await _shared_http.request("/api/libraries/import", { method: "POST", body: _shared_http.uploadBody(file) }); state.catalog = result.catalog; _library_catalog.renderLibrary(); _shared_dom.$("library-status").textContent = result.created ? "Подключена" : "Уже подключена"; }
  catch (error) { _shared_dom.$("library-status").textContent = error.message; }
  finally { _shared_dom.$("library-file").disabled = false; _shared_dom.$("library-refresh").disabled = false; _shared_dom.$("library-file").value = ""; }
};
