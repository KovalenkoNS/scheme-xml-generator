/* Read saved SCADA XML into a presentation model. No library inference, ID allocation or execution. */

// Выбирает прямые элементы указанного XML-узла для разбора POU, слоя или секции.
// Не захватывает вложенные кадры и возвращает пустой список для отсутствующего узла.
const children = (node, name) => Array.from(node?.children || []).filter(child => child.localName === name);
// Возвращает первый прямой дочерний узел из сохранённого XML.
// Используется для обязательных и необязательных контейнеров без создания данных.
const child = (node, name) => children(node, name)[0] || null;
// Находит все вложенные записи секции, включая дочерние HMI-кадры.
// Вход — DOM XML, результат — массив узлов для независимого обхода.
const descendants = (node, name) => Array.from(node.getElementsByTagName(name));
// Копирует атрибуты XML в обычный объект для модели и панели инспектора.
// Значения остаются строками; DOM исходного документа не изменяется.
const attrs = node => Object.fromEntries(Array.from(node?.attributes || []).map(item => [item.name, item.value]));
// Читает текст прямого дочернего элемента, например STCODE или PARAMS.
// Возвращает исходный textContent, а для отсутствующего элемента — пустую строку.
const text = (node, name) => child(node, name)?.textContent || '';
// Ограничивает числовые координаты перед передачей геометрии в SVG.
// Неконечные и непрактично большие значения превращаются в ноль, не в SVG-код.
const finite = value => { const n = Number(value); return Number.isFinite(n) && Math.abs(n) <= 1e7 ? n : 0; };

// Разбирает сохранённый PointList в координаты и явные отметки узлов ветвления.
// Некорректная строка возвращает пустой путь; недостающая трасса не выдумывается.
function points(value) {
  const source = String(value || '');
  const result = []; const pattern = /(\*)?\(\s*(-?\d+(?:\.\d+)?)\s*,\s*(-?\d+(?:\.\d+)?)\s*\)\s*;?/g;
  let match; let end = 0;
  while ((match = pattern.exec(source))) {
    if (source.slice(end, match.index).trim()) return [];
    result.push({ x: finite(match[2]), y: finite(match[3]), junction: !!match[1] }); end = pattern.lastIndex;
  }
  return source.slice(end).trim() ? [] : result;
}
// Разделяет FP/LP на ID блока, направление и имя порта для инспектора связи.
// Сохраняет исходную строку вместе с полями, чтобы пользователь мог сверить XML.
function endpoint(value) {
  const pieces = String(value || '').split('|');
  return { raw: value || '', block: pieces[0] || '', input: pieces[1] || '', port: pieces[2] || '', bounds: pieces.slice(3).join('|') };
}
// Читает формат [ключ]=значение из PARAMS графического примитива.
// Возвращает словарь без прототипа; значения с дополнительными знаками '=' сохраняются.
function parameters(value) {
  const result = Object.create(null);
  for (const line of String(value || '').split(/\r?\n/)) { const found = /^\[([^\]]+)\]=(.*)$/.exec(line); if (found) result[found[1]] = found[2]; }
  return result;
}
// Извлекает прямоугольную геометрию Graphics или OnePrim для SVG-просмотра.
// Размеры не бывают отрицательными; исходные атрибуты доступны отдельно в инспекторе.
const rect = node => { const a = attrs(node); return { x: finite(a.X), y: finite(a.Y), width: Math.max(0, finite(a.WIDTH)), height: Math.max(0, finite(a.HEIGHT)) }; };
// Индексирует секцию rec по ID для разрешения имён ISA-типов, символов и карточек.
// Читает только текущий XML-файл; к библиотеке и настройкам генерации не обращается.
const index = (document, name, key) => new Map(descendants(child(document.documentElement, name) || document.createElement('empty'), 'rec').map(item => [item.getAttribute('ID'), item.getAttribute(key) || '']));

