(() => {
  "use strict";

  const MAX_FILE_BYTES = 16 * 1024 * 1024;
  const MAX_ID = 2147483647;
  const MAX_MODULES = 4096;
  const MAX_MODULE_SUFFIX = 4095;
  const MAX_CHANNELS = 4096;
  const MAX_POUS = 128;
  const $ = (id) => document.getElementById(`skz-${id}`);
  const ui = {
    form: $("form"), file: $("file"), filename: $("filename"), mode: $("mode"), modeNote: $("mode-note"), generate: $("generate"),
    selection: $("selection"), summary: $("summary"), groups: $("groups"), selectAll: $("select-all"), clearSelection: $("clear-selection"),
    selectionSummary: $("selection-summary"), settingsStatus: $("settings-status"), contextDetails: $("context"),
    context: [...document.querySelectorAll("[data-skz-context]")], description: $("profile-description"), retryProfile: $("retry-profile"),
    status: $("status"), errors: $("errors"), warnings: $("warnings"), preview: $("preview"), search: $("search"), rows: $("rows"), searchEmpty: $("search-empty"),
    result: $("result"), resultName: $("result-name"), resultSummary: $("result-summary"), downloads: $("downloads"),
  };
  const state = { file: null, plan: null, controls: new Map(), previewVersion: 0, previewController: null,
    revision: 0, generating: false, profileReady: false, profileLoading: false, profileError: "", operationError: "", mode: "st",
    kindContexts: {}, fbdContexts: {}, contextDirty: new Set() };

  function notice(element, messages) {
    const values = (Array.isArray(messages) ? messages : [messages]).filter(Boolean).map((value) => String(value.message || value));
    element.replaceChildren();
    for (const value of new Set(values)) {
      const paragraph = document.createElement("p");
      paragraph.textContent = value;
      element.append(paragraph);
    }
    element.hidden = values.length === 0;
  }

  function showError(message = "") {
    state.operationError = message;
    notice(ui.errors, [state.profileError, state.operationError]);
  }

  async function requestJSON(url, options = {}) {
    let response;
    try { response = await fetch(url, { cache: "no-store", credentials: "same-origin", ...options }); }
    catch (error) {
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

  function integer(value) {
    const text = String(value).trim();
    const number = /^\d+$/.test(text) ? Number(text) : NaN;
    return Number.isSafeInteger(number) && number >= 0 && number <= MAX_ID ? number : null;
  }

  function selected() { return [...state.controls.values()].filter((control) => control.selected.checked); }

  function activeModules(control) { return control.modules.slice(0, control.moduleCount).map((entry) => entry.module); }

  function pouCount(control, sourceOnly = false) {
    return state.mode === "fbd" && control.group.kind === "AI"
      ? sourceOnly ? control.group.modules.length : control.moduleCount : 1;
  }

  function modulePOU(group, module) {
    if (state.mode === "st") return `${group.pouName}_channels`;
    return group.kind === "AI" ? `AI_${module.name.replace(/-/g, "_")}` : group.pouName;
  }

  function stats(controls, sourceOnly = false) {
    const scs = new Set();
    let pous = 0, modules = 0, channels = 0, reserves = 0;
    for (const control of controls) {
      const { group } = control;
      scs.add(group.scs.toUpperCase());
      pous += pouCount(control, sourceOnly);
      const effective = sourceOnly ? group.modules : activeModules(control);
      modules += effective.length;
      for (const module of effective) {
        channels += module.extra ? module.capacity : module.channels.length;
        reserves += module.extra ? module.capacity : module.channels.filter((channel) => channel.reserve).length;
      }
    }
    return { scs: scs.size, groups: controls.length, pous, modules, channels, reserves };
  }

  function describe(value) { return `${value.scs} ПЛК · ${value.pous} POU · ${value.modules} модулей · ${value.channels} каналов · ${value.reserves} резервов`; }

  function maximumModuleSuffix(group) {
    return Math.max(...group.modules.map((module) => Number(module.name.match(/[-_](\d+)$/)[1])));
  }

  function moduleCountIssue(control) {
    const count = integer(control.count.value);
    const { group } = control;
    if (count === null || count < group.modules.length || count > MAX_MODULES) {
      return { message: `${group.scs} / ${group.pouName}: количество модулей должно быть от ${group.modules.length} до ${MAX_MODULES}. Модули из XLSX удалять нельзя.`, input: control.count };
    }
    const last = maximumModuleSuffix(group) + count - group.modules.length;
    if (last > MAX_MODULE_SUFFIX) {
      return { message: `${group.scs} / ${group.pouName}: номер добавленного модуля ${group.prefix}-${last} превышает ${MAX_MODULE_SUFFIX}. Уменьшите количество модулей.`, input: control.count };
    }
    return null;
  }

  function moduleValidation(controls) {
    for (const control of controls) {
      const issue = moduleCountIssue(control);
      if (issue) return issue;
    }
    const moduleKey = (scs, name) => `${scs.toUpperCase()}:${name.replace(/_/g, "-").toUpperCase()}`;
    const sourceModules = new Map();
    for (const group of state.plan?.groups || []) {
      for (const module of group.modules) sourceModules.set(moduleKey(group.scs, module.name), group);
    }
    const addedModules = new Map();
    for (const control of controls) {
      const { group } = control;
      const first = maximumModuleSuffix(group) + 1;
      const added = integer(control.count.value) - group.modules.length;
      for (let suffix = first; suffix < first + added; suffix++) {
        const name = `${group.prefix}-${String(suffix).padStart(2, "0")}`;
        const key = moduleKey(group.scs, name);
        const sourceOwner = sourceModules.get(key);
        if (sourceOwner) return { message: `${group.scs}: добавленный модуль ${name} уже есть в XLSX (${sourceOwner.pouName}). Уменьшите количество модулей.`, input: control.count };
        const addedOwner = addedModules.get(key);
        if (addedOwner) return { message: `${group.scs}: добавленный модуль ${name} повторяется в выбранных группах ${addedOwner.pouName} и ${group.pouName}. Измените количество модулей или выбор групп.`, input: control.count };
        addedModules.set(key, group);
      }
    }
    const totals = stats(controls);
    if (totals.modules > MAX_MODULES || totals.channels > MAX_CHANNELS) {
      return { message: `Выбранный состав превышает лимит ${MAX_MODULES} модулей / ${MAX_CHANNELS} каналов: ${totals.modules} модулей, ${totals.channels} каналов. Уменьшите количество модулей или выберите меньше групп.` };
    }
    if (totals.pous > MAX_POUS) return { message: `За одну генерацию допускается до ${MAX_POUS} POU; в выбранном составе — ${totals.pous}. Для AI FBD каждый модуль создаёт отдельную POU. Уменьшите количество модулей или выберите меньше групп.` };
    return null;
  }

  function validation() {
    if (!state.plan) return { message: "Загрузите карту для выбора групп модулей." };
    const controls = selected();
    if (!controls.length) return { message: "Выберите хотя бы одну группу модулей." };
    const moduleIssue = moduleValidation(controls);
    if (moduleIssue) return moduleIssue;
    if (!state.profileReady) return { message: state.profileError || "Дождитесь загрузки профиля PLC 850." };
    const filename = ui.filename.value.trim().replace(/\.xml$/i, "");
    if (!filename || filename.length > 180 || /[<>:"/\\|?*\u0000-\u001f]/.test(filename) || /[. ]$/.test(filename)) {
      return { message: "Укажите допустимое базовое имя файла до 180 символов.", input: ui.filename };
    }
    for (const input of ui.context) {
      const value = input.value.trim();
      const key = input.dataset.skzContext;
      if (key === "controllerTypeName" && value !== "TENIX-CPU850") return { message: "Для СКЗ требуется тип контроллера TENIX-CPU850.", input, context: true };
      if ((key !== "project" && !value) || value.length > 200 || /[\u0000-\u001f\ufffe\uffff]/.test(value)
        || (["version", "controllerId", "resourceId", "groupId", "pouNumber"].includes(key) && integer(value) === null)) {
        return { message: "Проверьте параметры SCADA: версия, ID контроллера, ID ресурса, GroupID и POUNum должны быть целыми числами от 0 до 2147483647.", input, context: true };
      }
    }
    const pouNumber = ui.context.find((input) => input.dataset.skzContext === "pouNumber");
    const pousPerPLC = new Map();
    for (const control of controls) {
      const key = control.group.scs.toUpperCase();
      pousPerPLC.set(key, (pousPerPLC.get(key) || 0) + pouCount(control));
    }
    if (integer(pouNumber.value) + Math.max(...pousPerPLC.values()) - 1 > MAX_ID) {
      return { message: "Диапазон POUNum для выбранных POU выходит за 2147483647. Уменьшите первый POUNum.", input: pouNumber, context: true };
    }
    if (state.mode === "st") {
      const seen = new Set();
      for (const control of controls) {
        for (let i = 0; i < control.ids.length; i++) {
          const input = control.ids[i];
          const id = integer(input.value);
          const module = control.modules[i].module;
          if (id === null) return { message: `${control.group.scs} / ${module.name}: укажите реальный ModuleID от 0 до 2147483647.`, input };
          const key = `${control.group.scs.toLowerCase()}:${id}`;
          if (seen.has(key)) return { message: `${control.group.scs}: ModuleID ${id} повторяется в выбранных модулях. Укажите разные физические ID.`, input };
          seen.add(key);
        }
      }
    }
    return null;
  }

  function refresh() {
    const controls = selected();
    const issue = validation();
    ui.generate.disabled = state.generating || Boolean(issue);
    ui.generate.setAttribute("aria-busy", String(state.generating));
    for (const input of [ui.file, ui.filename, ui.mode]) input.disabled = state.generating;
    for (const input of ui.context) input.disabled = state.generating || !state.profileReady;
    ui.selectAll.disabled = state.generating || state.controls.size === 0 || controls.length === state.controls.size;
    ui.clearSelection.disabled = state.generating || controls.length === 0;
    ui.retryProfile.disabled = state.profileLoading || state.generating;
    ui.retryProfile.hidden = !state.profileError;
    const countIssue = moduleValidation(controls);
    ui.selectionSummary.textContent = controls.length
      ? countIssue?.input ? `Выбрано ${controls.length} групп · проверьте количество модулей` : `Выбрано: ${describe(stats(controls))}`
      : "Группы модулей не выбраны";
    ui.settingsStatus.textContent = issue?.message || (state.mode === "st" ? "Физические ID заполнены. Можно создать ST." : "Можно создать FBD: фрагменты AD3_v2 с привязкой к тегам AI и подключением DO к D32V.");
    ui.modeNote.textContent = state.mode === "st"
      ? "Укажите реальные ModuleID физических модулей. Обозначение A1-00 в карте не задаёт ModuleID. Для DO источником физического выхода служит D32V._NN."
      : "FBD создаёт полный фрагмент AD3_v2 с привязкой к тегам AI из карты. Каждый модуль AI получает отдельную POU, например AI_A1_00 и AI_A1_01. BOOL-теги DO из XLSX соединяются со входами iNN объекта D32V. Привязка к физическим каналам выполняется в ST.";
    for (const control of state.controls.values()) {
      control.selected.disabled = state.generating;
      control.count.disabled = state.generating || !control.selected.checked;
      control.idsPanel.hidden = state.mode !== "st";
      for (const input of control.ids) input.disabled = state.generating || state.mode !== "st" || !control.selected.checked;
      control.metadata.textContent = `${control.group.kind === "AI" ? "AI16H · AD3_v2" : "DO32P · D32V"} · ${control.moduleCount} модулей (из XLSX: ${control.group.modules.length}) · ${pouCount(control)} POU`;
    }
  }

  function changed(render = false) {
    state.revision++;
    ui.result.hidden = true;
    showError();
    refresh();
    if (render) renderRows();
  }

  function applyKindDefaults() {
    const kinds = new Set((state.plan?.groups || []).map((group) => group.kind));
    if (kinds.size !== 1) return;
    const kind = [...kinds][0];
    const defaults = state.mode === "fbd" ? state.fbdContexts[kind] || state.kindContexts[kind] : state.kindContexts[kind];
    if (!defaults) return;
    for (const input of ui.context) {
      const key = input.dataset.skzContext;
      if (["groupId", "pouNumber"].includes(key) && !state.contextDirty.has(key) && defaults[key] != null) input.value = String(defaults[key]);
    }
  }

  async function loadProfile() {
    if (state.profileLoading || state.generating) return;
    state.profileLoading = true;
    refresh();
    try {
      const profile = await requestJSON("/api/skz/profile");
      if (!profile?.context || ui.context.some((input) => !Object.hasOwn(profile.context, input.dataset.skzContext))) throw new Error("В ответе отсутствуют параметры профиля PLC 850.");
      for (const input of ui.context) {
        if (!state.contextDirty.has(input.dataset.skzContext)) input.value = String(profile.context[input.dataset.skzContext] ?? "");
      }
      state.kindContexts = profile.contexts || {};
      state.fbdContexts = profile.fbdContexts || {};
      applyKindDefaults();
      ui.description.textContent = profile.description || "Параметры из экспортированного примера PLC 850. Проверьте значения для целевого проекта.";
      state.profileReady = true;
      state.profileError = "";
    } catch (error) {
      state.profileReady = false;
      state.profileError = `Не удалось загрузить профиль PLC 850: ${error.message}`;
      ui.description.textContent = state.profileError;
    } finally {
      state.profileLoading = false;
      notice(ui.errors, [state.profileError, state.operationError]);
      refresh();
    }
  }

  function renderGroups() {
    const fragment = document.createDocumentFragment();
    for (const group of state.plan.groups) {
      const section = document.createElement("section");
      section.className = "temporary-st-plc";
      const heading = document.createElement("label");
      heading.className = "temporary-st-plc-heading";
      const checkbox = document.createElement("input");
      checkbox.type = "checkbox";
      checkbox.checked = false;
      checkbox.setAttribute("aria-label", `Генерировать ${group.pouName}, ${group.scs}`);
      const title = document.createElement("strong");
      title.textContent = `${group.pouName} · ${group.scs}`;
      heading.append(checkbox, title);
      const metadata = document.createElement("p");
      metadata.className = "temporary-preview-note";
      metadata.textContent = `${group.kind === "AI" ? "AI16H · AD3_v2" : "DO32P · D32V"} · ${group.modules.length} модулей`;
      const idsPanel = document.createElement("div");
      const caption = document.createElement("p");
      caption.className = "temporary-preview-note";
      caption.textContent = "Реальные физические ModuleID для ST";
      const list = document.createElement("div");
      list.className = "temporary-st-module-list";
      const countField = document.createElement("label");
      countField.className = "temporary-field";
      const countCaption = document.createElement("span");
      countCaption.textContent = "Количество модулей · ST и FBD";
      const count = document.createElement("input");
      count.type = "number";
      count.min = String(group.modules.length);
      count.max = String(MAX_MODULES);
      count.step = "1";
      count.value = String(group.modules.length);
      count.required = true;
      count.setAttribute("aria-label", `Количество модулей ${group.pouName}, ${group.scs}`);
      countField.append(countCaption, count);
      idsPanel.append(caption, list);
      section.append(heading, metadata, countField, idsPanel);
      fragment.append(section);
      const control = { group, selected: checkbox, metadata, count, moduleCount: group.modules.length, modules: [], idsPanel, list, ids: [] };
      for (const module of group.modules) control.modules.push(moduleEntry(group, module));
      state.controls.set(group.key, control);
      renderModuleIDs(control);
      const resize = () => { if (state.generating) return; resizeModules(control); changed(true); };
      count.addEventListener("input", resize);
      count.addEventListener("change", resize);
      checkbox.addEventListener("change", () => changed(true));
    }
    ui.groups.replaceChildren(fragment);
    const source = stats([...state.controls.values()], true);
    ui.summary.textContent = `В книге: ${source.scs} ПЛК · ${source.groups} групп · ${source.modules} модулей · ${source.channels} каналов · ${source.reserves} резервов`;
  }

  function moduleEntry(group, module) {
    const label = document.createElement("label");
    label.className = "temporary-field";
    const text = document.createElement("span");
    text.textContent = module.name + (module.extra ? " · резерв" : "");
    const input = document.createElement("input");
    input.type = "number";
    input.min = "0";
    input.max = String(MAX_ID);
    input.step = "1";
    input.value = "";
    input.placeholder = "Введите ModuleID";
    input.required = true;
    input.setAttribute("aria-label", `ModuleID ${module.name}, ${group.scs}, ${group.pouName}`);
    input.addEventListener("input", () => changed());
    label.append(text, input);
    return { module, input, label };
  }

  function renderModuleIDs(control) {
    const entries = control.modules.slice(0, control.moduleCount);
    control.ids = entries.map((entry) => entry.input);
    control.list.replaceChildren(...entries.map((entry) => entry.label));
  }

  function resizeModules(control) {
    const count = integer(control.count.value);
    const { group } = control;
    if (moduleCountIssue(control) || count === control.moduleCount) return;
    const maximumSuffix = maximumModuleSuffix(group);
    while (control.modules.length < count) {
      const suffix = maximumSuffix + 1 + control.modules.length - group.modules.length;
      const module = { name: `${group.prefix}-${String(suffix).padStart(2, "0")}`, extra: true,
        capacity: group.kind === "AI" ? 16 : 32, type: group.kind === "AI" ? "AI16H" : "DO32P",
        objectType: group.kind === "AI" ? "AD3_v2" : "D32V", channels: [] };
      control.modules.push(moduleEntry(group, module));
    }
    control.moduleCount = count;
    renderModuleIDs(control);
  }

  function moduleChannels(group, module) {
    if (!module.extra) return module.channels;
    const tag = `_${group.scs}_${module.name.replace(/-/g, "_")}`;
    return Array.from({ length: module.capacity }, (_, channel) => ({ channel, reserve: true, sourceRow: 0,
      tag: group.kind === "AI" ? `${tag}_${channel}` : tag,
      member: group.kind === "DO" ? `_${String(channel).padStart(2, "0")}` : "" }));
  }

  function renderRows() {
    const query = ui.search.value.trim().toLocaleLowerCase("ru-RU");
    const fragment = document.createDocumentFragment();
    let count = 0;
    const controls = selected();
    const issue = moduleValidation(controls);
    if (issue) {
      ui.rows.replaceChildren();
      ui.searchEmpty.hidden = false;
      ui.searchEmpty.textContent = issue.message;
      return;
    }
    for (const control of controls) {
      const { group } = control;
      for (const module of activeModules(control)) {
        const pouName = modulePOU(group, module);
        let first = true;
        for (const channel of moduleChannels(group, module)) {
          const sourceTag = channel.member ? `${channel.tag}.${channel.member}` : channel.tag;
          const objectType = module.objectType || (group.kind === "AI" ? "AD3_v2" : "D32V");
          const designation = channel.reserve ? group.kind === "DO" ? "Резерв D32V" : "Резерв" : "Сигнал";
          if (query && ![group.scs, group.pouName, pouName, module.name, objectType, channel.channel, sourceTag, channel.sourceRow, designation].join(" ").toLocaleLowerCase("ru-RU").includes(query)) continue;
          const row = document.createElement("tr");
          if (first) row.className = "temporary-module-start";
          if (channel.reserve) row.className += " temporary-reserve-row";
          for (const value of [first ? `${pouName} / ${group.scs}` : "", first ? module.name : "", channel.channel, sourceTag, objectType, channel.sourceRow || "—", designation]) {
            const cell = document.createElement("td");
            cell.textContent = value == null ? "—" : String(value);
            row.append(cell);
          }
          fragment.append(row);
          first = false;
          count++;
        }
      }
    }
    ui.rows.replaceChildren(fragment);
    ui.searchEmpty.hidden = count > 0;
    ui.searchEmpty.textContent = selected().length ? "Каналы по этому запросу не найдены." : "Выберите группы модулей для просмотра каналов.";
  }

  function validatePlan(plan) {
    if (!plan || !Array.isArray(plan.groups) || !plan.groups.length) throw new Error("В книге не найдены группы AI / DO для СКЗ.");
    const keys = new Set();
    for (const group of plan.groups) {
      if (!group || typeof group.key !== "string" || !group.key || keys.has(group.key) || typeof group.scs !== "string" || !group.scs
        || typeof group.prefix !== "string" || !/^A\d{1,6}$/.test(group.prefix)
        || !["AI", "DO"].includes(group.kind) || typeof group.pouName !== "string" || !group.pouName || !Array.isArray(group.modules) || !group.modules.length
        || group.modules.some((module) => !module || typeof module.name !== "string" || !/^A\d{1,6}[-_]\d+$/.test(module.name) || !Array.isArray(module.channels)
          || module.channels.some((channel) => !channel || !Number.isSafeInteger(channel.channel) || channel.channel < 0 || typeof channel.tag !== "string"))) {
        throw new Error("Сервер вернул некорректную разметку СКЗ.");
      }
      keys.add(group.key);
    }
  }

  async function previewFile() {
    if (state.generating) return;
    const version = ++state.previewVersion;
    state.revision++;
    state.previewController?.abort();
    state.previewController = null;
    state.file = ui.file.files[0] || null;
    state.plan = null;
    state.controls.clear();
    ui.groups.replaceChildren();
    ui.rows.replaceChildren();
    ui.search.value = "";
    ui.selection.hidden = true;
    ui.preview.hidden = true;
    ui.preview.setAttribute("aria-busy", "false");
    ui.result.hidden = true;
    showError();
    notice(ui.warnings, []);
    refresh();
    const file = state.file;
    if (!file) { ui.status.textContent = "Выберите карту AI или DO (.xlsx) для предварительного просмотра."; return; }
    if (!/\.xlsx$/i.test(file.name) || file.size === 0 || file.size > MAX_FILE_BYTES) {
      ui.status.textContent = "Файл не загружен.";
      showError(!/\.xlsx$/i.test(file.name) ? "Выберите карту AI или DO в формате .xlsx."
        : file.size === 0 ? "Выбранный файл пуст." : "Размер книги превышает 16 МБ.");
      return;
    }
    const controller = new AbortController();
    state.previewController = controller;
    const body = new FormData();
    body.append("file", file);
    ui.status.textContent = `Читаю ${file.name}…`;
    ui.preview.setAttribute("aria-busy", "true");
    try {
      const plan = await requestJSON("/api/skz/preview", { method: "POST", body, signal: controller.signal });
      if (version !== state.previewVersion || file !== state.file) return;
      validatePlan(plan);
      state.plan = plan;
      applyKindDefaults();
      renderGroups();
      renderRows();
      ui.selection.hidden = false;
      ui.preview.hidden = false;
      notice(ui.warnings, plan.warnings || []);
      ui.status.textContent = `${file.name}: карта прочитана. Выберите нужные группы модулей.`;
    } catch (error) {
      if (version !== state.previewVersion || file !== state.file || error?.name === "AbortError") return;
      state.plan = null;
      ui.status.textContent = "Не удалось прочитать карту. Проверьте файл и загрузите его снова.";
      showError(error.message);
    } finally {
      if (version === state.previewVersion && file === state.file) {
        state.previewController = null;
        ui.preview.setAttribute("aria-busy", "false");
        refresh();
      }
    }
  }

  function renderDownloads(files, controls) {
    if (!Array.isArray(files) || !files.length) throw new Error("Сервер не вернул XML-файлы для скачивания.");
    const fragment = document.createDocumentFragment();
    for (const file of files) {
      if (!file || typeof file.fileName !== "string" || !/\.xml$/i.test(file.fileName) || /[/\\]/.test(file.fileName)
        || typeof file.url !== "string" || typeof file.fcs !== "string" || !file.fcs) throw new Error("Сервер вернул некорректное описание XML-файла.");
      const url = new URL(file.url, window.location.href);
      const prefix = "/api/output/";
      if (url.origin !== window.location.origin || url.username || url.password || url.search || url.hash || !url.pathname.startsWith(prefix)
        || decodeURIComponent(url.pathname.slice(prefix.length)) !== file.fileName) throw new Error("Сервер вернул недопустимый адрес файла.");
      const row = document.createElement("li");
      row.className = "temporary-download-row";
      const details = document.createElement("div");
      const heading = document.createElement("strong");
      heading.textContent = file.fcs;
      const description = document.createElement("p");
      const filePOUs = stats(controls.filter((control) => control.group.scs.toUpperCase() === file.fcs.toUpperCase())).pous;
      description.textContent = `${file.fileName} · ${file.summary?.pouCount ?? filePOUs} POU`;
      details.append(heading, description);
      const link = document.createElement("a");
      link.className = "button button-secondary button-small";
      link.href = url.href;
      link.download = file.fileName;
      link.textContent = "Скачать XML";
      link.setAttribute("aria-label", `Скачать XML СКЗ для ${file.fcs}`);
      row.append(details, link);
      fragment.append(row);
    }
    ui.downloads.replaceChildren(fragment);
  }

  async function generate(event) {
    event.preventDefault();
    if (state.generating) return;
    const issue = validation();
    if (issue) {
      if (issue.context) ui.contextDetails.open = true;
      issue.input?.focus();
      showError(issue.message);
      return;
    }
    const file = state.file;
    const revision = state.revision;
    const controls = selected();
    const totals = stats(controls);
    const config = { kind: state.mode, pous: controls.map((control) => ({ groupKey: control.group.key, moduleCount: control.moduleCount, moduleIds: state.mode === "st" ? control.ids.map((input) => integer(input.value)) : [] })) };
    const context = Object.fromEntries(ui.context.map((input) => [input.dataset.skzContext, input.value.trim()]));
    const body = new FormData();
    body.append("file", file);
    body.append("fileName", ui.filename.value.trim().replace(/\.xml$/i, ""));
    body.append("context", JSON.stringify(context));
    body.append("config", JSON.stringify(config));
    state.generating = true;
    ui.result.hidden = true;
    showError();
    notice(ui.warnings, state.plan.warnings || []);
    ui.status.textContent = `Создаю XML ${state.mode.toUpperCase()} для СКЗ по карте ${file.name}…`;
    refresh();
    try {
      const result = await requestJSON("/api/skz/generate", { method: "POST", body });
      if (revision !== state.revision || file !== state.file) return;
      renderDownloads(result?.files, controls);
      const summary = result.summary || {};
      ui.resultName.textContent = `Файлов создано: ${result.files.length} · по одному на ПЛК`;
      ui.resultSummary.textContent = `${summary.pouCount ?? totals.pous} POU ${config.kind.toUpperCase()} · ${summary.signalCount ?? totals.channels} каналов`
        + (config.kind === "st" && summary.assignmentCount != null ? ` · ${summary.assignmentCount} присваиваний` : config.kind === "fbd" && summary.blocks != null ? ` · ${summary.blocks} блоков` : "");
      ui.result.hidden = false;
      notice(ui.warnings, [...(state.plan.warnings || []), ...(result.warnings || [])]);
      ui.status.textContent = "XML-файлы СКЗ сохранены в папке результатов. Скачайте файл нужного ПЛК для импорта.";
      window.dispatchEvent(new Event("schemegen:outputs-changed"));
    } catch (error) {
      if (revision !== state.revision || file !== state.file) return;
      ui.status.textContent = "Не удалось создать XML СКЗ. Проверьте параметры и повторите.";
      showError(error.message);
    } finally {
      state.generating = false;
      refresh();
    }
  }

  ui.mode.value = "st";
  ui.file.addEventListener("change", previewFile);
  ui.filename.addEventListener("input", () => changed());
  ui.mode.addEventListener("change", () => { state.mode = ui.mode.value === "fbd" ? "fbd" : "st"; applyKindDefaults(); changed(true); });
  for (const input of ui.context) input.addEventListener("input", () => { state.contextDirty.add(input.dataset.skzContext); changed(); });
  ui.selectAll.addEventListener("click", () => { if (state.generating) return; for (const control of state.controls.values()) control.selected.checked = true; changed(true); });
  ui.clearSelection.addEventListener("click", () => { if (state.generating) return; for (const control of state.controls.values()) control.selected.checked = false; changed(true); });
  ui.search.addEventListener("input", renderRows);
  ui.retryProfile.addEventListener("click", loadProfile);
  ui.form.addEventListener("submit", generate);
  refresh();
  loadProfile();
})();
