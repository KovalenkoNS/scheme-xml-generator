/* Dialog lifecycle, saved-file loading and inspection. The XML parser and renderer stay independent. */
import * as graphics from "./graphics.js";
import * as _preview_navigation from "./navigation.js";
import * as parser from "./parser.js";
const MAX_BYTES = 64 * 1024 * 1024;
let dialog, title, picker, status, canvas, details, zoomLabel, tools, body, navigation, active, controller, serial = 0, returnFocus;
let svg, view, original, currentPage;
// Создаёт HTML-элементы диалога из фиксированных имён и безопасного текстового содержимого.
// XML-строки используются только как текст, а не как HTML.
const node = (tag, className, text) => { const item = document.createElement(tag); if (className) item.className = className; if (text !== undefined) item.textContent = text; return item; };
// Собирает доступную кнопку панели с подписью для скринридера и одним действием.
// Возвращаемая кнопка не отправляет формы основной страницы генератора.
const button = (text, label, handler) => { const item = node('button', '', text); item.type = 'button'; item.setAttribute('aria-label', label); item.title = label; item.addEventListener('click', handler); return item; };

// Лениво создаёт один модальный просмотрщик и связывает управление масштабом, панорамой и закрытием.
// Повторное открытие переиспользует DOM; закрытие отменяет чтение файла и возвращает фокус вызывающему элементу.
function ensureDialog() {
  if (dialog) return;
  dialog = node('dialog', 'gp-dialog'); dialog.setAttribute('aria-labelledby', 'gp-title');
  const head = node('header', 'gp-head'); title = node('strong', '', 'Просмотр XML'); title.id = 'gp-title';
  head.append(title, button('Параметры', 'Показать или скрыть параметры', () => { details.hidden = !details.hidden; body.classList.toggle('gp-with-inspector', !details.hidden); updateView(); }), button('×', 'Закрыть просмотр', () => dialog.close()));
  tools = node('div', 'gp-tools'); picker = node('select'); picker.setAttribute('aria-label', 'POU или кадр'); picker.addEventListener('change', showPage);
  zoomLabel = node('output', 'gp-zoom', '100%'); zoomLabel.setAttribute('aria-label', 'Масштаб');
  tools.append(picker, button('−', 'Уменьшить', () => zoom(1.25)), zoomLabel, button('+', 'Увеличить', () => zoom(0.8)), button('Читать', 'Читаемый масштаб', () => readable()), button('Весь файл', 'Вместить графику', fit));
  navigation = _preview_navigation.create(item => { readable(item); inspect(item); });
  status = node('p', 'gp-status'); status.setAttribute('role', 'status');
  body = node('div', 'gp-body'); canvas = node('div', 'gp-canvas'); canvas.setAttribute('tabindex', '0'); canvas.setAttribute('aria-label', 'Графическая структура. Стрелки перемещают вид, плюс и минус меняют масштаб.');
  details = node('aside', 'gp-inspector'); details.setAttribute('aria-label', 'Параметры выбранного объекта'); body.append(canvas, details);
  const note = node('footer', 'gp-note', 'Структура XML · проверка исполнения — в SCADA');
  dialog.append(head, tools, navigation.element, status, body, note); document.body.append(dialog);
  dialog.addEventListener('close', () => { serial++; controller?.abort(); active = null; returnFocus?.focus?.(); });
  canvas.addEventListener('wheel', event => { if (!svg) return; event.preventDefault(); zoom(event.deltaY > 0 ? 1.15 : 1 / 1.15); }, { passive: false });
  canvas.addEventListener('keydown', event => {
    if (!svg || event.target !== canvas) return;
    const step = view.width / 12;
    if (event.key === '+' || event.key === '=') zoom(0.8);
    else if (event.key === '-') zoom(1.25);
    else if (event.key === 'Home') readable();
    else if (event.key === 'ArrowLeft') { view.x -= step; updateView(); }
    else if (event.key === 'ArrowRight') { view.x += step; updateView(); }
    else if (event.key === 'ArrowUp') { view.y -= step; updateView(); }
    else if (event.key === 'ArrowDown') { view.y += step; updateView(); }
    else return;
    event.preventDefault();
  });
  let drag = null;
  canvas.addEventListener('pointerdown', event => { if (!svg || event.button !== 0 || event.target.closest('[role=button]')) return; drag = { x: event.clientX, y: event.clientY, view: { ...view } }; canvas.setPointerCapture(event.pointerId); canvas.classList.add('gp-dragging'); });
  canvas.addEventListener('pointermove', event => { if (!drag) return; const scale = Math.max(drag.view.width / canvas.clientWidth, drag.view.height / canvas.clientHeight); view.x = drag.view.x - (event.clientX - drag.x) * scale; view.y = drag.view.y - (event.clientY - drag.y) * scale; updateView(); });
  // Завершает панорамирование при отпускании или отмене указателя.
  // Сбрасывает временную точку захвата, сохраняя достигнутый viewBox.
  const stop = () => { drag = null; canvas.classList.remove('gp-dragging'); }; canvas.addEventListener('pointerup', stop); canvas.addEventListener('pointercancel', stop);
}
// Применяет текущую область обзора к SVG и показывает её фактический масштаб.
// Используется после масштабирования и перемещения, не меняя координаты модели XML.
function updateView() { if (!svg || !view) return; svg.setAttribute('viewBox', `${view.x} ${view.y} ${view.width} ${view.height}`); zoomLabel.textContent = `${Math.round(Math.min(canvas.clientWidth / view.width, canvas.clientHeight / view.height) * 100)}%`; }
// Возвращает всю геометрию выбранной страницы в видимую область диалога.
// Рамка берётся из parser.bounds, масштаб рассчитывается по текущему размеру полотна.
function fit() { if (!svg) return; view = { ...original }; updateView(); }
// Открывает фактический фрагмент схемы с читаемыми надписями вместо уменьшения всей POU.
// Поиск передаёт точный блок; его координаты и соединения в XML не переставляются.
function readable(item) {
  if (!svg) return;
  const scale = 1.5;
  const width = canvas.clientWidth / scale, height = canvas.clientHeight / scale;
  const maxX = Math.max(original.x, original.x + original.width - width), maxY = Math.max(original.y, original.y + original.height - height);
  let focusX = item ? item.x - width / 3 : original.x;
  if (item?.kind === 'block') {
    const connected = new Set([item.id]);
    for (const link of currentPage.links) if (link.first.block === item.id || link.last.block === item.id) { connected.add(link.first.block); connected.add(link.last.block); }
    const neighbours = currentPage.blocks.filter(block => connected.has(block.id));
    const left = Math.min(...neighbours.map(block => block.x)), right = Math.max(...neighbours.map(block => block.x + block.width));
    if (right - left + 60 <= width) focusX = left - 30;
  }
  view = { x: Math.min(maxX, Math.max(original.x, focusX)), y: Math.min(maxY, Math.max(original.y, item ? item.y - 40 : original.y)), width, height };
  svg.querySelectorAll('.gp-search-match').forEach(element => element.classList.remove('gp-search-match'));
  if (item) {
    for (const element of svg.querySelectorAll('[data-id]')) if (element.dataset.id === item.id && !element.classList.contains('gp-link')) element.classList.add('gp-search-match');
    status.textContent = [...currentPage.warnings, item.label || item.name].filter(Boolean).join(' ');
  }
  updateView();
}
// Меняет масштаб вокруг центра текущего вида по кнопке, колесу или клавише.
// Ограничивает чрезмерное приближение и отдаление; сохранённый XML остаётся неизменным.
function zoom(factor) { if (!svg) return; const width = view.width * factor, height = view.height * factor; if (width < 20 || width > original.width * 30) return; view = { x: view.x + (view.width - width) / 2, y: view.y + (view.height - height) / 2, width, height }; updateView(); }
// Добавляет непустую группу исходных атрибутов или параметров в инспектор.
// Значения печатаются текстом; вложенные привязки сохраняют JSON-представление для проверки.
function section(name, values) {
  const entries = Object.entries(values || {}).filter(([, value]) => value !== undefined && value !== ''); if (!entries.length) return;
  details.append(node('h3', '', name)); const list = node('dl');
  for (const [key, value] of entries) { list.append(node('dt', '', key), node('dd', '', typeof value === 'object' ? JSON.stringify(value) : String(value))); } details.append(list);
}
// Показывает ID, FP/LP, параметры и привязки выбранной страницы или SVG-объекта.
// Заменяет только панель сведений; данные берутся из разобранного сохранённого файла.
function inspect(item) {
  details.replaceChildren(node('h2', '', item.label || item.name || 'Параметры'));
  if (item.kind === 'link') { section('Порты', { 'Откуда': `${item.first.block} · ${item.first.port}`, 'Куда': `${item.last.block} · ${item.last.port}`, FP: item.first.raw, LP: item.last.raw }); section('Геометрия', { PointList: item.rawPoints }); }
  else section('Идентификаторы', { ID: item.id, 'Тип': item.type || item.kind, 'ISA-тип': item.typeName, 'Символ': item.symbol, CardID: item.cardID, 'Привязка': item.card });
  section('XML', item.attributes); section('Параметры', item.params);
  if (item.initial) section('Начальное значение', { IV: item.initial });
  if (item.layer) section('Слой', item.layer);
  if (item.receptors?.length) section('Привязки', Object.fromEntries(item.receptors.map((value, i) => [`Receptor ${i + 1}`, value])));
  if (item.animators?.length) section('Динамика', Object.fromEntries(item.animators.map((value, i) => [`Animator ${i + 1}`, value])));
}
// Переключает выбранную POU или HMI-кадр между SVG и точным текстом STCODE.
// Обновляет подсказки ограничений, инспектор и масштаб, не обращаясь к форме генерации.
function showPage() {
  if (!active) return;
  const page = active.pages[Number(picker.value) || 0]; currentPage = page; canvas.replaceChildren(); svg = null; inspect(page); details.hidden = true; body.classList.remove('gp-with-inspector'); navigation.setPage(page);
  for (const control of tools.querySelectorAll('button,output')) control.hidden = page.kind === 'ST';
  status.textContent = page.warnings.length ? page.warnings.join(' ') : page.kind === 'ST' ? 'Исходный STCODE из файла' : `${page.blocks.length ? page.blocks.length + ' блоков · ' + page.links.length + ' связей' : page.primitives.length + ' графических объектов'}`;
  status.classList.toggle('gp-warning', !!page.warnings.length);
  canvas.classList.toggle('gp-code-mode', page.kind === 'ST');
  if (page.kind === 'ST') { const code = node('pre', 'gp-code', page.code); code.setAttribute('tabindex', '0'); canvas.append(code); zoomLabel.textContent = ''; }
  else { svg = graphics.render(page, inspect); canvas.append(svg); original = { ...page.bounds }; readable(); }
}
// Читает сохранённый XML без кеша и редиректов через локальный endpoint результата.
// Поток ограничен 64 МБ, отменяется при закрытии; возвращается строгий UTF-8 текст.
async function readSaved(url, signal) {
  const response = await fetch(url, { credentials: 'same-origin', cache: 'no-store', redirect: 'error', signal, headers: { Accept: 'application/xml' } });
  if (!response.ok) throw new Error(response.status === 404 ? 'Сохранённый файл не найден.' : 'Не удалось открыть сохранённый файл.');
  if (Number(response.headers.get('Content-Length')) > MAX_BYTES) throw new Error('Файл превышает лимит просмотра 64 МБ.');
  const reader = response.body.getReader(); const decoder = new TextDecoder('utf-8', { fatal: true }); let size = 0, source = '';
  try { while (true) { const { done, value } = await reader.read(); if (done) break; size += value.byteLength; if (size > MAX_BYTES) { await reader.cancel(); throw new Error('Файл превышает лимит просмотра 64 МБ.'); } source += decoder.decode(value, { stream: true }); } source += decoder.decode(); return source; }
  finally { reader.releaseLock(); }
}
// Открывает результат по file.url из /api/output/ и создаёт модель из фактически сохранённых байтов.
// Возвращает модель либо null при ошибке/отмене; ошибку показывает в диалоге, предыдущий запрос отменяет.
async function open(file) {
  ensureDialog(); if (!dialog.open) returnFocus = document.activeElement;
  controller?.abort(); controller = new AbortController(); const current = ++serial;
  active = null; svg = null; picker.replaceChildren(); canvas.replaceChildren(); details.replaceChildren(); navigation.element.hidden = true; status.classList.remove('gp-warning'); status.textContent = 'Открывается сохранённый XML…'; title.textContent = 'Просмотр XML'; tools.hidden = true;
  if (!dialog.open) dialog.showModal();
  try {
    const target = parser.outputURL(file?.url, location.href); title.textContent = target.name;
    const source = await readSaved(target.href, controller.signal); const parsed = parser.parseXML(source);
    if (current !== serial || !dialog.open) return null;
    active = parsed; for (const [index, page] of parsed.pages.entries()) { const option = node('option', '', `${page.kind} · ${page.name}`); option.value = String(index); picker.append(option); }
    // A root can be a folder and a rear panel only navigation: prefer a frame with actual bound symbols.
    const symbolPage = parsed.pages.findIndex(page => page.kind === 'HMI' && page.primitives.some(primitive => primitive.symbolID && primitive.cardID && primitive.cardID !== '0'));
    const visiblePage = parsed.pages.findIndex(page => page.kind === 'ST' || page.blocks.length || page.primitives.length);
    picker.value = String(Math.max(0, symbolPage >= 0 ? symbolPage : visiblePage));
    picker.disabled = parsed.pages.length === 1; tools.hidden = false; showPage(); return parsed;
  } catch (error) { if (current === serial && error.name !== 'AbortError') { status.textContent = error.message || 'Не удалось прочитать XML.'; status.classList.add('gp-warning'); } return null; }
}

export { open };