// Переводит OnePrim в геометрию и проверяемые привязки HMI/FBD.
// Невосстановимые библиотечные символы помечает для честного показа границ вместо динамики.
function primitive(node, document, layer) {
  const a = attrs(node); const raw = text(node, 'PARAMS'); const params = parameters(raw);
  const symbolID = text(node, 'ObjMSID'); const cardID = text(node, 'CardID');
  return { kind: 'primitive', id: a.SourceT11ID || '', type: a.OBJTYPE || '', ...rect(node), attributes: a, params,
    rawParameters: raw, points: points(params.PL), label: params.TEXT || document.symbols.get(symbolID) || document.cards.get(cardID) || `Объект ${a.SourceT11ID || a.OBJTYPE || ''}`,
    symbolID, cardID, symbol: document.symbols.get(symbolID) || '', card: document.cards.get(cardID) || '',
    layer: layer ? attrs(layer) : {}, fallback: a.OBJTYPE !== '7',
    receptors: children(child(node, 'Receptors'), 'OneReceptor').map(attrs), animators: children(child(node, 'Animators'), 'OneAnim').map(attrs) };
}

// Вычисляет рамку фактических блоков, примитивов и точек связей для команды «Вместить».
// Декларированный большой печатный лист не уменьшает масштаб маленькой схемы.
function bounds(page) {
  const values = [];
  for (const object of [...page.blocks, ...page.primitives]) {
    values.push({ x: object.x, y: object.y }, { x: object.x + object.width, y: object.y + object.height }, ...(object.points || []));
  }
  for (const link of page.links) values.push(...link.points);
  if (!values.length) return { x: 0, y: 0, width: 800, height: 500 };
  let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
  for (const point of values) { minX = Math.min(minX, point.x); minY = Math.min(minY, point.y); maxX = Math.max(maxX, point.x); maxY = Math.max(maxY, point.y); }
  const x = minX - 30, y = minY - 30;
  return { x, y, width: Math.max(100, maxX - x + 30), height: Math.max(100, maxY - y + 30) };
}

