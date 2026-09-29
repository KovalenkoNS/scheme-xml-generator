/* Безопасные DOM-элементы, поля и текстовые подписи компонентов интерфейса. */
// Находит известное поле интерфейса по ID; не содержит состояния или правил генерации.
export const $ = id => document.getElementById(id);

// Создаёт безопасный DOM-элемент для компонентов интерфейса; текст вставляется как текст, без исполнения HTML.
export function el(tag, className, text) {
  const element = document.createElement(tag);
  if (className) element.className = className;
  if (text !== undefined) element.textContent = text;
  return element;
};

// Связывает подпись настройки с полем DOM; формирует компактную строку текущего значения.
export function field(label, input, wide = false) { const node = el("label", `field${wide ? " wide" : ""}`); node.append(document.createTextNode(label), input); return node; };

// Создаёт текстовый пункт списка выбора ПЛК или библиотечного шаблона без HTML-разметки.
export function option(value, text) { const node = el("option", "", text); node.value = value; return node; };
