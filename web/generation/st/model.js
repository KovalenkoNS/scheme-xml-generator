/* Чистая модель generation/st/model.js: подготовка данных запроса без DOM, сети и записи файлов. */
import * as _generation_st_context from "./context.js";
import * as _equipment_controllers from "../../equipment/controllers.js";
import * as _sources_model from "../../sources/model.js";
// Планирует ST через API назначений; проверяет профиль CPU и реальные уникальные ModuleID выбранного ПЛК.
export function stJobs(sources, plc, options) {
  const selected = _sources_model.groups(sources, plc);
  if (!selected.length) throw new Error("Для ST нужна подготовленная карта AI/AO либо IO-лист DI/DO с назначениями.");
  if (options.cpu === _equipment_controllers.CPU850 && selected.some(group => group.family === "ao")) throw new Error("Текущий профиль AO ST для 850 не реализован.");
  if (options.cpu === _equipment_controllers.CPU715 && selected.some(group => group.kind === "DI")) throw new Error("Текущий профиль DI ST для 715 не подтверждён.");
  const allIDs = new Set(), jobs = []; let offset = 0;
  for (const source of sources) {
    for (const family of ["ao", "assignments"]) {
      const subset = selected.filter(group => group.source === source && group.family === family);
      if (!subset.length) continue;
      const choices = subset.map(group => _sources_model.moduleSettings(group, options.modules));
      for (const choice of choices) for (const id of choice.moduleIds) {
        if (allIDs.has(id)) throw new Error(`ModuleID ${id} повторяется внутри выбранного ПЛК.`);
        allIDs.add(id);
      }
      const context = _generation_st_context.mappingContext(options.context, options.cpu, options.profile, offset);
      const config = family === "ao" ? { pous: choices } : { kind: "st", pous: choices };
      jobs.push({ kind: "ST", file: source.file, url: family === "ao" ? "/api/temporary/ao/generate-st" : "/api/mappings/generate", fields: {
        fileName: `${options.fileName}_${family}_${jobs.length + 1}`, context, [family === "ao" ? "st" : "config"]: config,
      } });
      offset += subset.length;
    }
  }
  return jobs;
};
