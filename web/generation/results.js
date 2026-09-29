/* Результаты выпуска: краткие ошибки, ссылки на сохранённые файлы и открытие их реального XML. */
import * as _preview_viewer from "../preview/viewer.js";
import * as _shared_dom from "../shared/dom.js";
// Проверяет ссылку результата локального API; возвращает только адрес сохранённого файла внутри /api/output/.
export function safeDownload(file) {
  const url = new URL(file.url, location.origin);
  if (url.origin !== location.origin || !url.pathname.startsWith("/api/output/")) throw new Error("Некорректная ссылка на результат.");
  return url.pathname + url.search;
};

// Добавляет краткую ошибку выбранной области в список результатов; другие успешные файлы остаются доступны.
export function resultError(kind, text) { const row = _shared_dom.el("li"); row.append(_shared_dom.el("span", "result-kind", kind), _shared_dom.el("span", "result-error", text)); _shared_dom.$("result-list").append(row); };

// Показывает сохранённые файлы ответа API; связывает скачивание и графический просмотр с одним фактическим XML.
export function resultFiles(kind, response) {
  const files = Array.isArray(response.files) ? response.files : response.url ? [response] : [];
  if (!files.length) throw new Error("Генератор не вернул файлы результата.");
  for (const file of files) {
    const row = _shared_dom.el("li"), link = _shared_dom.el("a", "", file.fileName);
    link.href = safeDownload(file); link.download = file.fileName;
    const preview = _shared_dom.el("button", "quiet result-preview", "Просмотр"); preview.type = "button";
    preview.setAttribute("aria-label", `Просмотр ${file.fileName}`);
    preview.addEventListener("click", () => _preview_viewer.open({ fileName: file.fileName, url: safeDownload(file) }));
    row.append(_shared_dom.el("span", "result-kind", kind), link, preview); _shared_dom.$("result-list").append(row);
  }
  if (response.warnings?.length) resultError("INFO", response.warnings.join(" "));
  return files.length;
};
