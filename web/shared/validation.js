/* Проверка числовых полей запросов; не содержит моделей оборудования или экспортных профилей. */

// Проверяет целое неотрицательное значение поля запроса и возвращает число в заданном диапазоне.
export const integer = (value, label, max = 2147483647) => {
  if (!/^\d+$/.test(String(value)) || !Number.isSafeInteger(Number(value)) || Number(value) > max) throw new Error(`${label}: укажите целое число от 0 до ${max}.`);
  return Number(value);
};
// Проверяет явные GroupID/POUNum перед серверным выпуском; запрещает ноль вместо допустимого положительного идентификатора.
export function positive(value, label) {
  const result = integer(value, label);
  if (result === 0) throw new Error(`${label}: укажите положительное число; текущее значение 0 не подходит для выпуска.`);
  return result;
};
