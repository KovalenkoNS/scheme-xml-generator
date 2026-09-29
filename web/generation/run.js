/* Последовательный выпуск выбранных FBD/ST/HMI: согласование POUNum, HTTP-запросы и частичные результаты. */
import * as _shell_status from "../shell/status.js";
import * as _generation_fbd_jobs from "./fbd/jobs.js";
import * as _generation_hmi_model from "./hmi/model.js";
import * as _generation_results from "./results.js";
import * as _generation_settings from "./settings.js";
import * as _generation_st_model from "./st/model.js";
import * as _shared_dom from "../shared/dom.js";
import * as _shared_http from "../shared/http.js";
import * as _shell_preferences from "../shell/preferences.js";
import { state } from "../shell/state.js";
import * as _sources_local from "../sources/local.js";
// Планирует и последовательно выполняет выпуск выбранных областей; предотвращает пересечение POUNum и сохраняет частичные результаты.
export async function generate(event) {
  event.preventDefault(); if (state.busy || state.page !== "local" || !state.plc) return;
  _shared_dom.$("validation").hidden = true; _shared_dom.$("result-list").replaceChildren(); _shared_dom.$("results").hidden = false;
  const jobs = [], failures = [];
  const common = { cpu: state.cpu, fileName: state.fileName.trim() || "SCADA", modules: state.modules, profile: state.profile, templates: state.templates, inversion: state.inversion };
  for (const kind of state.outputs) {
    try {
      const options = { ...common, context: state.contexts[kind] };
      if (kind === "fbd") jobs.push(..._generation_fbd_jobs.fbdJobs(state.sources, state.plc, options, state.catalog));
      else jobs.push(...(kind === "st" ? _generation_st_model.stJobs(state.sources, state.plc, options) : _generation_hmi_model.hmiJobs(state.sources, state.plc, options)));
    } catch (error) { failures.push({ kind: kind.toUpperCase(), message: error.message }); }
  }
  // A joint import must not reuse POUNum between the selected FBD and ST.
  for (const fbd of jobs.filter(job => job.kind === "FBD")) {
    const first = fbd.json.pous[0].pouNumber, last = first + fbd.json.pous.length - 1;
    for (const job of jobs.filter(item => item.kind === "ST")) {
      const start = Number(job.fields.context.pouNumber), count = (job.fields.st || job.fields.config).pous.length;
      if (start <= last && start + count - 1 >= first) { failures.push({ kind: "ST", message: "Диапазоны POUNum FBD и ST пересекаются. Измените первый POUNum в контексте ST." }); job.skip = true; }
    }
  }
  failures.forEach(failure => _generation_results.resultError(failure.kind, failure.message));
  state.busy = true; _shared_dom.$("workspace-form").inert = true; _generation_settings.refreshAction(); _sources_local.renderSources();
  let files = 0, errors = failures.length;
  for (const job of jobs.filter(item => !item.skip)) {
    _shell_status.message(`${job.kind} · ${state.plc}…`);
    try {
      const response = await _shared_http.request(job.url, job.json ? { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(job.json) } : { method: "POST", body: _shared_http.uploadBody(job.file, job.fields) });
      files += _generation_results.resultFiles(job.kind, response);
    } catch (error) { errors++; _generation_results.resultError(job.kind, error.message); }
  }
  state.busy = false; _shared_dom.$("workspace-form").inert = false; _generation_settings.refreshAction(); _sources_local.renderSources();
  _shared_dom.$("result-summary").textContent = `${files} файлов${errors ? ` · ${errors} ошибок` : ""}`;
  _shell_status.message(errors ? (files ? "Выпуск частичный. Проверьте ошибки выбранных областей." : "Файлы не сформированы. Исправьте указанные причины.") : "Файлы готовы.", errors > 0); _shell_preferences.store();
};
