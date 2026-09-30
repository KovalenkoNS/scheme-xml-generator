/* Чистая модель generation/hmi/model.js: подготовка данных запроса без DOM, сети и записи файлов. */
import * as _sources_model from "../../sources/model.js";
import * as _equipment_controllers from "../../equipment/controllers.js";
import * as _shared_validation from "../../shared/validation.js";
// Планирует кадры HMI только выбранного ПЛК через диагностический API; явно отклоняет неподтверждённый профиль 850.
export function hmiJobs(sources, plc, options) {
  if (options.cpu !== _equipment_controllers.CPU715) throw new Error("Профиль HMI 850 по предоставленному 850_DIAG.xml ещё не реализован; текущая диагностика предназначена для 715.");
  if (sources.some(source => source.assignments?.source?.records.some(record => record.controllerName === plc))) throw new Error("Исходный SCS IO-лист прочитан. Для HMI ещё не определён полный аппаратный инвентарь этого ПЛК: типы крейтов, CPU/MI и размещение панелей. Привязка SCS-источника к диагностическим кадрам не реализована.");
  const matches = [];
  for (const source of sources) {
    const selected = (source.io?.controllers || []).filter(controller => _sources_model.nameOf(controller) === plc);
    if (selected.length) matches.push({ source, selected });
  }
  if (matches.length > 1) throw new Error("ПЛК найден в нескольких IO-листах. Оставьте один источник для HMI, чтобы избежать дубликатов кадров.");
  const context = { version: options.context.version, project: options.context.project, resourceNumber: options.context.resourceNumber };
  _shared_validation.integer(context.version, "Версия XML"); _shared_validation.integer(context.resourceNumber, "Номер ресурса");
  if (matches.length) {
    const { source, selected } = matches[0];
    return [{ kind: "HMI", file: source.file, url: "/api/temporary/diagnostic/generate", fields: { fileName: `${options.fileName}_HMI`, context, diagnostic: { controllers: selected.map(controller => ({ key: controller.key, name: plc })) } } }];
  }
  const aoSources = sources.filter(source => (source.ao?.groups || []).some(group => group.fcs === plc));
  if (aoSources.length !== 1) throw new Error("Для HMI загрузите один исходный IO-лист XLSX или карту AO TXT выбранного ПЛК.");
  return [{ kind: "HMI", file: aoSources[0].file, url: "/api/temporary/ao/generate-diagnostic", fields: { fileName: `${options.fileName}_HMI`, context, diagnostic: { fcs: [plc] } } }];
};
