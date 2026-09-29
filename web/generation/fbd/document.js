/* Планирование обычного FBD-документа по явным библиотечным шаблонам. */
import * as _equipment_controllers from "../../equipment/controllers.js";
import * as _generation_fbd_placement from "./placement.js";
import * as _shared_validation from "../../shared/validation.js";
import * as _sources_model from "../../sources/model.js";
// Формирует FBD для направлений кроме DO из явных библиотечных ключей; изолирует выбранный ПЛК и исключает дубликаты сигналов.
export function fbdRequest(sources, plc, options, catalog) {
  if (![_equipment_controllers.CPU715, _equipment_controllers.CPU850].includes(options.cpu)) throw new Error("Выберите модель ПЛК 715 или 850.");
  const templates = new Map((catalog.templates || []).map(template => [template.key, template]));
  const seen = new Map(), names = new Set(), pous = [];
  for (const group of _sources_model.fbdGroups(sources, plc).filter(group => group.kind !== "DO")) {
    const key = options.templates[group.kind];
    const template = templates.get(key);
    if (!template?.supported) throw new Error(`${group.kind}: выберите поддержанный шаблон из подключённой библиотеки.`);
    const signals = [];
    for (const module of group.modules) for (const channel of module.channels || []) {
      if (!channel.tag || channel.duplicate) continue;
      const identity = channel.tag.toLowerCase();
      if (seen.has(identity)) {
        if (seen.get(identity) !== key) throw new Error(`${channel.tag}: один экземпляр получил разные шаблоны из нескольких источников.`);
        continue;
      }
      seen.set(identity, key);
      const signal = { templateKey: key, objectName: channel.tag, nameMode: "base", description: channel.description || "", ..._generation_fbd_placement.signalOffsets(template, signals.length) };
      if (options.inversion?.[group.kind]) signal.invert = true;
      signals.push(signal);
    }
    if (!signals.length) continue;
    let name = String(group.name || group.pouName || group.kind).replace(/[^A-Za-z0-9_]/g, "_");
    if (!/^[A-Za-z_]/.test(name)) name = `POU_${name}`;
    const base = name; let suffix = 2;
    while (names.has(name.toLowerCase())) name = `${base}_${suffix++}`;
    names.add(name.toLowerCase());
    pous.push({ name, groupId: _shared_validation.positive(options.context.groupId, "GroupID"), pouNumber: _shared_validation.positive(options.context.pouNumber, "POUNum") + pous.length, signals });
  }
  if (!pous.length) throw new Error("В выбранном ПЛК нет экземпляров для FBD.");
  return { fileName: `${options.fileName}_${plc}_FBD.xml`, context: {
    controllerTypeName: options.cpu, version: options.context.version, project: options.context.project,
    controllerId: options.context.controllerId, resourceId: options.context.resourceId,
  }, pous };
};
