(() => {
  "use strict";

  const MAX_FILE_BYTES = 16 * 1024 * 1024;
  const $ = (id) => document.getElementById(`techobjects-${id}`);
  const ui = {
    form: $("form"), file: $("file"), filename: $("filename"), resource: $("resource"), generate: $("generate"),
    preview: $("preview"), summary: $("summary"), plcs: $("plcs"),
    selectAll: $("select-all"), clearSelection: $("clear-selection"),
    selectionSummary: $("selection-summary"), selectionEmpty: $("selection-empty"),
    status: $("status"), errors: $("errors"), warnings: $("warnings"),
    result: $("result"), resultName: $("result-name"), resultSummary: $("result-summary"), downloads: $("downloads"),
  };
  const state = { file: null, plan: null, controls: new Map(), previewVersion: 0, previewController: null, generating: false };

  function showNotice(element, messages) {
    const values = (Array.isArray(messages) ? messages : [messages]).filter(Boolean).map((item) => String(item.message || item));
    element.replaceChildren();
    for (const value of new Set(values)) {
      const paragraph = document.createElement("p");
      paragraph.textContent = value;
      element.append(paragraph);
    }
    element.hidden = values.length === 0;
  }

  async function requestJSON(url, options) {
    let response;
    try {
      response = await fetch(url, { cache: "no-store", credentials: "same-origin", ...options });
    } catch (error) {
      if (error?.name === "AbortError") throw error;
      throw new Error("Сервер приложения недоступен. Проверьте, что генератор запущен, и повторите.");
    }
    let data;
    try { data = await response.json(); }
    catch { throw new Error(`Сервер вернул некорректный ответ (${response.status}).`); }
    if (!response.ok) {
      const details = Array.isArray(data?.errors) ? data.errors.map((item) => typeof item === "string" ? item
        : `${item.row ? `Строка ${item.row}: ` : ""}${item.message || item.error || "Ошибка данных"}`).join("\n") : "";
      throw new Error(details || data?.error || `Ошибка сервера (${response.status}).`);
    }
    return data;
  }

  function selectedControllers() {
    return [...state.controls.values()].filter((control) => control.selected.checked);
  }

  function totals(controllers) {
    return controllers.reduce((sum, controller) => ({
      moduleCount: sum.moduleCount + controller.moduleCount,
      reserveCount: sum.reserveCount + controller.reserveCount,
      objectCount: sum.objectCount + controller.objectCount,
    }), { moduleCount: 0, reserveCount: 0, objectCount: 0 });
  }

  function describe(summary) {
    return `${summary.moduleCount} модулей · ${summary.reserveCount} резервов AI/AO · ${summary.objectCount} объектов`;
  }

  function refreshInputs() {
    const selected = selectedControllers();
    const count = state.controls.size;
    ui.file.disabled = state.generating;
    ui.filename.disabled = state.generating;
    ui.resource.disabled = state.generating;
    ui.generate.disabled = state.generating || !state.plan || selected.length === 0;
    ui.generate.setAttribute("aria-busy", String(state.generating));
    ui.selectAll.disabled = state.generating || count === 0 || selected.length === count;
    ui.clearSelection.disabled = state.generating || selected.length === 0;
    ui.selectionEmpty.hidden = selected.length > 0;
    ui.selectionSummary.textContent = selected.length
      ? `Выбрано ПЛК: ${selected.length} из ${count} · ${describe(totals(selected.map((control) => control.controller)))}`
      : "ПЛК не выбраны";
    for (const control of state.controls.values()) {
      control.selected.disabled = state.generating;
      control.name.disabled = state.generating || !control.selected.checked;
    }
  }

  function changed() {
    ui.result.hidden = true;
    showNotice(ui.errors, []);
    refreshInputs();
  }

  function selectAll(selected) {
    if (state.generating) return;
    for (const control of state.controls.values()) control.selected.checked = selected;
    changed();
  }

  function renderControllers() {
    const fragment = document.createDocumentFragment();
    for (const controller of state.plan.controllers) {
      const section = document.createElement("section");
      section.className = "temporary-st-plc techobjects-plc";
      const heading = document.createElement("label");
      heading.className = "temporary-st-plc-heading";
      const selected = document.createElement("input");
      selected.type = "checkbox";
      selected.checked = false;
      selected.setAttribute("aria-label", `Создать XLS для ${controller.name}`);
      const title = document.createElement("strong");
      title.textContent = `ПЛК ${controller.name}`;
      heading.append(selected, title);
      const source = document.createElement("p");
      source.className = "techobjects-source";
      source.textContent = [controller.sourceFcs && `FCS: ${controller.sourceFcs}`, controller.cabinet && `Шкаф: ${controller.cabinet}`].filter(Boolean).join(" · ");
      const field = document.createElement("label");
      field.className = "temporary-field";
      const caption = document.createElement("span");
      caption.textContent = "Имя ПЛК в объектах";
      const name = document.createElement("input");
      name.type = "text";
      name.value = controller.name;
      name.maxLength = 100;
      name.spellcheck = false;
      name.autocomplete = "off";
      name.setAttribute("aria-label", `Имя ПЛК для ${controller.sourceFcs || controller.name}`);
      field.append(caption, name);
      const types = document.createElement("p");
      types.className = "techobjects-counts";
      types.textContent = ["ai", "ao", "di", "do"].map((type) => `${type.toUpperCase()}: ${controller.types?.[type] ?? 0}`).join(" · ");
      const summary = document.createElement("p");
      summary.className = "techobjects-counts";
      summary.textContent = describe(controller);
      section.append(heading, source, field, types, summary);
      fragment.append(section);
      state.controls.set(controller.key, { controller, selected, name });
      selected.addEventListener("change", changed);
      name.addEventListener("input", changed);
    }
    ui.plcs.replaceChildren(fragment);
    ui.summary.textContent = `Найдено ПЛК: ${state.plan.controllers.length} · ${describe(totals(state.plan.controllers))}`;
  }

  function validatePlan(plan) {
    if (!plan || !Array.isArray(plan.controllers)) throw new Error("Сервер не вернул список ПЛК.");
    if (!plan.controllers.length) throw new Error("В книге IO не найдены модули AI, AO, DI, DO для создания технологических объектов.");
    const keys = new Set();
    for (const controller of plan.controllers) {
      if (!controller || typeof controller.key !== "string" || !controller.key || keys.has(controller.key)
        || typeof controller.name !== "string" || !controller.name
        || ["moduleCount", "reserveCount", "objectCount"].some((key) => !Number.isSafeInteger(controller[key]) || controller[key] < 0)) {
        throw new Error("Сервер вернул некорректный список ПЛК.");
      }
      keys.add(controller.key);
    }
  }

  async function previewFile() {
    if (state.generating) return;
    const version = ++state.previewVersion;
    state.previewController?.abort();
    state.previewController = null;
    state.file = ui.file.files[0] || null;
    state.plan = null;
    state.controls.clear();
    ui.plcs.replaceChildren();
    ui.preview.hidden = true;
    ui.preview.setAttribute("aria-busy", "false");
    ui.result.hidden = true;
    showNotice(ui.errors, []);
    showNotice(ui.warnings, []);
    refreshInputs();
    const file = state.file;
    if (!file) {
      ui.status.textContent = "Выберите исходную книгу Excel IO (.xlsx) для предварительного просмотра.";
      return;
    }
    if (!/\.xlsx$/i.test(file.name) || file.size > MAX_FILE_BYTES || file.size === 0) {
      ui.status.textContent = "Файл не загружен.";
      showNotice(ui.errors, !/\.xlsx$/i.test(file.name)
        ? "Выберите исходную книгу IO в формате .xlsx. Файлы примеров .xls описывают формат результата."
        : file.size === 0 ? "Выбранный файл пуст. Выберите книгу Excel IO с данными." : "Размер книги превышает 16 МБ. Выберите файл меньшего размера.");
      return;
    }
    const controller = new AbortController();
    state.previewController = controller;
    const body = new FormData();
    body.append("file", file);
    ui.status.textContent = `Читаю ${file.name}…`;
    ui.preview.setAttribute("aria-busy", "true");
    try {
      const plan = await requestJSON("/api/techobjects/preview", { method: "POST", body, signal: controller.signal });
      if (version !== state.previewVersion || file !== state.file) return;
      validatePlan(plan);
      state.plan = plan;
      renderControllers();
      showNotice(ui.warnings, plan.warnings || []);
      ui.preview.hidden = false;
      ui.status.textContent = `Книга ${file.name} прочитана. Выберите ПЛК и проверьте их имена.`;
    } catch (error) {
      if (version !== state.previewVersion || file !== state.file || error?.name === "AbortError") return;
      state.plan = null;
      ui.status.textContent = "Не удалось прочитать книгу. Проверьте файл и загрузите его снова.";
      showNotice(ui.errors, error.message);
    } finally {
      if (version === state.previewVersion && file === state.file) {
        state.previewController = null;
        ui.preview.setAttribute("aria-busy", "false");
        refreshInputs();
      }
    }
  }

  function configuration() {
    const resource = ui.resource.value.trim();
    const resourceNumber = Number(resource);
    if (!/^\d+$/.test(resource) || !Number.isSafeInteger(resourceNumber) || resourceNumber < 1 || resourceNumber > 2147483647) {
      ui.resource.focus();
      throw new Error("Номер ресурса ПЛК должен быть целым числом от 1 до 2147483647.");
    }
    const names = new Set();
    const controllers = selectedControllers().map((control) => {
      const name = control.name.value.trim();
      let error = "";
      if (!/^[A-Za-z0-9_]{1,100}$/.test(name)) error = `${control.controller.sourceFcs || control.controller.name}: имя ПЛК должно содержать только латиницу, цифры и подчёркивания (до 100 символов).`;
      else if (names.has(name.toLowerCase())) error = `Имя ПЛК ${name} повторяется. Укажите разные имена для выбранных контроллеров.`;
      if (error) { control.name.focus(); throw new Error(error); }
      names.add(name.toLowerCase());
      return { key: control.controller.key, name };
    });
    if (!controllers.length) throw new Error("Выберите хотя бы один ПЛК для создания XLS.");
    return { controllers, resourceNumber };
  }

  function renderDownloads(files) {
    if (!Array.isArray(files) || !files.length) throw new Error("Сервер не вернул XLS-файлы для скачивания.");
    const fragment = document.createDocumentFragment();
    for (const file of files) {
      if (!file || typeof file.fileName !== "string" || !/\.xls$/i.test(file.fileName) || /[/\\]/.test(file.fileName)
        || typeof file.url !== "string" || typeof file.fcs !== "string" || !file.fcs) {
        throw new Error("Сервер вернул некорректное описание XLS-файла.");
      }
      const url = new URL(file.url, window.location.href);
      const prefix = "/api/output/";
      if (url.origin !== window.location.origin || url.username || url.password || url.search || url.hash
        || !url.pathname.startsWith(prefix) || decodeURIComponent(url.pathname.slice(prefix.length)) !== file.fileName) {
        throw new Error("Сервер вернул недопустимый адрес файла.");
      }
      const row = document.createElement("li");
      row.className = "temporary-download-row";
      const details = document.createElement("div");
      const heading = document.createElement("strong");
      heading.textContent = file.fcs;
      const name = document.createElement("p");
      name.textContent = `${file.fileName} · ${file.summary?.moduleCount ?? "—"} модулей · ${file.summary?.reserveCount ?? "—"} резервов AI/AO`;
      details.append(heading, name);
      const link = document.createElement("a");
      link.className = "button button-secondary button-small";
      link.href = url.href;
      link.download = file.fileName;
      link.textContent = "Скачать XLS";
      link.setAttribute("aria-label", `Скачать XLS для ${file.fcs}`);
      row.append(details, link);
      fragment.append(row);
    }
    ui.downloads.replaceChildren(fragment);
  }

  async function generate(event) {
    event.preventDefault();
    if (!state.plan || !state.file || state.generating) return;
    let objects;
    const fileName = ui.filename.value.trim().replace(/\.xls$/i, "");
    try {
      if (!fileName || fileName.length > 180 || /[<>:"/\\|?*\u0000-\u001f]/.test(fileName) || /[. ]$/.test(fileName)) {
        ui.filename.focus();
        throw new Error("Укажите базовое имя файла до 180 символов без символов < > : \" / \\ | ? * и точки в конце.");
      }
      objects = configuration();
    } catch (error) { showNotice(ui.errors, error.message); return; }
    const file = state.file;
    const version = state.previewVersion;
    const stats = totals(selectedControllers().map((control) => control.controller));
    const body = new FormData();
    body.append("file", file);
    body.append("fileName", fileName);
    body.append("objects", JSON.stringify(objects));
    state.generating = true;
    ui.result.hidden = true;
    showNotice(ui.errors, []);
    showNotice(ui.warnings, state.plan.warnings || []);
    ui.status.textContent = `Создаю XLS технологических объектов по книге ${file.name}…`;
    refreshInputs();
    try {
      const result = await requestJSON("/api/techobjects/generate", { method: "POST", body });
      if (version !== state.previewVersion || file !== state.file) return;
      renderDownloads(result?.files);
      ui.resultName.textContent = `Файлов создано: ${result.files.length} · по одному на ПЛК`;
      ui.resultSummary.textContent = describe({
        moduleCount: result.summary?.moduleCount ?? stats.moduleCount,
        reserveCount: result.summary?.reserveCount ?? stats.reserveCount,
        objectCount: result.summary?.objectCount ?? stats.objectCount,
      });
      ui.result.hidden = false;
      showNotice(ui.warnings, [...(state.plan.warnings || []), ...(result.warnings || [])]);
      ui.status.textContent = "XLS-файлы сохранены в папке результатов. Скачайте файл нужного ПЛК для импорта технологических объектов.";
      window.dispatchEvent(new Event("schemegen:outputs-changed"));
    } catch (error) {
      if (version !== state.previewVersion || file !== state.file) return;
      ui.status.textContent = "Не удалось создать XLS. Проверьте параметры и повторите.";
      showNotice(ui.errors, error.message);
    } finally {
      state.generating = false;
      refreshInputs();
    }
  }

  ui.file.addEventListener("change", previewFile);
  ui.filename.addEventListener("input", changed);
  ui.resource.addEventListener("input", changed);
  ui.selectAll.addEventListener("click", () => selectAll(true));
  ui.clearSelection.addEventListener("click", () => selectAll(false));
  ui.form.addEventListener("submit", generate);
  refreshInputs();
})();
