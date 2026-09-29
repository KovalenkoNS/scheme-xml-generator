/* Начальная загрузка каталога и конфигурации с сохранением текущих предпочтений. */
import * as _shell_status from "./status.js";
import * as _library_catalog from "../library/catalog.js";
import * as _shared_http from "../shared/http.js";
import * as _shell_bindings from "./bindings.js";
import * as _shell_navigation from "./navigation.js";
import * as _shell_preferences from "./preferences.js";
import { state } from "./state.js";
// Читает фактическую конфигурацию и каталог сервера; объединяет их с локальными предпочтениями перед включением выпуска.
export async function init() {
  _shell_bindings.bind(); _shell_navigation.page();
  const responses = await Promise.allSettled([_shared_http.request("/api/workspace"), _shared_http.request("/api/templates")]);
  if (responses[0].status === "fulfilled") {
    const { common, page: configPage } = responses[0].value;
    const base = { version: common.version, project: common.project, controllerId: common.controllerId, resourceId: common.resourceId, groupId: configPage.groupId, pouNumber: configPage.pouNumber };
    state.contexts.fbd = { ...base, ...state.contexts.fbd };
    state.contexts.st = { ...base, pouNumber: String(Number(configPage.pouNumber) + 1000), ...state.contexts.st };
    state.contexts.hmi = { version: common.version, project: common.project, resourceNumber: "1", ...state.contexts.hmi };
    state.loaded = true;
  } else _shell_status.message(responses[0].reason.message, true);
  if (responses[1].status === "fulfilled") state.catalog = responses[1].value; else _shell_status.message(responses[1].reason.message, true);
  _library_catalog.renderLibrary(); _shell_navigation.updateControllers(); _shell_preferences.store();
};
init();