// Проверяет XML и собирает страницы FBD/ST/HMI для просмотрщика сохранённого файла.
// Возвращает исходные ID, геометрию и параметры; неизвестный корень, DTD и синтаксис отклоняет.
function parseXML(source, Parser = globalThis.DOMParser) {
  if (typeof source !== 'string' || !source.trim()) throw new Error('XML-файл пуст.');
  if (/<!DOCTYPE|<!ENTITY/i.test(source)) throw new Error('Объявления DTD и внешних сущностей не поддерживаются.');
  if (!Parser) throw new Error('В этой среде недоступен разбор XML.');
  const xml = new Parser().parseFromString(source, 'application/xml');
  if (xml.getElementsByTagName('parsererror').length) throw new Error('Файл содержит некорректный XML.');
  if (!['BufScadaPOUS', 'BufScada'].includes(xml.documentElement.localName)) throw new Error('Этот XML не является файлом FBD, ST или HMI генератора.');
  const document = { root: xml.documentElement.localName, common: attrs(child(xml.documentElement, 'Common')), pages: [], warnings: [], symbols: index(xml, 'PAGEMSINFO', 'Info'), cards: index(xml, 'CARDSINFO', 'CardInfo') };
  const isa = index(xml, 'ISAOBJSINFO', 'Info');
  if (document.root === 'BufScadaPOUS') {
    for (const pou of children(child(xml.documentElement, 'POUS'), 'OnePOU')) {
      const a = attrs(pou); const code = child(pou, 'STCODE');
      const page = { kind: code ? 'ST' : 'FBD', name: a.NAME || a.ID || 'POU', id: a.ID || '', attributes: a, params: attrs(child(pou, 'PARAMS')), code: code?.textContent || '', blocks: [], links: [], primitives: [], warnings: [] };
      if (!code) {
        const graph = child(pou, 'ISAGraf');
        for (const node of children(child(graph, 'Blocks'), 'Block')) {
          const a = attrs(node), p = attrs(child(node, 'Params')); const label = a.Info || p.T11Text || isa.get(p.IsaObjId) || `Блок ${a.T11ID || ''}`;
          page.blocks.push({ kind: 'block', id: a.T11ID || '', type: a.GROBJTYPE || '', typeName: isa.get(p.IsaObjId) || '', label, ...rect(child(node, 'Graphics')), attributes: a, params: p, initial: text(child(node, 'Params'), 'IV'), isNot: a.Info === 'NOT' || p.IsaObjId === '-32' });
        }
        let number = 0;
        for (const node of children(child(graph, 'Links'), 'Link')) {
          const rawPoints = child(node, 'PointList')?.getAttribute('PL') || '';
          const link = { kind: 'link', id: String(++number), label: 'Связь', attributes: attrs(node), rawPoints, points: points(rawPoints), first: endpoint(child(node, 'FirstPoint')?.getAttribute('FP')), last: endpoint(child(node, 'LastPoint')?.getAttribute('LP')) };
          if (link.points.length < 2) page.warnings.push(`У связи ${number} нет корректной геометрии PointList.`);
          page.links.push(link);
        }
        page.primitives = children(child(pou, 'GrObj'), 'OnePrim').map(node => primitive(node, document));
        const ids = new Set(page.blocks.map(block => block.id));
        for (const link of page.links) if (!ids.has(link.first.block) || !ids.has(link.last.block)) page.warnings.push(`Связь ${link.id} ссылается на отсутствующий блок.`);
        if (!graph && !page.primitives.length) page.warnings.push('Диалект этой POU не содержит поддержанных ISAGraf/GrObj.');
      }
      page.bounds = bounds(page); document.pages.push(page);
    }
  } else {
    for (const node of descendants(child(xml.documentElement, 'Pages') || xml.documentElement, 'OnePage')) {
      const page = { kind: 'HMI', name: text(node, 'NAME') || node.getAttribute('ID') || 'Кадр', id: node.getAttribute('ID') || text(node, 'ID'), attributes: attrs(node), params: {}, code: '', blocks: [], links: [], primitives: [], warnings: [] };
      for (const tag of ['WIDTH', 'HEIGHT', 'SHABLONPAGEID', 'FONCOLOR', 'DISC']) page.params[tag] = text(node, tag);
      for (const layer of children(child(node, 'PageLayers'), 'OneLayer')) page.primitives.push(...children(layer, 'OnePrim').map(item => primitive(item, document, layer)));
      if (page.primitives.some(item => item.fallback)) page.warnings.push('Библиотечные символы показаны границами и привязками; их графика и динамика здесь не воспроизводятся.');
      page.bounds = bounds(page); document.pages.push(page);
    }
  }
  if (!document.pages.length) throw new Error('В XML не найдены POU или кадры для просмотра.');
  document.kind = [...new Set(document.pages.map(page => page.kind))].join(' / ');
  return document;
}

// Проверяет адрес до fetch: разрешён только XML в /api/output/ того же экземпляра приложения.
// Возвращает канонический URL и имя файла; переходы по каталогам и внешние адреса отклоняет.
function outputURL(value, base) {
  const url = new URL(String(value || ''), base); const origin = new URL(base).origin;
  if (!/^https?:$/.test(url.protocol) || url.origin !== origin || url.username || url.password || url.search || url.hash || !url.pathname.startsWith('/api/output/')) throw new Error('Можно открыть только сохранённый XML этого генератора.');
  const name = decodeURIComponent(url.pathname.slice('/api/output/'.length));
  if (!name || /[/\\\u0000-\u001f]/.test(name) || !/\.xml$/i.test(name)) throw new Error('Предпросмотр поддерживает сохранённые XML-файлы.');
  return { href: url.href, name };
}

export { parseXML, outputURL, points, endpoint, parameters, bounds };
