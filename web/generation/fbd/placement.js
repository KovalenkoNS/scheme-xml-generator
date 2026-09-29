/* Расположение FBD-экземпляров по фактическим границам библиотечной геометрии. */

// Размещает экземпляры по фактическим границам шаблона из каталога; новые столбцы
// сохраняют читаемость и ограничение координат XML при больших IO-листах.
export function signalOffsets(template, index) {
  const bounds = template.layout || { minX: 0, minY: 0, maxX: template.width || 0, maxY: template.height || 0 };
  const width = Math.max(1, bounds.maxX - bounds.minX) + 160;
  const height = Math.max(1, bounds.maxY - bounds.minY) + 60;
  const baseX = 300 - Math.min(0, bounds.minX), baseY = 100 - Math.min(0, bounds.minY);
  const rows = Math.max(1, Math.min(24, Math.floor((100000 - baseY) / height)));
  const offsetX = baseX + Math.floor(index / rows) * width, offsetY = baseY + (index % rows) * height;
  if (offsetX > 100000 || offsetY > 100000) throw new Error("Шаблон и число сигналов превышают размер страницы XML. Разделите исходные группы.");
  return { offsetX, offsetY };
};
