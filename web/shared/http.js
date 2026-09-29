/* JSON-запросы и FormData локального API; преобразование ошибок транспорта в сообщения интерфейса. */

// Обращается к локальному HTTP API генератора; возвращает JSON или понятную ошибку соединения/операции.
export async function request(url, options = {}) {
  let response;
  try { response = await fetch(url, { cache: "no-store", credentials: "same-origin", ...options }); }
  catch { throw new Error("Генератор недоступен. Проверьте, что приложение запущено."); }
  let data;
  try { data = await response.json(); } catch { throw new Error("Генератор вернул некорректный ответ."); }
  if (!response.ok) throw new Error(typeof data.error === "string" ? data.error : "Не удалось выполнить запрос.");
  return data;
};

// Собирает multipart-запрос для IO-парсеров и импорта библиотеки из выбранного файла и параметров.
export function uploadBody(file, fields = {}) {
  const body = new FormData(); body.append("file", file);
  for (const [key, value] of Object.entries(fields)) body.append(key, typeof value === "object" ? JSON.stringify(value) : String(value));
  return body;
};
