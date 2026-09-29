/* Чистая модель sources/model.js: подготовка данных запроса без DOM, сети и записи файлов. */
import * as _shared_validation from "../shared/validation.js";
// Собирает имена ПЛК из ответов локальных IO-парсеров; возвращает общий отсортированный выбор без дубликатов.
export function controllers(sources) {
  const names = new Set();
  for (const source of sources) {
    for (const group of source.ao?.groups || []) names.add(group.fcs);
    for (const group of source.assignments?.groups || []) names.add(group.controllerName);
    for (const controller of source.io?.controllers || []) names.add(nameOf(controller));
  }
  return [...names].filter(Boolean).sort((a, b) => a.localeCompare(b, "ru", { numeric: true }));
};

// Определяет только направление физического IO-модуля; не выбирает SCADA-тип или библиотечный шаблон.
export function ioKind(module) { return /^(AI|AO|DI|DO)/i.exec(module.type || module.ioType || "")?.[1].toUpperCase() || "IO"; };

// Выбирает подготовленные группы назначений только указанного ПЛК; сохраняет связь с исходным файлом.
export function groups(sources, plc) {
  const result = [];
  for (const source of sources) {
    for (const group of source.ao?.groups || []) if (group.fcs === plc) result.push({ ...group, source, family: "ao", kind: "AO" });
    for (const group of source.assignments?.groups || []) if (group.controllerName === plc) result.push({ ...group, source, family: "assignments" });
  }
  return result;
};

// Дополняет подготовленные группы направлениями исходного IO-листа; не теряет каналы неподдержанного парсером вида.
export function fbdGroups(sources, plc) {
  const result = groups(sources, plc).map(group => ({ ...group, name: group.pouName || `${group.kind}_${group.prefix}` }));
  for (const source of sources) {
    const covered = new Set(result.filter(group => group.source === source).map(group => group.kind));
    for (const controller of source.io?.controllers || []) if (nameOf(controller) === plc) {
      const byKind = new Map();
      for (const module of controller.modules || []) {
        const kind = ioKind(module);
        if (covered.has(kind)) continue;
        if (!byKind.has(kind)) byKind.set(kind, []);
        byKind.get(kind).push(module);
      }
      for (const [kind, modules] of byKind) result.push({ source, family: "io", kind, key: `${controller.key}_${kind}`, name: `${kind}_${controller.key}`, modules });
    }
  }
  return result;
};

// Возвращает направления IO выбранного ПЛК для редактора библиотечных шаблонов FBD.
export function kinds(sources, plc) { return [...new Set(fbdGroups(sources, plc).map(group => group.kind))].sort(); };

// Формирует ключ настройки физических модулей из источника, семейства парсера и группы.
export function groupID(group) { return `${group.source.id}|${group.family}|${group.key}`; };

// Переносит сохранённые ModuleID со старого ключа адаптера на нейтральный; новые значения имеют приоритет.
export function migrateModuleSettings(saved) {
  const values = saved && typeof saved === "object" && !Array.isArray(saved) ? saved : {};
  const result = Object.fromEntries(Object.entries(values).map(([key, value]) => [key.replace(/\|skz\|(?=[^|]+$)/, "|assignments|"), value]));
  for (const [key, value] of Object.entries(values)) if (!/\|skz\|(?=[^|]+$)/.test(key)) result[key] = value;
  return result;
};

// Проверяет введённые количество и физические ID модулей; сохраняет нулевой ID и запрещает потерю исходных модулей.
export function moduleSettings(group, settings, requireIDs = true) {
  const record = settings[groupID(group)] || {};
  const count = _shared_validation.integer(record.count ?? group.modules.length, "Количество модулей", 4096);
  if (count < group.modules.length) throw new Error(`${group.pouName}: нельзя исключить исходные модули уменьшением количества.`);
  const ids = Array.from({ length: count }, (_, index) => requireIDs ? _shared_validation.integer(record.ids?.[index] ?? "", `${group.pouName} / ${group.modules[index]?.name || `добавленный модуль ${index + 1}`} · ModuleID`) : null);
  return { groupKey: group.key, moduleCount: count, moduleIds: ids };
};

// Имя ПЛК из ответов разных локальных парсеров; при отсутствии имени использует их исходный ключ.
export const nameOf = (controller) => controller.name || controller.sourceFcs || controller.key;
