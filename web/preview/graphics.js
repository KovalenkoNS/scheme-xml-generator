/* Safe SVG rendering of coordinates and connections read from the saved XML. */

const NS = 'http://www.w3.org/2000/svg';
// Создаёт только заданный программой SVG-элемент и его разрешённые атрибуты.
// Текст из XML передаётся через textContent, никогда не становится разметкой или скриптом.
const element = (name, attributes = {}, text) => { const node = document.createElementNS(NS, name); for (const [key, value] of Object.entries(attributes)) node.setAttribute(key, String(value)); if (text !== undefined) node.textContent = text; return node; };
// Укорачивает надпись под ширину блока, чтобы соседние объекты оставались различимы.
// Полная строка сохраняется в модели, SVG title и панели инспектора.
const shortened = (text, width) => { const limit = Math.max(4, Math.floor(width / 7)); return text.length > limit ? text.slice(0, limit - 1) + '…' : text; };
// Добавляет блоку, примитиву или связи выбор мышью и клавиатурой.
// При выборе подсвечивает один SVG-объект и передаёт его модель в инспектор диалога.
function selectable(node, item, inspect) {
  node.setAttribute('tabindex', '0'); node.setAttribute('role', 'button'); node.setAttribute('aria-label', `${item.kind === 'link' ? 'Связь' : item.label} · ID ${item.id}`);
  // Снимает прежнее выделение внутри этой схемы и открывает параметры текущего объекта.
  // Вызывается единым путём для клика, Enter и пробела без изменения XML.
  const select = () => { node.ownerSVGElement?.querySelectorAll('.gp-selected').forEach(item => item.classList.remove('gp-selected')); node.classList.add('gp-selected'); inspect(item); };
  node.addEventListener('click', select); node.addEventListener('keydown', event => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); select(); } });
  const title = element('title', {}, item.kind === 'link' ? `${item.first.block}.${item.first.port} → ${item.last.block}.${item.last.port}` : item.label); node.append(title); return node;
}
// Рисует страницу исключительно по Block/Link/OnePrim и их сохранённым координатам.
// Возвращает SVG с направлениями связей, портами и NOT; библиотечную динамику не исполняет.
function render(page, inspect) {
  const svg = element('svg', { class: 'gp-svg', role: 'group', 'aria-label': `${page.kind}: ${page.name}`, 'data-kind': page.kind });
  const defs = element('defs'); const marker = element('marker', { id: 'gp-arrow', viewBox: '0 0 8 8', refX: 7, refY: 4, markerWidth: 6, markerHeight: 6, orient: 'auto-start-reverse' });
  marker.append(element('path', { d: 'M 0 0 L 8 4 L 0 8', fill: 'currentColor' })); defs.append(marker); svg.append(defs);
  const primitives = element('g', { class: 'gp-primitives' });
  for (const item of page.primitives) {
    const group = element('g', { class: `gp-primitive${item.fallback ? ' gp-fallback' : ''}${item.layer.Visible === '0' ? ' gp-hidden-layer' : ''}`, 'data-id': item.id });
    if (item.type === '7' && item.points.length > 1) group.append(element('polyline', { points: item.points.map(p => `${p.x},${p.y}`).join(' '), fill: 'none' }));
    else {
      group.append(element('rect', { x: item.x, y: item.y, width: Math.max(1, item.width), height: Math.max(1, item.height), rx: item.fallback ? 3 : 0 }));
      const label = shortened(item.label, Math.max(item.width, 50));
      if (label) group.append(element('text', { x: item.x + 5, y: item.y + Math.min(item.height - 3, 16), 'font-size': 12 }, label));
    }
    primitives.append(selectable(group, item, inspect));
  }
  svg.append(primitives);
  const links = element('g', { class: 'gp-links' });
  for (const item of page.links) {
    if (item.points.length < 2) continue;
    const group = element('g', { class: 'gp-link', 'data-id': item.id }); const pointList = item.points.map(p => `${p.x},${p.y}`).join(' ');
    group.append(element('polyline', { class: 'gp-link-hit', points: pointList, fill: 'none' }));
    group.append(element('polyline', { points: pointList, fill: 'none', 'marker-end': 'url(#gp-arrow)' }));
    for (const point of item.points) if (point.junction) group.append(element('circle', { cx: point.x, cy: point.y, r: 3 }));
    links.append(selectable(group, item, inspect));
  }
  svg.append(links);
  const blocks = element('g', { class: 'gp-blocks' });
  for (const item of page.blocks) {
    const flavor = item.isNot ? 'gp-not' : /D32/i.test(item.typeName) ? 'gp-module' : item.type === '31' ? 'gp-signal' : 'gp-function';
    const group = element('g', { class: `gp-block ${flavor}`, 'data-id': item.id });
    group.append(element('rect', { x: item.x, y: item.y, width: Math.max(1, item.width), height: Math.max(1, item.height), rx: 2 }));
    const moduleBlock = flavor === 'gp-module' || item.type === '37';
    const fontSize = moduleBlock ? 11 : Math.max(8, Math.min(12, (item.width - 10) / Math.max(1, item.label.length) / 0.6));
    const labelLength = Math.max(4, Math.floor((item.width - 10) / (fontSize * 0.6)));
    const label = !moduleBlock && item.label.length > labelLength ? item.label.slice(0, labelLength - 1) + '…' : item.label;
    group.append(element('text', { x: item.x + 5, y: moduleBlock ? item.y - 7 : item.y + Math.min(16, item.height - 3), 'font-size': fontSize }, label));
    if (item.typeName && item.typeName !== item.label && item.height >= 35) group.append(element('text', { x: item.x + 5, y: item.y + (moduleBlock ? 16 : 31), 'font-size': 11 }, shortened(item.typeName, item.width - 8)));
    blocks.append(selectable(group, item, inspect));
  }
  svg.append(blocks);
  const ports = element('g', { class: 'gp-ports', 'aria-hidden': 'true' });
  const blocksByID = new Map(page.blocks.map(block => [block.id, block]));
  const labelledPorts = new Set();
  for (const link of page.links) {
    if (link.points.length < 2) continue;
    for (const [point, endpoint, last] of [[link.points[0], link.first, false], [link.points[link.points.length - 1], link.last, true]]) {
      ports.append(element('circle', { cx: point.x, cy: point.y, r: 2.5 }));
      const owner = blocksByID.get(endpoint.block), identity = `${endpoint.block}|${endpoint.port}|${last}`;
      if (endpoint.port && owner && !owner.isNot && (/D32/i.test(owner.typeName) || owner.height > 50) && !labelledPorts.has(identity)) {
        ports.append(element('text', { x: point.x + (last ? 5 : -5), y: point.y + 11, 'text-anchor': last ? 'start' : 'end', 'font-size': 10 }, endpoint.port));
        labelledPorts.add(identity);
      }
    }
  }
  svg.append(ports); return svg;
}

export { render };
