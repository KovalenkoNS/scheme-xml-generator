/* Текущее состояние двух страниц и предпочтения; файлы источников остаются только в памяти. */
import * as _equipment_controllers from "../equipment/controllers.js";
import * as _sources_model from "../sources/model.js";
export const STORAGE = "xml-generator.workspace.v1";
let saved = {};
try { saved = JSON.parse(localStorage.getItem(STORAGE) || "{}"); } catch { /* A broken preference must not prevent starting the app. */ }
export const state = {
  page: "main", busy: false, sources: [], catalog: { libraries: [], templates: [], types: [], errors: [] },
  plc: saved.plc || "", cpu: [_equipment_controllers.CPU715, _equipment_controllers.CPU850].includes(saved.cpu) ? saved.cpu : _equipment_controllers.CPU715,
  templates: saved.templates || {}, inversion: saved.inversion || {}, modules: _sources_model.migrateModuleSettings(saved.modules),
  contexts: saved.contexts || {}, outputs: Array.isArray(saved.outputs) ? saved.outputs : ["fbd"],
  profile: saved.profile || "", fileName: saved.fileName || "SCADA", loaded: false,
};
export const contextFields = [["version", "Версия XML"], ["controllerId", "ControllerID"], ["resourceId", "ResuorceID"], ["groupId", "GroupID"], ["pouNumber", "Первый POUNum"], ["project", "Проект в SCADA"]];
