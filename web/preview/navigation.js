/* Поиск по фактическим именам блоков и символов XML; переход не меняет геометрию файла. */

// Создаёт компактную строку поиска текущей POU; найденный объект передаёт управлению видом.
function create(choose) {
  const element = document.createElement('div'); element.className = 'gp-navigation';
  const search = document.createElement('input'); search.type = 'search'; search.placeholder = 'Найти сигнал или блок'; search.setAttribute('aria-label', 'Найти сигнал или блок');
  const count = document.createElement('span'); count.className = 'gp-match-count'; count.setAttribute('aria-live', 'polite');
  const previous = document.createElement('button'), next = document.createElement('button');
  previous.type = next.type = 'button'; previous.textContent = '↑'; next.textContent = '↓';
  previous.setAttribute('aria-label', 'Предыдущее совпадение'); next.setAttribute('aria-label', 'Следующее совпадение');
  let items = [], matches = [], cursor = -1;
  // Показывает индекс результата и передаёт его сохранённые координаты просмотрщику.
  function select(index) {
    cursor = matches.length ? (index + matches.length) % matches.length : -1;
    count.textContent = search.value.trim() ? matches.length ? `${cursor + 1} / ${matches.length}` : 'Не найдено' : '';
    previous.disabled = next.disabled = matches.length < 2;
    if (cursor >= 0) choose(matches[cursor]);
  }
  // Фильтрует имена, ID и типы в текущей POU без сетевого запроса или чтения формы генерации.
  function filter() {
    const query = search.value.trim().toLocaleLowerCase('ru');
    matches = query ? items.filter(item => `${item.label} ${item.typeName || ''} ${item.id}`.toLocaleLowerCase('ru').includes(query)) : [];
    // Exact signal names precede their .Out/.Stat or _MOS fields, preserving source order otherwise.
    matches.sort((left, right) => Number(right.label.toLocaleLowerCase('ru') === query) - Number(left.label.toLocaleLowerCase('ru') === query));
    select(0);
  }
  search.addEventListener('input', filter);
  search.addEventListener('keydown', event => { if (event.key === 'Enter') { event.preventDefault(); select(cursor + (event.shiftKey ? -1 : 1)); } });
  previous.addEventListener('click', () => select(cursor - 1)); next.addEventListener('click', () => select(cursor + 1));
  element.append(search, count, previous, next);
  // Переключает набор поиска вместе с POU/кадром и очищает прежнее выделение.
  function setPage(page) {
    items = [...page.blocks, ...page.primitives]; search.value = ''; matches = []; cursor = -1;
    count.textContent = ''; previous.disabled = next.disabled = true; element.hidden = page.kind === 'ST';
  }
  return { element, setPage };
}

export { create };
