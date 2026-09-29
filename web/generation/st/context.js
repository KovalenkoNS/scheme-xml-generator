/* Контекст адресации запроса ST, отдельный от общих проверок чисел. */
import { integer } from "../../shared/validation.js";
// Проверяет константы ST и формирует контекст адресации для серверного запроса с отдельным диапазоном POUNum.
export function mappingContext(settings, cpu, profile, offset = 0) {
  for (const field of ["version", "controllerId", "resourceId", "groupId", "pouNumber"]) integer(settings[field], field);
  return { ...settings, controllerTypeName: cpu, physicalProfile: profile, pouNumber: String(integer(settings.pouNumber, "POUNum") + offset) };
};
