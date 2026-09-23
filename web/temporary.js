(() => {
  "use strict";

  const MAX_FILE_BYTES = 2 * 1024 * 1024;
  const MAX_IO_FILE_BYTES = 16 * 1024 * 1024;
  const MAX_MODULES = 4096;
  const MAX_PHYSICAL_ID = 2147483647;
  const tabs = [...document.querySelectorAll(".workspace-tab")];
  const $ = (id) => document.getElementById(id);
  const ui = {
    form: $("temporary-form"), file: $("temporary-file"), filename: $("temporary-filename"),
    generate: $("temporary-generate"), context: [...document.querySelectorAll("[data-temporary-context]")],
    description: $("temporary-profile-description"), profileBadge: $("temporary-profile-badge"), status: $("temporary-status"),
    errors: $("temporary-errors"), warnings: $("temporary-warnings"), preview: $("temporary-preview"),
    summary: $("temporary-summary"), search: $("temporary-search"), rows: $("temporary-rows"),
    searchEmpty: $("temporary-search-empty"), result: $("temporary-result"),
    resultName: $("temporary-result-name"), resultSummary: $("temporary-result-summary"),
    downloads: $("temporary-downloads"),
    mode: $("temporary-mode"), modeNote: $("temporary-mode-note"), previewNote: $("temporary-preview-note"),
    stSettings: $("temporary-st-settings"), stPOUs: $("temporary-st-pous"), stEmpty: $("temporary-st-empty"), stExtraNote: $("temporary-st-extra-note"),
    stSelectAll: $("temporary-st-select-all"), stClearSelection: $("temporary-st-clear-selection"),
    stSelectionSummary: $("temporary-st-selection-summary"), stSelectionEmpty: $("temporary-st-selection-empty"),
    diagnosticSettings: $("temporary-diagnostic-settings"), diagnosticPLCs: $("temporary-diagnostic-plcs"),
    diagnosticEmpty: $("temporary-diagnostic-empty"), diagnosticSelectAll: $("temporary-diagnostic-select-all"),
    diagnosticClearSelection: $("temporary-diagnostic-clear-selection"), diagnosticSelectionSummary: $("temporary-diagnostic-selection-summary"),
    diagnosticSelectionEmpty: $("temporary-diagnostic-selection-empty"), diagnosticResource: $("temporary-diagnostic-resource"),
    diagnosticResourceField: $("temporary-diagnostic-resource-field"), contextNote: $("temporary-context-note"), groupHeading: $("temporary-group-heading"),
    fileCaption: $("temporary-file-caption"), scaleHeading: $("temporary-scale-heading"), sourceHeading: $("temporary-source-heading"),
  };
  const state = { file: null, plan: null, profileReady: false, profileError: "", profileDescription: ui.description.textContent || "Значения из экспортированного примера. Измените их, если импортируете в другой проект.", previewVersion: 0, previewController: null, generating: false, mode: "fbd", st: new Map(), stControls: new Map(), stFCSControls: new Map() };
  let searchTimer;
  const diagnosticSelection = new Set();
  const diagnosticControls = new Map();

  function activateTab(tab, focus = false) {
    for (const item of tabs) {
      const selected = item === tab;
      item.setAttribute("aria-selected", String(selected));
      item.tabIndex = selected ? 0 : -1;
      $(item.getAttribute("aria-controls")).hidden = !selected;
    }
    document.querySelector(".skip-link").setAttribute("href", `#${tab.getAttribute("aria-controls")}`);
    if (focus) tab.focus();
  }

  for (const tab of tabs) {
    tab.addEventListener("click", () => activateTab(tab));
    tab.addEventListener("keydown", (event) => {
      let index = tabs.indexOf(tab);
      if (event.key === "ArrowRight") index = (index + 1) % tabs.length;
      else if (event.key === "ArrowLeft") index = (index + tabs.length - 1) % tabs.length;
      else if (event.key === "Home") index = 0;
      else if (event.key === "End") index = tabs.length - 1;
      else return;
      event.preventDefault();
      activateTab(tabs[index], true);
    });
  }

  async function requestJSON(url, options) {
    const response = await fetch(url, options);
    let data;
    try {
      data = await response.json();
    } catch {
      throw new Error(`Сервер вернул некорректный ответ (${response.status}).`);
    }
    if (!response.ok) {
      const details = Array.isArray(data.errors)
        ? data.errors.map((item) => typeof item === "string" ? item : `${item.row ? `Строка ${item.row}: ` : ""}${item.message || item.error || "Ошибка разметки"}`).join("\n")
        : "";
      throw new Error(details || data.error || `Ошибка сервера (${response.status}).`);
    }
    return data;
  }

  function showNotice(element, messages) {
    const values = (Array.isArray(messages) ? messages : [messages]).filter(Boolean);
    element.replaceChildren();
    for (const value of [...new Set(values)]) {
      const paragraph = document.createElement("p");
      paragraph.textContent = String(value);
      element.append(paragraph);
    }
    element.hidden = values.length === 0;
  }

  function updateGenerateButton() {
    ui.generate.disabled = !state.plan || !state.profileReady || state.generating || (state.mode === "st" && selectedGroups().length === 0)
      || (state.mode === "diagnostic" && diagnosticSelection.size === 0);
    ui.generate.setAttribute("aria-busy", String(state.generating));
  }

  function renderProfileDescription() {
    if (state.mode === "diagnostic") {
      ui.description.textContent = "Диагностика строится по исходному листу IO книги Excel: группа ПЛК → передняя и задняя панели → кадры модулей AI/AO. Внутренние рецепторы связывают панели и дочерние кадры. Номер ресурса входит в путь CardInfo и не является служебным ID. Объекты сигналов и библиотечные элементы панели должны существовать в целевом проекте.";
      return;
    }
    ui.description.textContent = state.mode === "st"
      ? "Профиль ST создаёт присваивания физическим выходам, без графических экземпляров AN_v1. Служебные ID XML SCADA назначает заново при импорте. Физические ID в настройках модулей — реальные адреса: ID 41 даёт _IO_QU41_0 … _IO_QU41_3. Они записываются в ST без замены и должны соответствовать модулям проекта."
      : state.profileDescription;
  }

  async function loadProfile() {
    try {
      const profile = await requestJSON("/api/temporary/ao/profile");
      if (!profile.context) throw new Error("В ответе сервера отсутствуют параметры профиля AN_v1.");
      for (const input of ui.context) input.value = profile.context[input.dataset.temporaryContext] ?? "";
      if (profile.description) state.profileDescription = profile.description;
      renderProfileDescription();
      state.profileReady = true;
      state.profileError = "";
    } catch (error) {
      state.profileError = `Не удалось загрузить профиль: ${error.message} Обновите страницу, чтобы повторить.`;
      showNotice(ui.errors, state.profileError);
    }
    updateGenerateButton();
  }

  async function previewFile(selectedFile = ui.file.files[0] || null) {
    const version = ++state.previewVersion;
    state.previewController?.abort();
    state.previewController = null;
    state.file = selectedFile;
    state.plan = null;
    state.st.clear();
    state.stControls.clear();
    state.stFCSControls.clear();
    diagnosticSelection.clear();
    diagnosticControls.clear();
    ui.diagnosticPLCs.replaceChildren();
    ui.diagnosticEmpty.hidden = false;
    ui.stPOUs.replaceChildren();
    ui.stEmpty.hidden = false;
    ui.stExtraNote.hidden = true;
    refreshSTSelection();
    refreshDiagnosticSelection();
    ui.preview.hidden = true;
    ui.preview.setAttribute("aria-busy", "false");
    ui.result.hidden = true;
    ui.rows.replaceChildren();
    ui.search.value = "";
    showNotice(ui.errors, state.profileError);
    showNotice(ui.warnings, []);
    updateGenerateButton();
    if (!state.file) {
      ui.status.textContent = state.mode === "diagnostic" ? "Выберите Excel-файл Full_IO (.xlsx) для предварительного просмотра." : "Выберите TXT-файл для предварительного просмотра.";
      return;
    }
    const diagnostic = state.mode === "diagnostic";
    if (diagnostic && !/\.xlsx$/i.test(state.file.name) || !diagnostic && !/\.(txt|tsv)$/i.test(state.file.name)) {
      ui.status.textContent = "Файл не загружен.";
      showNotice(ui.errors, diagnostic ? "Для диагностики выберите исходную книгу IO в формате .xlsx, а не перекладку AI/AO или TXT." : "Для FBD/ST выберите карту .txt или .tsv.");
      return;
    }
    if (state.file.size > (diagnostic ? MAX_IO_FILE_BYTES : MAX_FILE_BYTES)) {
      ui.status.textContent = "Файл не загружен.";
      showNotice(ui.errors, `Размер карты превышает ${diagnostic ? 16 : 2} МБ. Выберите файл меньшего размера.`);
      return;
    }
    const file = state.file;
    const controller = new AbortController();
    state.previewController = controller;
    const body = new FormData();
    body.append("file", file);
    ui.status.textContent = `Читаю ${file.name}…`;
    ui.preview.setAttribute("aria-busy", "true");
    try {
      const plan = await requestJSON(diagnostic ? "/api/temporary/diagnostic/preview" : "/api/temporary/ao/preview", { method: "POST", body, signal: controller.signal });
      if (version !== state.previewVersion || file !== state.file) return;
      if (!Array.isArray(diagnostic ? plan.controllers : plan.groups)) throw new Error("Сервер вернул некорректный план разметки.");
      state.plan = plan;
      if (diagnostic) initializeDiagnostic();
      else initializeST();
      ui.preview.hidden = false;
      renderSummary();
      ui.status.textContent = diagnostic
        ? `${file.name} · лист ${plan.sheetName}: ${plan.rowCount} строк · ${plan.controllers.length} ПЛК · ${plan.moduleCount} модулей. Выберите ПЛК и проверьте их имена.`
        : `${file.name}: прочитано строк — ${plan.rowCount}, уникальных тегов — ${plan.uniqueTagCount}. Данные готовы к преобразованию.`;
      showNotice(ui.warnings, plan.warnings || []);
      renderRows();
    } catch (error) {
      if (version !== state.previewVersion || error.name === "AbortError") return;
      ui.status.textContent = "Проверьте разметку файла и загрузите исправленную версию.";
      showNotice(ui.errors, [state.profileError, error.message]);
    } finally {
      if (version === state.previewVersion) {
        ui.preview.setAttribute("aria-busy", "false");
        state.previewController = null;
        updateGenerateButton();
      }
    }
  }

  function cell(row, value, className = "") {
    const item = document.createElement("td");
    item.textContent = value == null || value === "" ? "—" : String(value);
    if (className) item.className = className;
    row.append(item);
    return item;
  }

  function duplicateSummary(count) {
    return count ? ` · ${count} повторов — пусто` : "";
  }

  function integer(value, minimum = 0, maximum = MAX_PHYSICAL_ID) {
    const text = String(value).trim();
    const number = /^\d+$/.test(text) ? Number(text) : NaN;
    return Number.isSafeInteger(number) && number >= minimum && number <= maximum ? number : null;
  }

  function initializeST() {
    const nextIDs = new Map();
    for (const group of state.plan.groups) {
      let next = nextIDs.get(group.fcs) || 0;
      state.st.set(group.key, { selected: false, modules: group.modules.map((module) => ({ name: module.name, id: String(next++) })) });
      nextIDs.set(group.fcs, next);
    }
    renderSTControls();
  }

  function numberInput(value, minimum, maximum, label) {
    const input = document.createElement("input");
    input.type = "number";
    input.min = String(minimum);
    input.max = String(maximum);
    input.step = "1";
    input.value = String(value);
    input.required = true;
    input.setAttribute("aria-label", label);
    return input;
  }

  function field(label, input) {
    const element = document.createElement("label");
    element.className = "temporary-field";
    const caption = document.createElement("span");
    caption.textContent = label;
    element.append(caption, input);
    return element;
  }

  function renderSTControls() {
    const fragment = document.createDocumentFragment();
    state.stControls.clear();
    state.stFCSControls.clear();
    for (const group of state.plan.groups) {
      let plc = state.stFCSControls.get(group.fcs);
      if (!plc) {
        const section = document.createElement("section");
        section.className = "temporary-st-plc";
        section.setAttribute("aria-label", `ПЛК ${group.fcs}`);
        const heading = document.createElement("label");
        heading.className = "temporary-st-plc-heading";
        const selected = document.createElement("input");
        selected.type = "checkbox";
        selected.setAttribute("aria-label", `Все POU ПЛК ${group.fcs}`);
        const title = document.createElement("strong");
        title.textContent = `ПЛК ${group.fcs}`;
        const count = document.createElement("span");
        heading.append(selected, title, count);
        const list = document.createElement("div");
        list.className = "temporary-st-pou-list";
        section.append(heading, list);
        plc = { selected, count, list, groups: [] };
        state.stFCSControls.set(group.fcs, plc);
        const selection = plc;
        selected.addEventListener("change", () => setSTSelection(selection.groups, selected.checked));
        fragment.append(section);
      }
      plc.groups.push(group);
      const config = state.st.get(group.key);
      const row = document.createElement("div");
      row.className = "temporary-st-pou";
      const details = document.createElement("details");
      details.className = "temporary-st-module-settings";
      const summary = document.createElement("summary");
      summary.textContent = "Модули и ID";
      summary.setAttribute("aria-label", `Модули и ID ${group.pouName}_channels, ${group.fcs}`);
      const title = document.createElement("span");
      const toolbar = document.createElement("div");
      toolbar.className = "temporary-st-pou-toolbar";
      const enabled = document.createElement("input");
      enabled.type = "checkbox";
      enabled.checked = config.selected;
      enabled.setAttribute("aria-label", `Генерировать ${group.pouName}_channels, ${group.fcs}`);
      const enabledLabel = document.createElement("label");
      enabledLabel.className = "temporary-st-enabled";
      enabledLabel.append(enabled, title);
      const count = numberInput(config.modules.length, group.modules.length, MAX_MODULES, `Количество модулей ${group.pouName}_channels, ${group.fcs}`);
      toolbar.append(field(`Модулей × 4 (из TXT: ${group.modules.length})`, count));
      const list = document.createElement("div");
      list.className = "temporary-st-module-list";
      details.append(summary, toolbar, list);
      row.append(enabledLabel, details);
      const control = { group, config, details, title, enabled, count, list, ids: [] };
      state.stControls.set(group.key, control);
      enabled.addEventListener("change", () => setSTSelection([group], enabled.checked));
      count.addEventListener("change", () => {
        try {
          resizeSTModules(control);
          showNotice(ui.errors, state.profileError);
          changedST();
        } catch (error) { showNotice(ui.errors, error.message); }
      });
      renderModuleIDs(control);
      plc.list.append(row);
    }
    ui.stPOUs.replaceChildren(fragment);
    ui.stEmpty.hidden = true;
    refreshSTSelection();
  }

  function renderModuleIDs(control) {
    const fragment = document.createDocumentFragment();
    control.ids = control.config.modules.map((module, index) => {
      const input = numberInput(module.id, 0, MAX_PHYSICAL_ID, `Физический ID ${module.name}, ${control.group.fcs}`);
      input.dataset.module = module.name;
      input.addEventListener("input", () => { module.id = input.value; ui.result.hidden = true; });
      input.addEventListener("change", changedST);
      const label = `${module.name} → ID${index >= control.group.modules.length ? " · резерв" : ""}`;
      fragment.append(field(label, input));
      return input;
    });
    control.list.replaceChildren(fragment);
    refreshSTControl(control);
  }

  function refreshSTControl(control) {
    const { group, config } = control;
    control.title.textContent = `${group.pouName}_channels · ${config.modules.length} × 4`;
    control.enabled.checked = config.selected;
    control.enabled.disabled = state.generating || state.mode !== "st";
    for (const input of [control.count, ...control.ids]) input.disabled = state.generating || state.mode !== "st" || !config.selected;
  }

  function resizeSTModules(control) {
    const { group, config } = control;
    const count = integer(control.count.value, group.modules.length, MAX_MODULES);
    if (count === null) {
      control.details.open = true;
      throw new Error(`${group.pouName}_channels (${group.fcs}): количество модулей должно быть от ${group.modules.length} до ${MAX_MODULES}. Модули из TXT удалять нельзя.`);
    }
    if (count === config.modules.length) return;
    if (count < config.modules.length) config.modules.length = count;
    else {
      const used = new Set();
      for (const sibling of state.plan.groups.filter((item) => item.fcs === group.fcs)) {
        for (const module of state.st.get(sibling.key).modules) {
          const id = integer(module.id);
          if (id !== null) used.add(id);
        }
      }
      let nextID = 0;
      let nextSuffix = Math.max(...config.modules.map((module) => Number(module.name.match(/_(\d+)$/)?.[1] || 0))) + 1;
      while (config.modules.length < count) {
        while (used.has(nextID)) nextID += 1;
        config.modules.push({ name: `${group.prefix}_${String(nextSuffix++).padStart(2, "0")}`, id: String(nextID) });
        used.add(nextID++);
      }
    }
    renderModuleIDs(control);
  }

  function selectedGroups() {
    return state.plan?.groups?.filter((group) => state.st.get(group.key)?.selected) || [];
  }

  function setSTSelection(groups, selected) {
    if (!state.plan || state.generating || state.mode !== "st") return;
    for (const group of groups) {
      state.st.get(group.key).selected = selected;
      refreshSTControl(state.stControls.get(group.key));
    }
    showNotice(ui.errors, state.profileError);
    changedST();
  }

  function refreshSTSelection() {
    const groups = selectedGroups();
    const fcsCount = new Set(groups.map((group) => group.fcs)).size;
    const disabled = state.generating || state.mode !== "st" || !state.plan;
    for (const plc of state.stFCSControls.values()) {
      const count = plc.groups.filter((group) => state.st.get(group.key).selected).length;
      plc.selected.checked = count === plc.groups.length;
      plc.selected.indeterminate = count > 0 && count < plc.groups.length;
      plc.selected.disabled = disabled;
      plc.count.textContent = `${count}/${plc.groups.length} POU`;
    }
    ui.stSelectAll.disabled = disabled || groups.length === state.plan?.groups?.length;
    ui.stClearSelection.disabled = disabled || groups.length === 0;
    ui.stSelectionSummary.textContent = state.plan?.groups
      ? `Выбрано ${groups.length} из ${state.plan.groups.length} POU · ${fcsCount} ПЛК → ${fcsCount} XML`
      : "POU не выбраны";
    ui.stSelectionEmpty.hidden = !state.plan || groups.length !== 0;
    updateGenerateButton();
  }

  function changedST() {
    ui.result.hidden = true;
    refreshSTSelection();
    ui.stExtraNote.hidden = !state.plan.groups.some((group) => state.st.get(group.key).selected && state.st.get(group.key).modules.length > group.modules.length);
    renderSummary();
    renderRows();
  }

  function stStats() {
    const groups = selectedGroups();
    const moduleCount = groups.reduce((sum, group) => sum + state.st.get(group.key).modules.length, 0);
    // Count repeated invocations among the selected POUs, independently for each FCS.
    const seen = new Set();
    let repeats = 0;
    for (const group of groups) for (const module of group.modules) for (const channel of module.channels) {
      const key = `${group.fcs}\0${channel.tag.toLocaleLowerCase("ru-RU")}`;
      if (seen.has(key)) repeats += 1;
      seen.add(key);
    }
    return { pouCount: groups.length, fcsCount: new Set(groups.map((group) => group.fcs)).size, moduleCount, assignments: moduleCount * 4, repeats };
  }

  function diagnosticFrameName(fcs, module) {
    if (!["AI16H", "AOC4H"].includes(module.type)) return "";
    const separator = fcs.lastIndexOf("_SC_");
    const controller = separator < 0 ? fcs : fcs.slice(separator + 4);
    return `${module.type === "AI16H" ? "AI" : "AO"}_${controller}_${module.name}_${module.type}`;
  }

  function controllerName(controller) {
    return diagnosticControls.get(controller.key)?.name.value.trim() || controller.name;
  }

  function controllerStats(controller) {
    const types = { AI16H: 0, AOC4H: 0, DI32: 0, DO32P: 0 };
    for (const module of controller.modules) types[module.type] = (types[module.type] || 0) + 1;
    // Native exports retain both cabinet panels, including an empty panel.
    return { types, frames: 3 + types.AI16H + types.AOC4H, channels: controller.modules.reduce((sum, module) => sum + module.channels.length, 0) };
  }

  function diagnosticTypeCounts(types) {
    return `AI ${types.AI16H} · AO ${types.AOC4H} · DI ${types.DI32} · DO ${types.DO32P}`;
  }

  function renderDiagnosticHierarchy(control) {
    const fragment = document.createDocumentFragment();
    const name = controllerName(control.controller);
    for (const panel of ["front", "back"]) {
      const racks = control.controller.racks.filter((rack) => rack.panel === panel).sort((a, b) => a.order - b.order);
      const title = document.createElement("strong");
      title.textContent = panel === "front" ? "Передняя панель" : "Задняя панель";
      fragment.append(title);
      if (!racks.length) {
        const empty = document.createElement("p");
        empty.textContent = "Пустая панель · сохраняется в XML";
        fragment.append(empty);
      }
      for (const rack of racks) {
        const row = document.createElement("p");
        const modules = control.controller.modules.filter((module) => module.rack === rack.name).sort((a, b) => a.slot - b.slot);
        row.textContent = `${rack.name}: ${modules.map((module) => diagnosticFrameName(name, module) || `${module.name} · ${module.type} (панель)`).join(" · ")}`;
        fragment.append(row);
      }
    }
    control.frames.replaceChildren(fragment);
  }

  function initializeDiagnostic() {
    const fragment = document.createDocumentFragment();
    for (const controller of state.plan.controllers) {
        const section = document.createElement("section");
        section.className = "temporary-st-plc temporary-diagnostic-plc";
        section.setAttribute("aria-label", `Диагностика ПЛК ${controller.name}`);
        const heading = document.createElement("label");
        heading.className = "temporary-st-plc-heading";
        const selected = document.createElement("input");
        selected.type = "checkbox";
        selected.setAttribute("aria-label", `Все кадры ПЛК ${controller.name}`);
        const title = document.createElement("strong");
        title.textContent = `${controller.sourceFcs} · ${controller.cabinet}`;
        const count = document.createElement("span");
        heading.append(selected, title, count);
        const name = document.createElement("input");
        name.type = "text";
        name.value = controller.name;
        name.required = true;
        name.maxLength = 100;
        name.pattern = "[A-Za-z0-9_]+";
        name.spellcheck = false;
        name.autocomplete = "off";
        name.setAttribute("aria-label", `Имя ПЛК для ${controller.sourceFcs}, ${controller.cabinet}`);
        const nameField = field("Имя ПЛК в проекте SCADA", name);
        const types = document.createElement("p");
        types.className = "temporary-diagnostic-types";
        const stats = controllerStats(controller);
        types.textContent = diagnosticTypeCounts(stats.types);
        count.textContent = `${stats.frames} кадров`;
        const details = document.createElement("details");
        details.className = "temporary-diagnostic-frames";
        const summary = document.createElement("summary");
        summary.textContent = "Панели, стойки и дочерние кадры";
        const frames = document.createElement("div");
        details.append(summary, frames);
        section.append(heading, nameField, types, details);
        const control = { selected, count, frames, name, controller };
        diagnosticControls.set(controller.key, control);
        selected.addEventListener("change", () => setDiagnosticSelection([controller.key], selected.checked));
        name.addEventListener("input", () => {
          ui.result.hidden = true;
          renderDiagnosticHierarchy(control);
          renderRows();
        });
        renderDiagnosticHierarchy(control);
        fragment.append(section);
    }
    ui.diagnosticPLCs.replaceChildren(fragment);
    ui.diagnosticEmpty.hidden = true;
    refreshDiagnosticSelection();
  }

  function selectedDiagnosticControllers() {
    return state.plan?.controllers?.filter((controller) => diagnosticSelection.has(controller.key)) || [];
  }

  function diagnosticStats() {
    const controllers = selectedDiagnosticControllers();
    const stats = { fcsCount: controllers.length, frames: 0, channels: 0, modules: 0, types: { AI16H: 0, AOC4H: 0, DI32: 0, DO32P: 0 } };
    for (const controller of controllers) {
      const counts = controllerStats(controller);
      stats.frames += counts.frames;
      stats.channels += counts.channels;
      stats.modules += controller.modules.length;
      for (const type of Object.keys(stats.types)) stats.types[type] += counts.types[type];
    }
    return stats;
  }

  function setDiagnosticSelection(fcsNames, selected) {
    if (!state.plan || state.generating || state.mode !== "diagnostic") return;
    for (const fcs of fcsNames) {
      if (selected) diagnosticSelection.add(fcs);
      else diagnosticSelection.delete(fcs);
    }
    ui.result.hidden = true;
    showNotice(ui.errors, state.profileError);
    refreshDiagnosticSelection();
    renderSummary();
    renderRows();
  }

  function refreshDiagnosticSelection() {
    const disabled = state.generating || state.mode !== "diagnostic" || !state.plan;
    for (const [fcs, plc] of diagnosticControls) {
      plc.selected.checked = diagnosticSelection.has(fcs);
      plc.selected.disabled = disabled;
      plc.name.disabled = disabled || !plc.selected.checked;
    }
    ui.diagnosticSelectAll.disabled = disabled || diagnosticSelection.size === diagnosticControls.size;
    ui.diagnosticClearSelection.disabled = disabled || diagnosticSelection.size === 0;
    const stats = diagnosticStats();
    ui.diagnosticSelectionSummary.textContent = state.plan?.controllers
      ? `Выбрано ${stats.fcsCount} из ${diagnosticControls.size} ПЛК · ${stats.frames} кадров → ${stats.fcsCount} XML`
      : "ПЛК не выбраны";
    ui.diagnosticSelectionEmpty.hidden = !state.plan || diagnosticSelection.size !== 0;
    updateGenerateButton();
  }

  function refreshInputs() {
    const diagnostic = state.mode === "diagnostic";
    for (const input of [ui.file, ui.filename, ui.mode]) input.disabled = state.generating;
    for (const input of ui.context) {
      const relevant = !diagnostic || ["project", "version"].includes(input.dataset.temporaryContext);
      const label = input.closest("label");
      if (label) label.hidden = !relevant;
      input.disabled = state.generating || !relevant;
    }
    ui.diagnosticResourceField.hidden = !diagnostic;
    ui.diagnosticResource.disabled = state.generating || !diagnostic;
    for (const control of state.stControls.values()) refreshSTControl(control);
    refreshSTSelection();
    refreshDiagnosticSelection();
  }

  function renderSummary() {
    if (!state.plan) return;
    if (state.mode === "diagnostic") {
      const stats = diagnosticStats();
      ui.summary.textContent = `${stats.fcsCount} ПЛК / XML · ${stats.frames} кадров · ${stats.modules} модулей · ${stats.channels} каналов · ${diagnosticTypeCounts(stats.types)}`;
    } else if (state.mode === "st") {
      const stats = stStats();
      ui.summary.textContent = `${stats.fcsCount} FCS / XML · ${stats.pouCount} POU ST · ${stats.moduleCount} модулей · ${stats.assignments} присваиваний · ${stats.repeats} повторов — включены в ST`;
    } else {
      const plan = state.plan;
      const fcsCount = new Set(plan.groups.map((group) => group.fcs)).size;
      ui.summary.textContent = `${fcsCount} FCS / XML · ${plan.groupCount} POU · ${plan.moduleCount} модулей · ${plan.channelCount} каналов · ${plan.reserveCount} резервов${duplicateSummary(plan.duplicateCount)}`;
    }
  }

  function stConfiguration() {
    const pous = [];
    const physicalIDs = new Map();
    let moduleTotal = 0;
    for (const group of selectedGroups()) {
      const control = state.stControls.get(group.key);
      resizeSTModules(control);
      const moduleIds = control.config.modules.map((module, index) => {
        const value = integer(module.id);
        const key = `${group.fcs}\0${value}`;
        let error = "";
        if (value === null) error = `${group.fcs} / ${module.name}: ID должен быть целым числом от 0 до ${MAX_PHYSICAL_ID}.`;
        else if (physicalIDs.has(key)) error = `${group.fcs}: ID ${value} повторяется у ${physicalIDs.get(key)} и ${module.name}. Укажите разные физические ID.`;
        if (error) {
          control.details.open = true;
          control.ids[index].focus();
          throw new Error(error);
        }
        physicalIDs.set(key, module.name);
        return value;
      });
      moduleTotal += moduleIds.length;
      pous.push({ groupKey: group.key, moduleCount: moduleIds.length, moduleIds });
    }
    if (!pous.length) throw new Error("Выберите хотя бы одну POU для генерации ST.");
    if (moduleTotal > MAX_MODULES) throw new Error(`В одном запросе ST допускается не более ${MAX_MODULES} модулей.`);
    return { pous };
  }

  function changeMode() {
    if (state.generating) { ui.mode.value = state.mode; return; }
    const wasDiagnostic = state.mode === "diagnostic";
    state.mode = ["st", "diagnostic"].includes(ui.mode.value) ? ui.mode.value : "fbd";
    renderProfileDescription();
    const st = state.mode === "st";
    const diagnostic = state.mode === "diagnostic";
    ui.stSettings.hidden = !st;
    ui.diagnosticSettings.hidden = !diagnostic;
    ui.profileBadge.textContent = diagnostic ? "Диагностика ПЛК · IO" : "Профиль AN_v1";
    ui.groupHeading.textContent = diagnostic ? "Кадр / ПЛК" : "POU / FCS";
    ui.scaleHeading.textContent = diagnostic ? "Тип модуля" : "Шкала";
    ui.sourceHeading.textContent = diagnostic ? "Строка Excel" : "Строка TXT";
    ui.fileCaption.textContent = diagnostic ? "Книга IO · .xlsx, до 16 МБ" : "Карта сигналов · TXT / TSV, до 2 МБ";
    ui.file.accept = diagnostic ? ".xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" : ".txt,.tsv,text/plain,text/tab-separated-values";
    ui.contextNote.hidden = diagnostic;
    ui.modeNote.textContent = diagnostic
      ? "Полная группа диагностики ПЛК из исходного Excel IO: панели шкафа, модули и внутренние переходы. Один выбранный ПЛК — один XML."
      : st
      ? "ST POU AO_A11_channels, AO_A12_channels. В выбранных POU сохраняются все теги, включая повторные. Каждый ПЛК экспортируется отдельно."
      : "Графические POU AO_A11, AO_A12. Повторные теги оставляют пустые позиции.";
    ui.previewNote.textContent = diagnostic
      ? "AI — 16 каналов, AO — 4, DI/DO — 32. AI/AO открывают дочерние кадры; DI/DO показаны на панели без отдельных кадров, как в образце. Таблица включает свободные и резервированные подключения. Диагностика не создаёт объекты FBD/ST."
      : st
      ? "В ST повторные теги сохраняются. ID задаёт физический модуль: _IO_QU41_0 … _IO_QU41_3. Таблица показывает выбранные POU; дополнительные модули используют резервные теги."
      : "В каждом модуле 4 канала. Свободные каналы получают резерв. Повторное размещение тега остаётся пустым — без блока и без заглушки; остальные позиции не сдвигаются.";
    if (wasDiagnostic !== diagnostic) {
      ui.file.value = "";
      if (ui.filename.value === "AO_import" && diagnostic) ui.filename.value = "diagnostic_import";
      else if (ui.filename.value === "diagnostic_import" && !diagnostic) ui.filename.value = "AO_import";
      previewFile(null);
    }
    refreshInputs();
    ui.result.hidden = true;
    showNotice(ui.errors, state.profileError);
    showNotice(ui.warnings, state.plan?.warnings || []);
    renderSummary();
    renderRows();
  }

  function renderRows() {
    if (!state.plan) return;
    if (state.mode === "diagnostic") { renderDiagnosticRows(); return; }
    const query = ui.search.value.trim().toLocaleLowerCase("ru-RU");
    const fragment = document.createDocumentFragment();
    let count = 0;
    const st = state.mode === "st";
    for (const group of st ? selectedGroups() : state.plan.groups) {
      let firstGroupRow = true;
      const modules = [...group.modules];
      if (st) for (const module of state.st.get(group.key).modules.slice(group.modules.length)) {
        modules.push({ name: module.name, channels: Array.from({ length: 4 }, (_, channel) => ({ channel, tag: `_${group.fcs}_${module.name}_${channel}`, reserve: true, min: "0.0", max: "100.0" })) });
      }
      for (const [moduleIndex, module] of modules.entries()) {
        let firstModuleRow = true;
        for (const channel of module.channels) {
          const duplicate = Boolean(channel.duplicate);
          const skipped = duplicate && !st;
          const physicalID = st ? state.st.get(group.key).modules[moduleIndex].id : "";
          const searchable = [group.pouName, st ? `${group.pouName}_channels` : "", group.fcs, group.prefix, module.name, module.mainModule, module.redundantModule, channel.tag, channel.duplicateOf, channel.channel, st ? `_IO_QU${physicalID}_${channel.channel}` : "", duplicate ? st ? "повтор включён ST" : "повтор пусто" : channel.reserve ? "резерв" : "сигнал"].join(" ").toLocaleLowerCase("ru-RU");
          if (query && !searchable.includes(query)) continue;
          const row = document.createElement("tr");
          row.classList.toggle("temporary-module-start", firstModuleRow);
          row.classList.toggle("temporary-group-start", firstGroupRow);
          row.classList.toggle("temporary-reserve-row", !duplicate && Boolean(channel.reserve));
          row.classList.toggle("temporary-duplicate-row", skipped);
          row.classList.toggle("temporary-st-repeated-row", duplicate && !skipped);
          const showGroupName = firstGroupRow;
          const groupCell = cell(row, showGroupName ? `${group.pouName}${st ? "_channels" : ""}` : "", "temporary-pou-cell");
          if (showGroupName) {
            const fcs = document.createElement("small");
            fcs.textContent = group.fcs;
            groupCell.append(fcs);
          } else groupCell.textContent = "";
          const moduleCell = cell(row, firstModuleRow ? module.name || module.mainModule : "", "temporary-mono");
          if (!firstModuleRow) moduleCell.textContent = "";
          if (firstModuleRow && module.redundantModule) moduleCell.title = `Резервный модуль: ${module.redundantModule}`;
          if (st && firstModuleRow) {
            const physical = document.createElement("small");
            physical.textContent = ` → ID ${physicalID}`;
            moduleCell.append(physical);
          }
          cell(row, channel.channel, "temporary-channel-cell");
          const tagCell = cell(row, skipped ? "" : channel.tag, "temporary-mono");
          if (skipped) tagCell.title = `Повтор ${channel.tag} не создаёт блок и резерв.${channel.duplicateOf ? ` Первое размещение: ${channel.duplicateOf}.` : ""}`;
          if (st) tagCell.title = `_IO_QU${physicalID}_${channel.channel}.ValueDINT := REAL_TO_DINT(${channel.tag}.OUT, ${channel.min}, ${channel.max});`;
          cell(row, skipped ? "" : `${channel.min ?? "—"} … ${channel.max ?? "—"}`, "temporary-scale-cell");
          cell(row, channel.sourceRow || "—", "temporary-source-cell");
          const badge = document.createElement("span");
          badge.className = skipped ? "temporary-duplicate-badge" : channel.reserve ? "temporary-reserve-badge" : "temporary-signal-badge";
          badge.textContent = duplicate ? st ? "Повтор в карте — включён в ST" : "Повтор — пусто" : channel.reserve ? "Резерв" : "Сигнал";
          cell(row, "").replaceChildren(badge);
          fragment.append(row);
          firstModuleRow = false;
          firstGroupRow = false;
          count += 1;
        }
      }
    }
    ui.rows.replaceChildren(fragment);
    ui.searchEmpty.hidden = count !== 0;
  }

  function renderDiagnosticRows() {
    const query = ui.search.value.trim().toLocaleLowerCase("ru-RU");
    const fragment = document.createDocumentFragment();
    let count = 0;
    for (const controller of selectedDiagnosticControllers()) {
      const name = controllerName(controller);
      for (const module of controller.modules) {
        let first = true;
        const frame = diagnosticFrameName(name, module);
        const rack = controller.racks.find((item) => item.name === module.rack);
        const panel = rack?.panel === "back" ? "Задняя панель" : "Передняя панель";
        for (const channel of module.channels) {
          const tag = channel.reserve ? `_${name}_${module.name}_${channel.channel}` : channel.tag;
          const inferredName = Boolean(frame) && !channel.reserve && channel.bindingSource === "io-rule";
          const searchable = [frame, name, controller.cabinet, controller.sourceFcs, panel, module.rack, module.name, module.type, channel.channel, tag, channel.sourceRow, channel.sourceTag, channel.reserve ? "резерв" : channel.redundant ? "резервированное подключение" : "сигнал", inferredName ? "имя по IO проверить" : ""].join(" ").toLocaleLowerCase("ru-RU");
          if (query && !searchable.includes(query)) continue;
          const row = document.createElement("tr");
          row.classList.toggle("temporary-module-start", first);
          row.classList.toggle("temporary-group-start", first);
          row.classList.toggle("temporary-reserve-row", Boolean(channel.reserve));
          row.classList.toggle("temporary-st-repeated-row", Boolean(channel.redundant));
          const group = cell(row, first ? frame || `${panel} · ${module.name}` : "", "temporary-pou-cell");
          if (first) {
            const path = document.createElement("small");
            path.textContent = `${name} / ${panel} / ${module.rack}`;
            group.append(path);
          } else group.textContent = "";
          const moduleCell = cell(row, first ? module.name : "", "temporary-mono");
          if (!first) moduleCell.textContent = "";
          cell(row, channel.channel, "temporary-channel-cell");
          const signal = cell(row, tag, "temporary-mono");
          if (channel.sourceTag) signal.title = `Тег IO: ${channel.sourceTag}`;
          if (inferredName) signal.title = `${signal.title ? `${signal.title}. ` : ""}Имя получено по правилам IO, не подтверждено перекладкой AI/AO. Проверьте объект в проекте SCADA.`;
          cell(row, module.type, "temporary-scale-cell");
          cell(row, channel.sourceRow || "—", "temporary-source-cell");
          const badge = document.createElement("span");
          badge.className = inferredName ? "temporary-inferred-badge" : channel.reserve ? "temporary-reserve-badge" : "temporary-signal-badge";
          badge.textContent = inferredName ? "Имя по IO · проверить" : channel.reserve ? "Резерв" : channel.redundant ? "Резервированное подключение" : "Сигнал";
          if (inferredName) badge.title = signal.title;
          cell(row, "").replaceChildren(badge);
          fragment.append(row);
          first = false;
          count += 1;
        }
      }
    }
    ui.rows.replaceChildren(fragment);
    ui.searchEmpty.hidden = count !== 0;
  }

  function diagnosticConfiguration() {
    const names = new Set();
    const controllers = selectedDiagnosticControllers().map((controller) => {
      const input = diagnosticControls.get(controller.key).name;
      const name = input.value.trim();
      let error = "";
      if (!/^[A-Za-z0-9_]+$/.test(name) || name.length > 100) error = `${controller.sourceFcs}: имя ПЛК должно содержать только латиницу, цифры и подчёркивания (до 100 символов).`;
      else if (names.has(name.toLowerCase())) error = `Имя ПЛК ${name} повторяется. Укажите разные имена для выбранных контроллеров.`;
      if (error) { input.focus(); throw new Error(error); }
      names.add(name.toLowerCase());
      return { key: controller.key, name };
    });
    if (!controllers.length) throw new Error("Выберите хотя бы один ПЛК для генерации кадров диагностики.");
    return { controllers };
  }

  async function generate(event) {
    event.preventDefault();
    if (!state.plan || !state.file || !state.profileReady || state.generating) return;
    const st = state.mode === "st";
    const diagnostic = state.mode === "diagnostic";
    let configuration;
    if (st) {
      try {
        configuration = stConfiguration();
        changedST();
      } catch (error) {
        showNotice(ui.errors, error.message);
        return;
      }
    }
    if (diagnostic) {
      if (!diagnosticSelection.size) {
        showNotice(ui.errors, "Выберите хотя бы один ПЛК для генерации кадров диагностики.");
        return;
      }
      if (integer(ui.diagnosticResource.value, 1) === null) {
        const details = ui.diagnosticResource.closest("details");
        if (details) details.open = true;
        ui.diagnosticResource.focus();
        showNotice(ui.errors, `Номер ресурса в пути привязки должен быть целым числом от 1 до ${MAX_PHYSICAL_ID}.`);
        return;
      }
      try { configuration = diagnosticConfiguration(); }
      catch (error) { showNotice(ui.errors, error.message); return; }
    }
    const file = state.file;
    const previewVersion = state.previewVersion;
    const stats = diagnostic ? diagnosticStats() : st ? stStats() : null;
    const context = Object.fromEntries(ui.context.filter((input) => !diagnostic || ["version", "project"].includes(input.dataset.temporaryContext)).map((input) => [input.dataset.temporaryContext, input.value.trim()]));
    if (diagnostic) context.resourceNumber = ui.diagnosticResource.value.trim();
    const body = new FormData();
    body.append("file", file);
    body.append("fileName", ui.filename.value.trim());
    body.append("context", JSON.stringify(context));
    if (st) body.append("st", JSON.stringify(configuration));
    if (diagnostic) body.append("diagnostic", JSON.stringify(configuration));
    state.generating = true;
    ui.result.hidden = true;
    showNotice(ui.errors, []);
    showNotice(ui.warnings, state.plan.warnings || []);
    ui.status.textContent = `Создаю XML ${diagnostic ? "кадров диагностики" : st ? "ST" : "FBD"} по карте ${file.name}…`;
    refreshInputs();
    try {
      const result = await requestJSON(diagnostic ? "/api/temporary/diagnostic/generate" : st ? "/api/temporary/ao/generate-st" : "/api/temporary/ao/generate", { method: "POST", body });
      if (previewVersion !== state.previewVersion || file !== state.file) return;
      renderDownloads(result.files, diagnostic);
      ui.resultName.textContent = `Файлов создано: ${result.files.length} · по одному на FCS`;
      const summary = result.summary || {};
      if (diagnostic) {
        ui.resultSummary.textContent = `${summary.frameCount ?? stats.frames} кадров · ${summary.signalCount ?? stats.channels} каналов · ${summary.graphics ?? "—"} графических элементов · ${summary.cards ?? "—"} карточек привязки`;
      } else if (st) {
        ui.resultSummary.textContent = `${summary.pouCount ?? stats.pouCount} POU ST · ${summary.assignmentCount ?? summary.signalCount ?? stats.assignments} присваиваний · ${summary.repeatedAssignmentCount ?? stats.repeats} повторов — включены в ST`;
      } else {
        const duplicateCount = summary.skippedDuplicateCount ?? state.plan.duplicateCount ?? 0;
        const blockCount = summary.blocks ?? state.plan.channelCount - duplicateCount;
        ui.resultSummary.textContent = `${summary.pouCount ?? state.plan.groupCount} POU · ${summary.signalCount ?? state.plan.channelCount} каналов · ${summary.cards ?? blockCount} карточек · ${blockCount} блоков${duplicateSummary(duplicateCount)}`;
      }
      ui.result.hidden = false;
      showNotice(ui.warnings, [...(state.plan.warnings || []), ...(result.warnings || [])]);
      ui.status.textContent = diagnostic
        ? "XML-файлы кадров сохранены в папке результатов. Скачайте файл нужного ПЛК и импортируйте в панель оператора соответствующего проекта."
        : "XML-файлы сохранены в папке результатов. Скачайте файл нужного FCS и импортируйте в соответствующий ПЛК.";
      window.dispatchEvent(new Event("schemegen:outputs-changed"));
    } catch (error) {
      if (previewVersion !== state.previewVersion || file !== state.file) return;
      ui.status.textContent = "Не удалось создать XML. Проверьте параметры и повторите.";
      showNotice(ui.errors, error.message);
    } finally {
      state.generating = false;
      refreshInputs();
    }
  }

  function renderDownloads(files, diagnostic = false) {
    if (!Array.isArray(files) || files.length === 0) throw new Error("Сервер не вернул XML-файлы для скачивания.");
    const fragment = document.createDocumentFragment();
    for (const file of files) {
      if (!file || typeof file.fileName !== "string" || !file.fileName || /[/\\]/.test(file.fileName)
        || typeof file.url !== "string" || typeof file.fcs !== "string" || !file.fcs) {
        throw new Error("Сервер вернул некорректное описание XML-файла.");
      }
      const downloadURL = new URL(file.url, window.location.href);
      const pathPrefix = "/api/output/";
      if (downloadURL.origin !== window.location.origin || downloadURL.username || downloadURL.password
        || downloadURL.search || downloadURL.hash || !downloadURL.pathname.startsWith(pathPrefix)
        || decodeURIComponent(downloadURL.pathname.slice(pathPrefix.length)) !== file.fileName) {
        throw new Error("Сервер вернул недопустимый адрес файла.");
      }
      const row = document.createElement("li");
      row.className = "temporary-download-row";
      const details = document.createElement("div");
      const heading = document.createElement("strong");
      heading.textContent = file.fcs;
      const name = document.createElement("p");
      name.textContent = diagnostic
        ? `${file.fileName} · ${file.summary?.frameCount ?? "—"} кадров`
        : `${file.fileName} · ${file.summary?.pouCount ?? "—"} POU`;
      details.append(heading, name);
      const link = document.createElement("a");
      link.className = "button button-secondary button-small";
      link.href = downloadURL.href;
      link.download = file.fileName;
      link.textContent = "Скачать XML";
      link.setAttribute("aria-label", `Скачать XML для ${file.fcs}`);
      row.append(details, link);
      fragment.append(row);
    }
    ui.downloads.replaceChildren(fragment);
  }

  ui.file.addEventListener("change", () => previewFile());
  ui.mode.value = "fbd";
  ui.mode.addEventListener("change", changeMode);
  ui.stSelectAll.addEventListener("click", () => setSTSelection(state.plan?.groups || [], true));
  ui.stClearSelection.addEventListener("click", () => setSTSelection(state.plan?.groups || [], false));
  ui.diagnosticSelectAll.addEventListener("click", () => setDiagnosticSelection([...diagnosticControls.keys()], true));
  ui.diagnosticClearSelection.addEventListener("click", () => setDiagnosticSelection([...diagnosticControls.keys()], false));
  ui.form.addEventListener("submit", generate);
  ui.form.addEventListener("invalid", (event) => {
    const details = event.target.closest("details");
    if (details) details.open = true;
  }, true);
  ui.form.addEventListener("input", () => { ui.result.hidden = true; });
  ui.search.addEventListener("input", () => {
    clearTimeout(searchTimer);
    searchTimer = setTimeout(renderRows, 120);
  });
  refreshInputs();
  loadProfile();
})();
