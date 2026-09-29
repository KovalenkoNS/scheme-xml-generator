/* Общий DOM-контейнер предметных настроек FBD, ST и HMI. */
import * as _shared_dom from "../shared/dom.js";
// Создаёт компактный контейнер настроек FBD, ST или HMI; возвращает DOM для предметного редактора.
export function panel(kind, title, subtitle) {
  const result = _shared_dom.el("section", "output-panel"); result.dataset.output = kind;
  const head = _shared_dom.el("div", "section-title"); head.append(_shared_dom.el("h2", "", title), _shared_dom.el("span", "", subtitle)); result.append(head); return result;
};
