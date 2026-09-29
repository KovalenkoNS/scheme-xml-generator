/* Сохранение постоянных настроек рабочей области в localStorage без исходных файлов и результатов XML. */
import { state, STORAGE } from "./state.js";
// Сохраняет текущие настройки ПЛК, шаблонов и выпуска в локальном хранилище; исходные файлы туда не записывает.
export function store() {
  try { localStorage.setItem(STORAGE, JSON.stringify({ plc: state.plc, cpu: state.cpu, templates: state.templates, inversion: state.inversion, modules: state.modules, contexts: state.contexts, outputs: state.outputs, profile: state.profile, fileName: state.fileName })); } catch { /* Work continues in memory when storage is unavailable. */ }
};
