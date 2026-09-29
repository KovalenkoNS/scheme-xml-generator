/* Согласование библиотечных FBD-запросов и непересекающихся номеров POU. */
import * as _generation_fbd_document from "./document.js";
import * as _generation_fbd_modules from "./modules.js";
import * as _sources_model from "../../sources/model.js";
// Разделяет обычные библиотечные FBD и модульные DO-запросы; выделяет непересекающиеся номера POU.
export function fbdJobs(sources, plc, options, catalog) {
  const selected = _sources_model.kinds(sources, plc), jobs = [];
  if (selected.some(kind => kind !== "DO")) jobs.push({ kind: "FBD", url: "/api/generate", json: _generation_fbd_document.fbdRequest(sources, plc, options, catalog) });
  if (selected.includes("DO")) jobs.push({ kind: "FBD", url: "/api/generate/library-do", json: _generation_fbd_modules.libraryDORequest(sources, plc, options, catalog, jobs[0]?.json.pous.length || 0) });
  if (!jobs.length) throw new Error("В выбранном ПЛК нет данных для FBD.");
  return jobs;
};
