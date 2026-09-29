/* Поиск и отображение реальных типов, полей и шаблонов библиотеки. */
import * as _shared_dom from "../shared/dom.js";
import { state } from "../shell/state.js";
// Фильтрует реальные типы, поля и шаблоны каталога по поиску; ограничивает количество DOM-строк без потери данных каталога.
export function renderTypes() {
  const query = _shared_dom.$("library-search").value.trim().toLowerCase();
  _shared_dom.$("library-types").replaceChildren();
  let types = state.catalog.types || [];
  if (!types.length) { const seen = new Set(); types = state.catalog.templates.filter(template => { const key = `${template.libraryFile}|${template.ownerId}`; if (seen.has(key)) return false; seen.add(key); return true; }).map(template => ({ id: template.ownerId, name: template.ownerName, libraryFile: template.libraryFile })); }
  const filtered = types.filter(type => `${type.name} ${type.libraryFile} ${(type.fields || []).map(item => item.name).join(" ")} ${state.catalog.templates.filter(template => template.ownerId === type.id && template.libraryFile === type.libraryFile).map(template => template.name).join(" ")}`.toLowerCase().includes(query));
  _shared_dom.$("library-types").append(_shared_dom.el("p", "settings-note", `${filtered.length} типов${filtered.length > 120 ? " · показаны первые 120, уточните поиск" : ""}`));
  for (const type of filtered.slice(0, 120)) {
    const row = _shared_dom.el("div", "type-item"); row.append(_shared_dom.el("strong", "", type.name), _shared_dom.el("small", "", `${type.libraryFile} · ID ${type.id}`));
    const templates = state.catalog.templates.filter(template => template.ownerId === type.id && template.libraryFile === type.libraryFile);
    if (templates.length || type.fields?.length) { const details = _shared_dom.el("details"), list = _shared_dom.el("ul"); details.append(_shared_dom.el("summary", "", `${type.fields?.length || 0} полей · ${templates.length} шаблонов`)); for (const value of type.fields || []) list.append(_shared_dom.el("li", "", `${value.name} · ${value.typeName || value.kind || "тип в библиотеке"}`)); for (const template of templates) list.append(_shared_dom.el("li", "", `${template.name}${template.supported ? "" : " · не поддержан"}`)); details.append(list); row.append(details); }
    _shared_dom.$("library-types").append(row);
  }
};
