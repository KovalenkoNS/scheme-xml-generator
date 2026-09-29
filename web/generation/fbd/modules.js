/* Планирование модульного DO: реальные каналы, физические ID и независимый NOT. */
import * as _equipment_controllers from "../../equipment/controllers.js";
import * as _shared_validation from "../../shared/validation.js";
import * as _sources_model from "../../sources/model.js";
// Формирует модульную схему DO для библиотечного API: один модуль, реальные каналы, физический ID и переключаемый NOT.
export function libraryDORequest(sources, plc, options, catalog, offset = 0) {
  if (options.cpu !== _equipment_controllers.CPU850) throw new Error("Библиотечная модульная схема DO сейчас подтверждена только для 850.");
  const templateKey = options.templates.DO;
  if (!(catalog.templates || []).some(template => template.key === templateKey && template.supported)) throw new Error("DO: выберите поддержанный шаблон из подключённой библиотеки.");
  const ids = new Set(), names = new Set();
  const pous = _sources_model.fbdGroups(sources, plc).filter(group => group.kind === "DO").map((group, index) => {
    const choice = _sources_model.moduleSettings(group, options.modules);
    const modules = choice.moduleIds.map((id, moduleIndex) => {
      if (ids.has(id)) throw new Error(`ModuleID ${id} повторяется внутри выбранного ПЛК.`);
      ids.add(id);
      const module = group.modules[moduleIndex];
      // Reserve names follow the parsed physical prefix; no signal is invented.
      const name = module?.name || (group.prefix ? `${group.prefix}-${String((group.maxModuleSuffix || group.modules.length) + moduleIndex - group.modules.length + 1).padStart(2, "0")}` : "");
      if (!name) throw new Error("Для дополнительного DO-модуля нужен исходный префикс имени. Используйте подготовленную карту модулей.");
      return { name, id, channels: (module?.channels || []).filter(channel => channel.tag && !channel.duplicate).map(channel => ({ channel: _shared_validation.integer(channel.channel, `${name} · канал`, 31), tag: channel.tag, invert: !!options.inversion?.DO })) };
    });
    let name = String(group.name || group.pouName).replace(/[^A-Za-z0-9_]/g, "_");
    if (!/^[A-Za-z_]/.test(name)) name = `POU_${name}`;
    if (names.has(name)) throw new Error(`${name}: повторная POU DO в нескольких источниках. Оставьте один источник.`);
    names.add(name);
    return { name, groupId: _shared_validation.positive(options.context.groupId, "GroupID"), pouNumber: _shared_validation.positive(options.context.pouNumber, "POUNum") + offset + index, modules };
  });
  if (!pous.length) throw new Error("В выбранном ПЛК нет модулей DO.");
  return { templateKey, plcName: plc, physicalProfile: "measurement-quality", context: { controllerTypeName: options.cpu, version: options.context.version, project: options.context.project, controllerId: options.context.controllerId, resourceId: options.context.resourceId }, fileName: `${options.fileName}_${plc}_FBD_DO.xml`, pous };
};
