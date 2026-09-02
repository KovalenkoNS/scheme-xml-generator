(() => {
  "use strict";

  const API = Object.freeze({
    templates: "/api/templates",
    refresh: "/api/refresh",
    previewName: "/api/preview-name",
    generate: "/api/generate",
    outputs: "/api/outputs",
  });

  const NAME_PREVIEW_DELAY = 320;
  const MOBILE_CATALOG_QUERY = "(max-width: 900px)";
  const MAX_DOCUMENT_POUS = 128;
  const MAX_DOCUMENT_SIGNALS = 4096;
  const MAX_DOCUMENT_OBJECTS = 200000;
  const MAX_DOCUMENT_CARDS = 100000;
  const MAX_TRANSPORT_ID = 2147483647;
  const MAX_JSON_BYTES = 8 * 1024 * 1024;
  const PREVIEW_CONCURRENCY = 8;
  const IO_CAPACITY = Object.freeze({ AI: 16, AO: 4, DI: 32, DO: 32 });

  const elements = {
    connectionStatus: document.querySelector("#connection-status"),
    connectionStatusText: document.querySelector("#connection-status .status-text"),
    reloadAll: document.querySelector("#reload-all"),
    catalogPanel: document.querySelector("#catalog-panel"),
    catalogToggle: document.querySelector("#catalog-toggle"),
    templateTotal: document.querySelector("#template-total"),
    templateSearch: document.querySelector("#template-search"),
    clearSearch: document.querySelector("#clear-search"),
    librarySummary: document.querySelector("#library-summary"),
    catalogTarget: document.querySelector("#catalog-target"),
    catalogList: document.querySelector("#catalog-list"),
    catalogEmpty: document.querySelector("#catalog-empty"),
    projectForm: document.querySelector("#project-form"),
    projectSummary: document.querySelector("#project-summary"),
    addPou: document.querySelector("#add-pou"),
    generateButton: document.querySelector("#generate-button"),
    generateButtonLabel: document.querySelector("#generate-button .button-label"),
    fileName: document.querySelector("#file-name"),
    t11Start: document.querySelector("#t11-start"),
    cardStart: document.querySelector("#card-start"),
    validationSummary: document.querySelector("#validation-summary"),
    pouList: document.querySelector("#pou-list"),
    pouEmpty: document.querySelector("#pou-empty"),
    pouCardTemplate: document.querySelector("#pou-card-template"),
    moduleCardTemplate: document.querySelector("#module-card-template"),
    signalCardTemplate: document.querySelector("#signal-card-template"),
    formStatus: document.querySelector("#form-status"),
    projectHealth: document.querySelector("#project-health"),
    reloadOutputs: document.querySelector("#reload-outputs"),
    outputList: document.querySelector("#output-list"),
    outputEmpty: document.querySelector("#output-empty"),
    outputItemTemplate: document.querySelector("#output-item-template"),
    toastRegion: document.querySelector("#toast-region"),
  };

  const state = {
    libraries: [],
    templates: [],
    outputs: [],
    pous: [],
    activePouId: null,
    activeModuleId: null,
    expandedPouId: null,
    expandedModuleId: null,
    expandedSignalId: null,
    openLibraries: new Set(),
    templateErrors: [],
    catalogError: "",
    templatesLoading: true,
    outputsLoading: true,
    generating: false,
    lastErrors: [],
  };

  const previewTimers = new Map();
  const previewControllers = new Map();
  let nextUid = 1;

  function init() {
    bindEvents();
    if (window.matchMedia(MOBILE_CATALOG_QUERY).matches) {
      setCatalogCollapsed(true);
    }
    renderPous();
    updateProjectUi();
    Promise.allSettled([loadTemplates(), loadOutputs()]);
  }

  function bindEvents() {
    elements.reloadAll.addEventListener("click", refreshLibraries);
    elements.reloadOutputs.addEventListener("click", () => loadOutputs({ announceErrors: true }));
    elements.addPou.addEventListener("click", () => addPou({ focus: true }));
    elements.projectForm.addEventListener("submit", generateXml);

    elements.templateSearch.addEventListener("input", () => {
      elements.clearSearch.hidden = elements.templateSearch.value.length === 0;
      renderCatalog();
    });
    elements.clearSearch.addEventListener("click", () => {
      elements.templateSearch.value = "";
      elements.clearSearch.hidden = true;
      renderCatalog();
      elements.templateSearch.focus();
    });
    elements.catalogToggle.addEventListener("click", () => {
      setCatalogCollapsed(!elements.catalogPanel.classList.contains("is-collapsed"));
    });
    const mobileCatalog = window.matchMedia(MOBILE_CATALOG_QUERY);
    const handleMobileCatalog = (event) => {
      if (event.matches) setCatalogCollapsed(true);
    };
    if (typeof mobileCatalog.addEventListener === "function") mobileCatalog.addEventListener("change", handleMobileCatalog);
    elements.catalogList.addEventListener("click", handleCatalogClick);

    elements.pouList.addEventListener("click", handleEditorClick);
    elements.pouList.addEventListener("input", handleEditorInput);
    elements.pouList.addEventListener("change", handleEditorChange);
    elements.pouEmpty.addEventListener("click", (event) => {
      if (event.target.closest('[data-action="add-pou"]')) addPou({ focus: true });
    });

    for (const input of [elements.fileName, elements.t11Start, elements.cardStart]) {
      input.addEventListener("input", () => {
        clearValidationUi();
        updateProjectUi();
      });
    }
    elements.fileName.addEventListener("blur", () => {
      elements.fileName.value = stripXmlExtension(elements.fileName.value.trim());
    });
  }

  async function apiRequest(url, options = {}) {
    const request = {
      cache: "no-store",
      credentials: "same-origin",
      ...options,
      headers: {
        Accept: "application/json",
        ...(options.body ? { "Content-Type": "application/json" } : {}),
        ...(options.headers || {}),
      },
    };

    let response;
    try {
      response = await fetch(url, request);
    } catch (error) {
      if (error?.name === "AbortError") throw error;
      throw new Error("Сервер приложения недоступен.");
    }

    const contentType = response.headers.get("content-type") || "";
    let payload = null;
    if (response.status !== 204) {
      try {
        payload = contentType.includes("application/json") ? await response.json() : await response.text();
      } catch {
        payload = null;
      }
    }

    if (!response.ok) {
      const message = payload && typeof payload === "object"
        ? payload.error || payload.message || payload.detail
        : payload;
      throw new Error(String(message || `Ошибка сервера: HTTP ${response.status}`));
    }
    return payload || {};
  }

  async function loadTemplates({ announceErrors = true } = {}) {
    state.templatesLoading = true;
    state.catalogError = "";
    renderCatalog();
    setConnectionState("checking", "Читаю библиотеки…");

    try {
      const payload = await apiRequest(API.templates);
      state.libraries = Array.isArray(payload.libraries) ? payload.libraries : [];
      state.templates = Array.isArray(payload.templates)
        ? payload.templates.map(normalizeTemplate).filter((item) => item.key)
        : [];
      state.templateErrors = Array.isArray(payload.errors) ? payload.errors : [];
      for (const pou of state.pous) reflowAutomaticSignals(pou);

      const libraryKeys = new Set(state.templates.map((item) => item.libraryFile || "Без имени"));
      for (const key of libraryKeys) state.openLibraries.add(key);

      setConnectionState("online", "Сервер подключён");
      if (announceErrors && state.templateErrors.length > 0) {
        showToast(formatBackendError(state.templateErrors[0]), "warning", 8000);
      }
    } catch (error) {
      state.libraries = [];
      state.templates = [];
      state.templateErrors = [];
      state.catalogError = errorMessage(error);
      setConnectionState("offline", "Нет связи с сервером");
      if (announceErrors) showToast(state.catalogError, "error", 8000);
    } finally {
      state.templatesLoading = false;
      renderCatalog();
      renderPous();
      updateProjectUi();
    }
  }

  function normalizeTemplate(raw) {
    const preview = raw && typeof raw.preview === "object" && raw.preview ? raw.preview : {};
    const hasLayout = Boolean(raw && typeof raw.layout === "object" && raw.layout);
    const layout = hasLayout ? raw.layout : {};
    const width = positiveNumber(raw?.width, positiveNumber(preview.width, 0));
    const height = positiveNumber(raw?.height, positiveNumber(preview.height, 400));
    return {
      ...raw,
      key: safeText(raw?.key),
      libraryFile: safeText(raw?.libraryFile, "Без имени"),
      ownerName: safeText(raw?.ownerName),
      id: safeText(raw?.id),
      name: safeText(raw?.name, `Шаблон ${safeText(raw?.id)}`),
      description: safeText(raw?.description, "Без описания"),
      width,
      height,
      layout: {
        minX: Math.trunc(finiteNumber(layout.minX, 0)),
        minY: Math.trunc(finiteNumber(layout.minY, 0)),
        maxX: Math.max(0, Math.trunc(finiteNumber(layout.maxX, hasLayout ? 0 : width))),
        maxY: Math.max(0, Math.trunc(finiteNumber(layout.maxY, hasLayout ? 0 : height))),
      },
      primitiveCount: nonNegativeInteger(raw?.primitiveCount),
      blockCount: nonNegativeInteger(raw?.blockCount),
      linkCount: nonNegativeInteger(raw?.linkCount),
      cardCount: nonNegativeInteger(raw?.cardCount),
      supported: raw?.supported !== false,
      warnings: Array.isArray(raw?.warnings) ? raw.warnings.map(String) : [],
      ioCapabilities: Array.isArray(raw?.ioCapabilities)
        ? raw.ioCapabilities.map((value) => safeText(value).toUpperCase()).filter((value) => IO_CAPACITY[value])
        : [],
    };
  }

  function renderCatalog() {
    elements.catalogList.setAttribute("aria-busy", String(state.templatesLoading));
    elements.templateTotal.textContent = state.templatesLoading ? "—" : String(state.templates.length);
    renderLibrarySummary();

    if (state.templatesLoading) {
      elements.catalogList.hidden = false;
      elements.catalogEmpty.hidden = true;
      elements.catalogList.replaceChildren(createSkeleton(), createSkeleton());
      return;
    }

    if (state.catalogError) {
      const message = document.createElement("div");
      message.className = "catalog-message error-message";
      const title = document.createElement("strong");
      title.textContent = "Не удалось загрузить каталог";
      const detail = document.createElement("span");
      detail.textContent = state.catalogError;
      message.append(title, detail);
      elements.catalogList.replaceChildren(message);
      elements.catalogList.hidden = false;
      elements.catalogEmpty.hidden = true;
      return;
    }

    const query = elements.templateSearch.value.trim().toLocaleLowerCase("ru-RU");
    const filtered = state.templates.filter((template) => {
      if (!query) return true;
      return [template.name, template.description, template.libraryFile, template.ownerName, template.id]
        .some((value) => safeText(value).toLocaleLowerCase("ru-RU").includes(query));
    });

    const groups = new Map();
    for (const template of filtered) {
      const key = template.libraryFile || "Без имени";
      if (!groups.has(key)) groups.set(key, []);
      groups.get(key).push(template);
    }

    const fragment = document.createDocumentFragment();
    for (const [libraryFile, templates] of groups) {
      fragment.append(createLibraryGroup(libraryFile, templates, Boolean(query)));
    }

    elements.catalogList.replaceChildren(fragment);
    elements.catalogList.hidden = filtered.length === 0;
    elements.catalogEmpty.hidden = filtered.length !== 0;
  }

  function createSkeleton() {
    const node = document.createElement("div");
    node.className = "catalog-skeleton";
    node.setAttribute("aria-hidden", "true");
    return node;
  }

  function createLibraryGroup(libraryFile, templates, forceOpen) {
    const section = document.createElement("section");
    section.className = "library-group";
    section.dataset.libraryKey = libraryFile;

    const isOpen = forceOpen || state.openLibraries.has(libraryFile);
    const header = document.createElement("button");
    header.type = "button";
    header.className = "library-toggle";
    header.dataset.libraryKey = libraryFile;
    header.setAttribute("aria-expanded", String(isOpen));

    const chevron = document.createElement("span");
    chevron.className = "library-chevron";
    chevron.textContent = "⌄";
    chevron.setAttribute("aria-hidden", "true");
    const copy = document.createElement("span");
    copy.className = "library-copy";
    const name = document.createElement("strong");
    name.textContent = libraryFile;
    const library = findLibrary(libraryFile);
    const allLibraryTemplates = state.templates.filter((item) => item.libraryFile === libraryFile);
    const totalCount = nonNegativeInteger(library?.templateCount) || allLibraryTemplates.length;
    const supportedCount = library?.supportedTemplateCount === 0
      ? 0
      : nonNegativeInteger(library?.supportedTemplateCount) || allLibraryTemplates.filter((item) => item.supported).length;
    const meta = document.createElement("small");
    meta.textContent = [
      `${supportedCount}/${totalCount} поддерживается`,
      library?.version ? `версия ${library.version}` : "",
    ].filter(Boolean).join(" · ");
    copy.append(name, meta);
    const libraryWarnings = Array.isArray(library?.warnings) ? library.warnings.map(String).filter(Boolean) : [];
    if (libraryWarnings.length) {
      const warning = document.createElement("span");
      warning.className = "library-warning";
      warning.textContent = shortWarning(libraryWarnings[0]);
      warning.title = libraryWarnings.join("\n");
      copy.append(warning);
    }
    header.append(chevron, copy);
    section.append(header);

    const list = document.createElement("div");
    list.className = "library-template-list";
    list.hidden = !isOpen;
    for (const template of templates) list.append(createTemplateRow(template));
    section.append(list);
    return section;
  }

  function createTemplateRow(template) {
    const row = document.createElement("div");
    row.className = "template-row";
    row.classList.toggle("is-unsupported", !template.supported);
    const activeIOType = activePou()?.io.type || "";
    row.classList.toggle("is-io-mismatch", Boolean(activeIOType && !template.ioCapabilities.includes(activeIOType)));
    row.dataset.templateKey = template.key;

    const copy = document.createElement("button");
    copy.type = "button";
    copy.className = "template-row-copy";
    copy.classList.add("set-default-template");
    copy.dataset.templateKey = template.key;
    copy.disabled = !template.supported;
    copy.title = template.supported
      ? `Назначить ${template.name} общим шаблоном активной POU`
      : template.warnings[0] || "Шаблон пока не поддерживается генератором";
    const title = document.createElement("span");
    title.className = "template-row-title";
    const name = document.createElement("strong");
    name.textContent = template.name;
    const id = document.createElement("span");
    id.textContent = template.id ? `#${template.id}` : "";
    title.append(name, id);
    const meta = document.createElement("small");
    meta.textContent = template.supported
      ? `${template.blockCount} бл. · ${template.cardCount} карт. · ${formatInteger(template.width)}×${formatInteger(template.height)}`
      : "Не поддерживается";
    meta.title = [template.description, ...template.warnings].filter(Boolean).join("\n");
    copy.append(title, meta);
    if (template.ioCapabilities.length) {
      const capabilities = document.createElement("span");
      capabilities.className = "template-io-capabilities";
      for (const capability of template.ioCapabilities) {
        const badge = document.createElement("span");
        badge.textContent = capability;
        capabilities.append(badge);
      }
      copy.append(capabilities);
    }

    const add = document.createElement("button");
    add.type = "button";
    add.className = "icon-button add-template";
    add.dataset.templateKey = template.key;
    add.textContent = "+";
    add.title = `Добавить сигнал с шаблоном ${template.name}`;
    add.setAttribute("aria-label", `Добавить сигнал с шаблоном ${template.name}`);
    add.disabled = !template.supported;
    if (!template.supported) {
      add.title = template.warnings[0] || "Шаблон пока не поддерживается генератором";
    }
    row.append(copy, add);
    return row;
  }

  function renderLibrarySummary() {
    if (state.templatesLoading) {
      elements.librarySummary.textContent = "Загружаю библиотеки…";
      return;
    }
    if (state.catalogError) {
      elements.librarySummary.textContent = "Каталог недоступен";
      return;
    }
    const libraryCount = state.libraries.length
      || new Set(state.templates.map((item) => item.libraryFile || "Без имени")).size;
    const supportedCount = state.libraries.length
      ? state.libraries.reduce((sum, item) => sum + nonNegativeInteger(item?.supportedTemplateCount), 0)
      : state.templates.filter((item) => item.supported).length;
    const totalCount = state.libraries.length
      ? state.libraries.reduce((sum, item) => sum + nonNegativeInteger(item?.templateCount), 0)
      : state.templates.length;
    const errorPart = state.templateErrors.length ? ` · ошибок: ${state.templateErrors.length}` : "";
    elements.librarySummary.textContent = `${libraryCount} ${plural(libraryCount, "библиотека", "библиотеки", "библиотек")} · ${supportedCount}/${totalCount} поддерживается${errorPart}`;
  }

  function findLibrary(fileName) {
    return state.libraries.find((item) => [item?.file, item?.fileName, item?.name, item?.path]
      .some((value) => safeText(value).endsWith(fileName))) || null;
  }

  function handleCatalogClick(event) {
    if (state.generating) return;
    const toggle = event.target.closest(".library-toggle");
    if (toggle) {
      const key = toggle.dataset.libraryKey;
      if (state.openLibraries.has(key)) state.openLibraries.delete(key);
      else state.openLibraries.add(key);
      renderCatalog();
      return;
    }

    const action = event.target.closest(".add-template, .set-default-template");
    if (!action) return;
    const template = templateByKey(action.dataset.templateKey);
    if (!template) return;
    if (!template.supported) {
      showToast(template.warnings[0] || "Этот шаблон пока не поддерживается генератором.", "warning", 8000);
      return;
    }

    let pou = activePou();
    if (!pou) pou = addPou({ focus: false, render: false });
    if (!pou) return;
    if (action.classList.contains("set-default-template")) {
      setPouDefaultTemplate(pou, template.key);
      renderPous();
      updateProjectUi();
      showToast(`Общий шаблон POU «${pou.name}»: ${template.name}.`, "success");
    } else {
      let module = activeModule(pou);
      if (!module || module.signals.length >= capacityFor(pou)) module = addModule(pou.id, { render: false });
      if (!module) return;
      if (!pou.defaultTemplateKey) setPouDefaultTemplate(pou, template.key);
      const overrideKey = pou.defaultTemplateKey === template.key ? "" : template.key;
      addSignal(pou.id, module.uid, overrideKey, { focus: true });
    }
    collapseCatalogOnMobile();
  }

  function setCatalogCollapsed(collapsed) {
    elements.catalogPanel.classList.toggle("is-collapsed", collapsed);
    elements.catalogToggle.setAttribute("aria-expanded", String(!collapsed));
    elements.catalogToggle.setAttribute("aria-label", collapsed ? "Развернуть каталог" : "Свернуть каталог");
  }

  function collapseCatalogOnMobile() {
    if (window.matchMedia(MOBILE_CATALOG_QUERY).matches) setCatalogCollapsed(true);
  }

  function addPou({ focus = false, render = true } = {}) {
    if (state.generating) return null;
    if (state.pous.length >= MAX_DOCUMENT_POUS) {
      showToast(`В одном файле допускается не более ${MAX_DOCUMENT_POUS} POU.`, "warning", 7000);
      return null;
    }
    const pou = {
      id: uid("pou"),
      name: uniquePouName(`POU_${state.pous.length + 1}`),
      description: "",
      pouId: "",
      groupId: "",
      pouNumber: "",
      defaultTemplateKey: "",
      page: { width: "", height: "", dparams: "", backgroundColor: "", marginRight: "", marginBottom: "" },
      io: { type: "AI", modules: [] },
    };
    state.pous.push(pou);
    const module = createModule(pou);
    pou.io.modules.push(module);
    state.activePouId = pou.id;
    state.activeModuleId = module.uid;
    state.expandedPouId = pou.id;
    state.expandedModuleId = module.uid;
    state.expandedSignalId = null;
    clearValidationUi();
    if (render) {
      renderPous();
      updateProjectUi();
      if (focus) focusAfterRender(`[data-pou-id="${pou.id}"] .pou-name`);
    }
    return pou;
  }

  function createModule(pou) {
    const used = new Set();
    for (const existingPou of state.pous) {
      for (const module of existingPou.io?.modules || []) {
        const value = Number(module.moduleId);
        if (Number.isSafeInteger(value) && value >= 0) used.add(value);
      }
    }
    for (const value of (pou.io?.modules || [])
      .map((module) => Number(module.moduleId))
      .filter((item) => Number.isSafeInteger(item) && item >= 0)) {
      used.add(value);
    }
    let moduleId = 0;
    while (used.has(moduleId)) moduleId += 1;
    return {
      uid: uid("module"),
      moduleId: String(moduleId),
      // Physical addresses belong to a concrete module and must not be copied.
      bindingPrefix: "",
      instanceName: "",
      signals: [],
    };
  }

  function addModule(pouId, { focus = false, render = true } = {}) {
    if (state.generating) return null;
    const pou = pouById(pouId);
    if (!pou) return null;
    const module = createModule(pou);
    pou.io.modules.push(module);
    state.activePouId = pou.id;
    state.activeModuleId = module.uid;
    state.expandedPouId = pou.id;
    state.expandedModuleId = module.uid;
    state.expandedSignalId = null;
    clearValidationUi();
    if (render) {
      renderPous();
      updateProjectUi();
      if (focus) focusAfterRender(`[data-module-id="${module.uid}"] [data-module-field="moduleId"]`);
    }
    return module;
  }

  function addSignal(pouId, moduleUid, templateKey = "", { focus = false } = {}) {
    if (state.generating) return null;
    const pou = pouById(pouId);
    const module = moduleById(pou, moduleUid);
    if (!pou || !module) return null;
    if (allSignals().length >= MAX_DOCUMENT_SIGNALS) {
      showToast(`В одном файле допускается не более ${MAX_DOCUMENT_SIGNALS} сигналов.`, "warning", 7000);
      return null;
    }
    const capacity = capacityFor(pou);
    if (module.signals.length >= capacity) {
      showToast(`Модуль ${pou.io.type} уже содержит ${capacity} каналов. Добавьте следующий модуль.`, "warning", 7000);
      return null;
    }
    const signal = {
      id: uid("signal"),
      templateKey,
      objectName: "",
      nameMode: "auto",
      description: "",
      klPath: "",
      offsetX: String(defaultSignalX(effectiveTemplateKey(pou, { templateKey }))),
      offsetY: String(nextSignalY(pou, effectiveTemplateKey(pou, { templateKey }))),
      autoX: true,
      autoY: true,
      preview: { state: "idle", objectNames: [], status: "Введите имя", error: "" },
    };
    module.signals.push(signal);
    state.activePouId = pou.id;
    state.activeModuleId = module.uid;
    state.expandedPouId = pou.id;
    state.expandedModuleId = module.uid;
    state.expandedSignalId = signal.id;
    clearValidationUi();
    renderPous();
    updateProjectUi();
    if (focus) focusAfterRender(`[data-signal-id="${signal.id}"] .signal-object-name`);
    return signal;
  }

  function copyPou(pouId) {
    if (state.generating) return;
    const source = pouById(pouId);
    if (!source) return;
    if (state.pous.length >= MAX_DOCUMENT_POUS) {
      showToast(`В одном файле допускается не более ${MAX_DOCUMENT_POUS} POU.`, "warning", 7000);
      return;
    }
    if (allSignals().length + allPouSignals(source).length > MAX_DOCUMENT_SIGNALS) {
      showToast(`Копия превысит лимит ${MAX_DOCUMENT_SIGNALS} сигналов в одном файле.`, "warning", 7000);
      return;
    }

    const usedObjectNames = new Set(allSignals().map((item) => canonical(item.signal.objectName)).filter(Boolean));
    const copy = {
      id: uid("pou"),
      name: uniquePouName(copyName(source.name || "POU", state.pous.map((item) => item.name))),
      description: source.description,
      pouId: "",
      groupId: source.groupId,
      pouNumber: "",
      defaultTemplateKey: source.defaultTemplateKey,
      page: { ...source.page },
      io: { type: source.io.type, modules: [] },
    };

    for (const sourceModule of source.io.modules) {
      const module = createModule(copy);
      for (const original of sourceModule.signals) {
        const proposed = copyName(original.objectName || "OBJECT", [...usedObjectNames]);
        usedObjectNames.add(canonical(proposed));
        module.signals.push({
          ...original,
          id: uid("signal"),
          objectName: proposed,
          preview: { state: "idle", objectNames: [], status: "Требуется проверка", error: "" },
        });
      }
      copy.io.modules.push(module);
    }

    const index = state.pous.indexOf(source);
    state.pous.splice(index + 1, 0, copy);
    state.activePouId = copy.id;
    state.activeModuleId = copy.io.modules[0]?.uid || null;
    state.expandedPouId = copy.id;
    state.expandedModuleId = state.activeModuleId;
    state.expandedSignalId = copy.io.modules[0]?.signals[0]?.id || null;
    clearValidationUi();
    renderPous();
    updateProjectUi();
    focusAfterRender(`[data-pou-id="${copy.id}"] .pou-name`, true);
    showToast(`Создана копия POU «${copy.name}». Проверьте имена и идентификаторы.`, "info", 7000);
  }

  function copyModule(pouId, moduleUid) {
    if (state.generating) return;
    const pou = pouById(pouId);
    const source = moduleById(pou, moduleUid);
    if (!pou || !source) return;
    if (allSignals().length + source.signals.length > MAX_DOCUMENT_SIGNALS) {
      showToast(`Копия превысит лимит ${MAX_DOCUMENT_SIGNALS} сигналов.`, "warning", 7000);
      return;
    }
    const module = createModule(pou);
    for (const original of source.signals) {
      module.signals.push({
        ...original,
        id: uid("signal"),
        objectName: uniqueObjectName(copyName(original.objectName || "OBJECT", allSignals().map((item) => item.signal.objectName))),
        preview: { state: "idle", objectNames: [], status: "Требуется проверка", error: "" },
      });
    }
    const index = pou.io.modules.indexOf(source);
    pou.io.modules.splice(index + 1, 0, module);
    state.activeModuleId = module.uid;
    state.expandedModuleId = module.uid;
    state.expandedSignalId = module.signals[0]?.id || null;
    clearValidationUi();
    renderPous();
    updateProjectUi();
  }

  function copySignal(pouId, moduleUid, signalId) {
    if (state.generating) return;
    const pou = pouById(pouId);
    const module = moduleById(pou, moduleUid);
    const source = signalById(module, signalId);
    if (!pou || !module || !source) return;
    if (allSignals().length >= MAX_DOCUMENT_SIGNALS) {
      showToast(`В одном файле допускается не более ${MAX_DOCUMENT_SIGNALS} сигналов.`, "warning", 7000);
      return;
    }
    if (module.signals.length >= capacityFor(pou)) {
      showToast("В модуле нет свободного канала.", "warning");
      return;
    }
    const copy = {
      ...source,
      id: uid("signal"),
      objectName: uniqueObjectName(copyName(source.objectName || "OBJECT", allSignals().map((item) => item.signal.objectName))),
      offsetY: String(nextSignalY(pou, effectiveTemplateKey(pou, source))),
      autoY: true,
      preview: { state: "idle", objectNames: [], status: "Требуется проверка", error: "" },
    };
    const index = module.signals.indexOf(source);
    module.signals.splice(index + 1, 0, copy);
    state.activePouId = pou.id;
    state.activeModuleId = module.uid;
    state.expandedPouId = pou.id;
    state.expandedModuleId = module.uid;
    state.expandedSignalId = copy.id;
    clearValidationUi();
    renderPous();
    updateProjectUi();
    focusAfterRender(`[data-signal-id="${copy.id}"] .signal-object-name`, true);
  }

  function deletePou(pouId) {
    if (state.generating) return;
    const pou = pouById(pouId);
    if (!pou) return;
    const signalCount = allPouSignals(pou).length;
    const suffix = signalCount
      ? ` и ${signalCount} ${plural(signalCount, "сигнал", "сигнала", "сигналов")}`
      : "";
    if (!window.confirm(`Удалить POU «${pou.name || "Без имени"}»${suffix}?`)) return;
    for (const signal of allPouSignals(pou)) disposeSignalPreview(signal.id);
    state.pous = state.pous.filter((item) => item.id !== pou.id);
    state.activePouId = state.pous[0]?.id || null;
    state.activeModuleId = state.pous[0]?.io.modules[0]?.uid || null;
    state.expandedPouId = state.activePouId;
    state.expandedModuleId = state.activeModuleId;
    state.expandedSignalId = null;
    clearValidationUi();
    renderPous();
    updateProjectUi();
  }

  function deleteModule(pouId, moduleUid) {
    if (state.generating) return;
    const pou = pouById(pouId);
    const module = moduleById(pou, moduleUid);
    if (!pou || !module) return;
    const count = module.signals.length;
    if (!window.confirm(`Удалить модуль ID ${module.moduleId || "—"}${count ? ` и ${count} сигналов` : ""}?`)) return;
    for (const signal of module.signals) disposeSignalPreview(signal.id);
    pou.io.modules = pou.io.modules.filter((item) => item.uid !== module.uid);
    state.activeModuleId = pou.io.modules[0]?.uid || null;
    state.expandedModuleId = state.activeModuleId;
    state.expandedSignalId = null;
    clearValidationUi();
    renderPous();
    updateProjectUi();
  }

  function deleteSignal(pouId, moduleUid, signalId) {
    if (state.generating) return;
    const pou = pouById(pouId);
    const module = moduleById(pou, moduleUid);
    const signal = signalById(module, signalId);
    if (!pou || !module || !signal) return;
    disposeSignalPreview(signal.id);
    module.signals = module.signals.filter((item) => item.id !== signal.id);
    if (state.expandedSignalId === signal.id) state.expandedSignalId = null;
    reflowAutomaticSignals(pou);
    clearValidationUi();
    renderPous();
    updateProjectUi();
    showToast(`Сигнал «${signal.objectName || "Без имени"}» удалён.`, "info");
  }

  function nextSignalY(pou, templateKey) {
    let cursor = 100;
    const signals = allPouSignals(pou);
    if (signals.length) {
      const bottoms = signals.map((signal) => {
        const y = integerOr(signal.offsetY, 100);
        return y + templateLayout(effectiveTemplateKey(pou, signal)).maxY;
      });
      cursor = Math.max(...bottoms) + 20;
    }
    return cursor - Math.min(0, templateLayout(templateKey).minY);
  }

  function reflowAutomaticSignals(pou) {
    let cursor = 100;
    for (const signal of allPouSignals(pou)) {
      const key = effectiveTemplateKey(pou, signal);
      const layout = templateLayout(key);
      if (signal.autoX !== false) signal.offsetX = String(defaultSignalX(key));
      if (signal.autoY !== false) signal.offsetY = String(cursor - Math.min(0, layout.minY));
      const y = integerOr(signal.offsetY, cursor);
      cursor = Math.max(cursor, y + layout.maxY + 20);
    }
  }

  function setPouDefaultTemplate(pou, key) {
    pou.defaultTemplateKey = key;
    for (const signal of allPouSignals(pou)) {
      if (signal.templateKey) continue;
      signal.preview = { state: "idle", objectNames: [], status: "Требуется проверка", error: "" };
      scheduleSignalPreview(signal);
    }
    reflowAutomaticSignals(pou);
    const template = templateByKey(key);
    if (template && !template.ioCapabilities.includes(pou.io.type)) {
      showToast(`Шаблон ${template.name} не помечен как совместимый с ${pou.io.type}; сервер выполнит окончательную проверку.`, "warning", 7500);
    }
  }

  function effectiveTemplateKey(pou, signal) {
    return safeText(signal?.templateKey || pou?.defaultTemplateKey);
  }

  function capacityFor(pou) {
    return IO_CAPACITY[pou?.io?.type] || 0;
  }

  function defaultSignalX(key) {
    return 300 - Math.min(0, templateLayout(key).minX);
  }

  function templateLayout(key) {
    const template = templateByKey(key);
    const width = Math.max(1, Math.round(template?.width || 400));
    const height = Math.max(1, Math.round(template?.height || 400));
    return template?.layout || { minX: 0, minY: 0, maxX: width, maxY: height };
  }

  function renderPous() {
    const fragment = document.createDocumentFragment();
    state.pous.forEach((pou, index) => fragment.append(renderPou(pou, index)));
    elements.pouList.replaceChildren(fragment);
    elements.pouList.hidden = state.pous.length === 0;
    elements.pouEmpty.hidden = state.pous.length !== 0;
  }

  function renderPou(pou, index) {
    const node = elements.pouCardTemplate.content.firstElementChild.cloneNode(true);
    node.dataset.pouId = pou.id;
    node.classList.toggle("is-active", pou.id === state.activePouId);
    const expanded = pou.id === state.expandedPouId;
    const toggle = node.querySelector(".pou-toggle");
    const content = node.querySelector(".pou-content");
    const contentId = `pou-content-${pou.id}`;
    toggle.setAttribute("aria-expanded", String(expanded));
    toggle.setAttribute("aria-controls", contentId);
    content.id = contentId;
    content.hidden = !expanded;

    node.querySelector(".pou-index").textContent = String(index + 1);
    node.querySelector(".pou-display-name").textContent = pou.name || "POU без имени";
    const pouSignals = allPouSignals(pou);
    const templateName = templateByKey(pou.defaultTemplateKey)?.name || "шаблон не выбран";
    node.querySelector(".pou-meta").textContent = `${pou.io.type} · ${pou.io.modules.length} мод. · ${pouSignals.length} сигн. · ${templateName}${pou.pouNumber ? ` · POUNum ${pou.pouNumber}` : ""}`;
    setEntityStatus(node.querySelector(".pou-header .entity-status"), pouStatus(pou));
    if (!expanded) {
      content.replaceChildren();
      return node;
    }

    setInputValue(node, '[data-field="name"]', pou.name);
    setInputValue(node, '[data-field="description"]', pou.description);
    setInputValue(node, '[data-field="pouId"]', pou.pouId);
    setInputValue(node, '[data-field="groupId"]', pou.groupId);
    setInputValue(node, '[data-field="pouNumber"]', pou.pouNumber);
    setInputValue(node, '[data-io-field="type"]', pou.io.type);
    fillTemplateSelect(node.querySelector(".pou-default-template"), pou.defaultTemplateKey, { ioType: pou.io.type });
    for (const [key, value] of Object.entries(pou.page)) {
      setInputValue(node, `[data-page-field="${key}"]`, value);
    }

    const moduleList = node.querySelector(".module-list");
    const moduleFragment = document.createDocumentFragment();
    pou.io.modules.forEach((module, moduleIndex) => moduleFragment.append(renderModule(pou, module, moduleIndex)));
    moduleList.replaceChildren(moduleFragment);
    moduleList.hidden = pou.io.modules.length === 0;
    node.querySelector(".module-empty").hidden = pou.io.modules.length !== 0;
    node.querySelector(".module-count").textContent = `${pou.io.modules.length} ${plural(pou.io.modules.length, "модуль", "модуля", "модулей")} · ${pouSignals.length} ${plural(pouSignals.length, "сигнал", "сигнала", "сигналов")}`;
    node.querySelector(".add-module").disabled = allSignals().length >= MAX_DOCUMENT_SIGNALS;
    return node;
  }

  function renderModule(pou, module, index) {
    const node = elements.moduleCardTemplate.content.firstElementChild.cloneNode(true);
    node.dataset.moduleId = module.uid;
    const expanded = module.uid === state.expandedModuleId && pou.id === state.expandedPouId;
    const toggle = node.querySelector(".module-toggle");
    const content = node.querySelector(".module-content");
    const contentId = `module-content-${module.uid}`;
    toggle.setAttribute("aria-expanded", String(expanded));
    toggle.setAttribute("aria-controls", contentId);
    toggle.title = expanded ? "Свернуть модуль" : "Развернуть модуль";
    toggle.setAttribute("aria-label", toggle.title);
    content.id = contentId;
    content.hidden = !expanded;

    node.querySelector(".module-kind").textContent = pou.io.type;
    node.querySelector(".module-kind").title = `Модуль ${index + 1}`;
    setInputValue(node, '[data-module-field="moduleId"]', module.moduleId);
    const capacity = capacityFor(pou);
    node.querySelector(".module-usage").textContent = `${module.signals.length}/${capacity}`;
    node.querySelector(".module-usage").title = `${module.signals.length} из ${capacity} каналов занято`;
    setEntityStatus(node.querySelector(".module-status"), moduleStatus(pou, module));
    if (!expanded) {
      content.replaceChildren();
      return node;
    }

    setInputValue(node, '[data-module-field="bindingPrefix"]', module.bindingPrefix);
    setInputValue(node, '[data-module-field="instanceName"]', module.instanceName);
    const instanceField = node.querySelector(".module-instance-field");
    if (instanceField) instanceField.hidden = !["DI", "DO"].includes(pou.io.type);
    const signalList = node.querySelector(".signal-list");
    const signalFragment = document.createDocumentFragment();
    module.signals.forEach((signal, signalIndex) => signalFragment.append(renderSignal(pou, module, signal, signalIndex)));
    signalList.replaceChildren(signalFragment);
    signalList.hidden = module.signals.length === 0;
    node.querySelector(".signal-empty").hidden = module.signals.length !== 0;
    node.querySelector(".add-signal").disabled = module.signals.length >= capacity
      || allSignals().length >= MAX_DOCUMENT_SIGNALS;
    return node;
  }

  function renderSignal(pou, module, signal, index) {
    const node = elements.signalCardTemplate.content.firstElementChild.cloneNode(true);
    node.dataset.signalId = signal.id;
    node.dataset.moduleId = module.uid;
    const expanded = signal.id === state.expandedSignalId && pou.id === state.expandedPouId;
    const toggle = node.querySelector(".signal-toggle");
    const content = node.querySelector(".signal-content");
    const contentId = `signal-content-${signal.id}`;
    toggle.setAttribute("aria-expanded", String(expanded));
    toggle.setAttribute("aria-controls", contentId);
    content.id = contentId;
    content.hidden = !expanded;

    node.querySelector(".signal-channel").textContent = String(index).padStart(2, "0");
    node.querySelector(".signal-channel").title = `Канал ${index}`;
    const inlineName = node.querySelector(".signal-inline-name");
    inlineName.value = signal.objectName;
    inlineName.setAttribute("aria-label", `Имя объекта, канал ${index}`);
    updateEffectiveTemplateBadge(node, pou, signal);
    setEntityStatus(node.querySelector(".signal-header .entity-status"), signalStatus(signal, pou));
    if (!expanded) {
      content.replaceChildren();
      return node;
    }

    const select = node.querySelector(".signal-template");
    fillTemplateSelect(select, signal.templateKey, { allowInherit: true, pou, ioType: pou.io.type });
    setInputValue(node, '[data-field="nameMode"]', signal.nameMode);
    setInputValue(node, '[data-field="description"]', signal.description);
    setInputValue(node, '[data-field="klPath"]', signal.klPath);
    setInputValue(node, '[data-field="offsetX"]', signal.offsetX);
    setInputValue(node, '[data-field="offsetY"]', signal.offsetY);
    renderSignalPreview(node, signal);
    return node;
  }

  function fillTemplateSelect(select, selectedKey, { allowInherit = false, pou = null, ioType = "" } = {}) {
    select.replaceChildren();
    const placeholder = document.createElement("option");
    placeholder.value = "";
    if (allowInherit) {
      const inherited = templateByKey(pou?.defaultTemplateKey)?.name || "не выбран";
      placeholder.textContent = `Общий шаблон: ${inherited}`;
    } else {
      placeholder.textContent = state.templates.some((item) => item.supported)
        ? "Выберите общий шаблон"
        : "Поддерживаемые шаблоны недоступны";
    }
    select.append(placeholder);

    const groups = new Map();
    for (const template of state.templates) {
      const key = template.libraryFile || "Без имени";
      if (!groups.has(key)) groups.set(key, []);
      groups.get(key).push(template);
    }
    for (const [library, templates] of groups) {
      const group = document.createElement("optgroup");
      group.label = library;
      for (const template of templates) {
        const option = document.createElement("option");
        option.value = template.key;
        const capabilities = template.ioCapabilities.length ? ` · ${template.ioCapabilities.join("/")}` : "";
        const mismatch = ioType && !template.ioCapabilities.includes(ioType)
          ? ` · не помечен для ${ioType}`
          : "";
        option.textContent = `${template.name}${template.id ? ` · #${template.id}` : ""}${capabilities}${mismatch}`;
        option.disabled = !template.supported;
        if (!template.supported) option.textContent += " · не поддерживается";
        group.append(option);
      }
      select.append(group);
    }
    select.value = selectedKey && state.templates.some((item) => item.key === selectedKey) ? selectedKey : "";
  }

  function updateEffectiveTemplateBadge(node, pou, signal) {
    const key = effectiveTemplateKey(pou, signal);
    const template = templateByKey(key);
    const badge = node.querySelector(".effective-template");
    badge.textContent = template?.name || "Шаблон не выбран";
    badge.dataset.mode = signal.templateKey ? "override" : "inherited";
    badge.dataset.compatibility = template && !template.ioCapabilities.includes(pou.io.type)
      ? "warning"
      : "ok";
    badge.title = signal.templateKey
      ? `Индивидуальный шаблон${template?.ioCapabilities.length ? ` · ${template.ioCapabilities.join("/")}` : ""}`
      : `Общий шаблон POU${template?.ioCapabilities.length ? ` · ${template.ioCapabilities.join("/")}` : ""}`;
  }

  function setInputValue(root, selector, value) {
    const input = root.querySelector(selector);
    if (input) input.value = safeText(value);
  }

  function handleEditorClick(event) {
    const pouNode = event.target.closest(".pou-card");
    if (!pouNode) return;
    const pouId = pouNode.dataset.pouId;
    const moduleNode = event.target.closest(".module-card");
    const moduleUid = moduleNode?.dataset.moduleId || null;
    const signalNode = event.target.closest(".signal-card");
    const signalId = signalNode?.dataset.signalId || null;

    if (event.target.closest(".pou-toggle")) {
      state.activePouId = pouId;
      state.expandedPouId = state.expandedPouId === pouId ? null : pouId;
      if (state.expandedPouId !== pouId) {
        state.expandedModuleId = null;
        state.expandedSignalId = null;
      }
      renderPous();
      renderCatalog();
      updateProjectUi();
      return;
    }
    if (event.target.closest(".copy-pou")) return copyPou(pouId);
    if (event.target.closest(".delete-pou")) return deletePou(pouId);
    if (event.target.closest(".add-module")) return addModule(pouId, { focus: true });
    if (event.target.closest(".module-toggle") && moduleUid) {
      state.activePouId = pouId;
      state.activeModuleId = moduleUid;
      state.expandedPouId = pouId;
      state.expandedModuleId = state.expandedModuleId === moduleUid ? null : moduleUid;
      if (state.expandedModuleId !== moduleUid) state.expandedSignalId = null;
      renderPous();
      updateProjectUi();
      return;
    }
    if (event.target.closest(".copy-module") && moduleUid) return copyModule(pouId, moduleUid);
    if (event.target.closest(".delete-module") && moduleUid) return deleteModule(pouId, moduleUid);
    if (event.target.closest(".add-signal") && moduleUid) return addSignal(pouId, moduleUid, "", { focus: true });
    if (event.target.closest(".signal-toggle") && signalId) {
      state.activePouId = pouId;
      state.activeModuleId = moduleUid;
      state.expandedPouId = pouId;
      state.expandedModuleId = moduleUid;
      state.expandedSignalId = state.expandedSignalId === signalId ? null : signalId;
      renderPous();
      updateProjectUi();
      return;
    }
    if (event.target.closest(".copy-signal") && signalId) return copySignal(pouId, moduleUid, signalId);
    if (event.target.closest(".delete-signal") && signalId) return deleteSignal(pouId, moduleUid, signalId);
  }

  function handleEditorInput(event) {
    if (state.generating) return;
    updateEditorModel(event.target, false);
  }

  function handleEditorChange(event) {
    if (state.generating) return;
    if (event.target.matches("select")) updateEditorModel(event.target, true);
  }

  function updateEditorModel(target, fromChange) {
    const pouNode = target.closest(".pou-card");
    if (!pouNode) return;
    const pou = pouById(pouNode.dataset.pouId);
    if (!pou) return;
    state.activePouId = pou.id;

    const moduleNode = target.closest(".module-card");
    const module = moduleById(pou, moduleNode?.dataset.moduleId);
    const signalNode = target.closest(".signal-card");
    if (signalNode) {
      const signal = signalById(module, signalNode.dataset.signalId);
      const field = target.dataset.field;
      if (!signal || !field) return;
      if (fromChange && field !== "templateKey" && field !== "nameMode") return;
      signal[field] = target.value;
      if (field === "offsetX") signal.autoX = false;
      if (field === "offsetY") signal.autoY = false;
      if (field === "templateKey") reflowAutomaticSignals(pou);
      if (["templateKey", "objectName", "nameMode"].includes(field)) {
        signal.preview = { state: "idle", objectNames: [], status: "Требуется проверка", error: "" };
        scheduleSignalPreview(signal);
      }
      clearValidationUi();
      if (field === "templateKey") {
        renderPous();
        updateProjectUi();
        return;
      }
      updateSignalHeader(signalNode, pou, signal);
      renderSignalPreview(signalNode, signal);
      updatePouHeader(pouNode, pou);
      updateProjectUi();
      return;
    }

    if (module && target.dataset.moduleField) {
      const field = target.dataset.moduleField;
      module[field] = target.value;
      state.activeModuleId = module.uid;
      clearValidationUi();
      updateModuleHeader(moduleNode, pou, module);
      updatePouHeader(pouNode, pou);
      updateProjectUi();
      return;
    }

    if (target.dataset.ioField === "type") {
      pou.io.type = target.value;
      clearValidationUi();
      renderPous();
      renderCatalog();
      updateProjectUi();
      return;
    }

    if (target.dataset.pageField) {
      pou.page[target.dataset.pageField] = target.value;
    } else if (target.dataset.field === "defaultTemplateKey") {
      setPouDefaultTemplate(pou, target.value);
      clearValidationUi();
      renderPous();
      updateProjectUi();
      return;
    } else if (target.dataset.field) {
      pou[target.dataset.field] = target.value;
    } else {
      return;
    }
    clearValidationUi();
    updatePouHeader(pouNode, pou);
    updateProjectUi();
  }

  function updatePouHeader(node, pou) {
    node.querySelector(".pou-display-name").textContent = pou.name || "POU без имени";
    const signals = allPouSignals(pou);
    node.querySelector(".pou-meta").textContent = `${pou.io.type} · ${pou.io.modules.length} мод. · ${signals.length} сигн. · ${templateByKey(pou.defaultTemplateKey)?.name || "шаблон не выбран"}${pou.pouNumber ? ` · POUNum ${pou.pouNumber}` : ""}`;
    setEntityStatus(node.querySelector(".pou-header .entity-status"), pouStatus(pou));
  }

  function updateModuleHeader(node, pou, module) {
    if (!node) return;
    const usage = node.querySelector(".module-usage");
    if (usage) usage.textContent = `${module.signals.length}/${capacityFor(pou)}`;
    const status = node.querySelector(".module-status");
    if (status) setEntityStatus(status, moduleStatus(pou, module));
  }

  function updateSignalHeader(node, pou, signal) {
    updateEffectiveTemplateBadge(node, pou, signal);
    setEntityStatus(node.querySelector(".signal-header .entity-status"), signalStatus(signal, pou));
  }

  function pouStatus(pou) {
    if (state.lastErrors.some((error) => error.kind === "pou" && error.id === pou.id)) return { state: "error", text: "Ошибка" };
    if (!isPouNameValid(pou.name) || !templateByKey(pou.defaultTemplateKey)?.supported || !IO_CAPACITY[pou.io.type] || pou.io.modules.length === 0) return { state: "draft", text: "Не заполнено" };
    if (pou.io.modules.some((module) => moduleStatus(pou, module).state !== "ready")) return { state: "draft", text: "Есть черновики" };
    return { state: "ready", text: "Готово" };
  }

  function moduleStatus(pou, module) {
    if (state.lastErrors.some((error) => error.kind === "module" && error.id === module.uid)) return { state: "error", text: "Ошибка" };
    const moduleID = Number(module.moduleId);
    if (!Number.isSafeInteger(moduleID) || moduleID < 0 || module.signals.length === 0 || module.signals.length > capacityFor(pou)) return { state: "draft", text: "Не заполнено" };
    if (module.signals.some((signal) => signalStatus(signal, pou).state !== "ready")) return { state: "draft", text: "Есть черновики" };
    return { state: "ready", text: "Готово" };
  }

  function signalStatus(signal, pou = signalOwner(signal)?.pou) {
    if (state.lastErrors.some((error) => error.kind === "signal" && error.id === signal.id)) return { state: "error", text: "Ошибка" };
    if (signal.preview.state === "error") return { state: "error", text: "Ошибка" };
    if (!templateByKey(effectiveTemplateKey(pou, signal))?.supported || !isObjectNameValid(signal.objectName)) return { state: "draft", text: "Не заполнено" };
    if (signal.preview.state === "loading") return { state: "loading", text: "Проверка" };
    if (signal.preview.state !== "success") return { state: "draft", text: "Не проверено" };
    return { state: "ready", text: "Готово" };
  }

  function setEntityStatus(node, status) {
    node.dataset.state = status.state;
    node.textContent = status.text;
  }

  function scheduleSignalPreview(signal) {
    disposeSignalPreview(signal.id);
    const owner = signalOwner(signal);
    const key = effectiveTemplateKey(owner?.pou, signal);
    if (!templateByKey(key)?.supported || !isObjectNameValid(signal.objectName)) {
      signal.preview = {
        state: "idle",
        objectNames: [],
        status: signal.objectName.trim() ? "Проверьте имя и шаблон" : "Введите имя",
        error: "",
      };
      updateSignalPreviewView(signal);
      return;
    }
    signal.preview = { state: "loading", objectNames: [], status: "Проверяю…", error: "" };
    updateSignalPreviewView(signal);
    previewTimers.set(signal.id, window.setTimeout(() => fetchSignalPreview(signal), NAME_PREVIEW_DELAY));
  }

  async function fetchSignalPreview(signal, { throwOnError = false } = {}) {
    disposeSignalPreview(signal.id);
    const owner = signalOwner(signal);
    const key = effectiveTemplateKey(owner?.pou, signal);
    if (!templateByKey(key)?.supported || !isObjectNameValid(signal.objectName)) return false;

    const signature = signalSignature(signal, owner?.pou);
    const controller = new AbortController();
    previewControllers.set(signal.id, controller);
    signal.preview = { state: "loading", objectNames: [], status: "Проверяю…", error: "" };
    updateSignalPreviewView(signal);

    try {
      const payload = await apiRequest(API.previewName, {
        method: "POST",
        signal: controller.signal,
        body: JSON.stringify({
          templateKey: key,
          objectName: signal.objectName.trim(),
          nameMode: signal.nameMode,
        }),
      });
      if (controller.signal.aborted || signature !== signalSignature(signal, owner?.pou)) return false;
      const names = normalizePreviewNames(payload);
      const pieces = [];
      if (payload.baseName) pieces.push(`база: ${payload.baseName}`);
      if (payload.matchedPrefix) pieces.push(`суффикс: ${payload.matchedPrefix}`);
      if (names.length) pieces.push(`${names.length} ${plural(names.length, "объект", "объекта", "объектов")}`);
      signal.preview = {
        state: "success",
        objectNames: names,
        status: pieces.join(" · ") || "Имена проверены",
        error: "",
      };
      updateSignalPreviewView(signal);
      return true;
    } catch (error) {
      if (error?.name === "AbortError") return false;
      signal.preview = { state: "error", objectNames: [], status: "Ошибка проверки", error: errorMessage(error) };
      updateSignalPreviewView(signal);
      if (throwOnError) throw error;
      return false;
    } finally {
      if (previewControllers.get(signal.id) === controller) previewControllers.delete(signal.id);
    }
  }

  function normalizePreviewNames(payload) {
    return Array.isArray(payload?.objectNames)
      ? payload.objectNames
        .map((item) => typeof item === "string" ? item : item?.name || item?.info || "")
        .map(String)
        .filter(Boolean)
      : [];
  }

  function renderSignalPreview(node, signal) {
    const preview = node.querySelector(".name-preview");
    if (!preview) return;
    preview.dataset.state = signal.preview.state;
    const status = preview.querySelector(".preview-status");
    status.textContent = signal.preview.error || signal.preview.status || "Введите имя";
    status.dataset.state = signal.preview.state;
    const fragment = document.createDocumentFragment();
    for (const name of signal.preview.objectNames || []) {
      const chip = document.createElement("span");
      chip.className = "derived-name";
      chip.title = name;
      chip.textContent = name;
      fragment.append(chip);
    }
    preview.querySelector(".derived-names").replaceChildren(fragment);
  }

  function updateSignalPreviewView(signal) {
    const node = elements.pouList.querySelector(`[data-signal-id="${signal.id}"]`);
    if (!node) {
      updateProjectUi();
      return;
    }
    renderSignalPreview(node, signal);
    const owner = signalOwner(signal);
    const pou = owner?.pou;
    updateSignalHeader(node, pou, signal);
    const pouNode = pou ? elements.pouList.querySelector(`[data-pou-id="${pou.id}"]`) : null;
    if (pou && pouNode) updatePouHeader(pouNode, pou);
    updateProjectUi();
  }

  function disposeSignalPreview(signalId) {
    const timer = previewTimers.get(signalId);
    if (timer) window.clearTimeout(timer);
    previewTimers.delete(signalId);
    previewControllers.get(signalId)?.abort();
    previewControllers.delete(signalId);
  }

  function signalSignature(signal, pou = signalOwner(signal)?.pou) {
    return `${effectiveTemplateKey(pou, signal)}\u0000${signal.objectName.trim()}\u0000${signal.nameMode}`;
  }

  async function generateXml(event) {
    event.preventDefault();
    if (state.generating) return;
    if (elements.reloadAll.disabled) {
      setFormStatus("Дождитесь завершения обновления библиотек.", "error");
      return;
    }
    clearValidationUi();

    let errors = validateProject({ requirePreview: false });
    if (errors.length) {
      presentValidation(errors);
      setFormStatus("Исправьте отмеченные поля.", "error");
      return;
    }

    state.generating = true;
    setGenerateLoading(true);
    setFormStatus("Проверяю имена всех сигналов…");
    try {
      await runWithConcurrency(allSignals(), PREVIEW_CONCURRENCY, ({ signal }) => fetchSignalPreview(signal));
      errors = validateProject({ requirePreview: true });
      if (errors.length) {
        presentValidation(errors);
        setFormStatus("Не удалось собрать проект: исправьте отмеченные поля.", "error");
        return;
      }

      setFormStatus("Собираю и проверяю общий XML…");
      const payload = buildGeneratePayload();
      const payloadBody = JSON.stringify(payload);
      if (new TextEncoder().encode(payloadBody).byteLength > MAX_JSON_BYTES) {
        throw new Error("Проект превышает допустимый размер JSON 8 МиБ.");
      }
      const result = await apiRequest(API.generate, { method: "POST", body: payloadBody });
      const resultName = safeText(result.fileName, payload.fileName || "XML-файл");
      setFormStatus(`Готово: ${resultName}`, "success");
      showToast(`Файл «${resultName}» успешно создан.`, "success");
      const warnings = Array.isArray(result.warnings) ? result.warnings : [];
      if (warnings.length) showToast(warnings.join(" · "), "warning", 9000);
      await loadOutputs({ announceErrors: false });
    } catch (error) {
      setFormStatus(errorMessage(error), "error");
      showToast(errorMessage(error), "error", 9000);
    } finally {
      state.generating = false;
      setGenerateLoading(false);
      renderPous();
      updateProjectUi();
    }
  }

  function validateProject({ requirePreview }) {
    const errors = [];
    const add = (kind, id, field, message) => errors.push({ kind, id, field, message });
    const signalEntries = allSignals();
    const requirements = signalEntries.reduce((totals, { pou, signal }) => {
      const template = templateByKey(effectiveTemplateKey(pou, signal));
      totals.objects += nonNegativeInteger(template?.primitiveCount);
      totals.cards += nonNegativeInteger(template?.cardCount);
      return totals;
    }, { objects: 0, cards: 0 });
    for (const pou of state.pous) {
      for (const module of pou.io.modules) {
        switch (pou.io.type) {
        case "AI":
          requirements.objects += module.signals.length * 4;
          requirements.cards += module.signals.length;
          break;
        case "AO":
          requirements.objects += module.signals.length * 2;
          requirements.cards += module.signals.length;
          break;
        case "DI":
        case "DO":
          requirements.objects += 65 + module.signals.length;
          requirements.cards += 33;
          break;
        default:
          break;
        }
      }
    }
    const rawFileName = stripXmlExtension(elements.fileName.value.trim());
    if (!rawFileName) add("global", "project", "fileName", "Укажите имя итогового файла.");
    else if (!sanitizeFilePart(rawFileName)) add("global", "project", "fileName", "Имя файла не содержит допустимых символов.");
    validateOptionalInteger(elements.t11Start.value, "global", "project", "t11Start", "Начальный T11ID", add, { min: 1, max: MAX_TRANSPORT_ID });
    validateOptionalInteger(elements.cardStart.value, "global", "project", "cardStart", "Начальный cardId", add, { min: 1, max: MAX_TRANSPORT_ID });
    validateTransportRangeEnd(elements.t11Start.value, requirements.objects, "t11Start", "Диапазон T11ID", add);
    validateTransportRangeEnd(elements.cardStart.value, requirements.cards, "cardStart", "Диапазон cardId", add);

    if (!state.pous.length) add("global", "project", "pous", "Добавьте хотя бы одну POU.");
    if (state.pous.length > MAX_DOCUMENT_POUS) add("global", "project", "pous", `Один файл может содержать не более ${MAX_DOCUMENT_POUS} POU.`);
    if (signalEntries.length > MAX_DOCUMENT_SIGNALS) add("global", "project", "pous", `Один файл может содержать не более ${MAX_DOCUMENT_SIGNALS} сигналов.`);
    if (requirements.objects > MAX_DOCUMENT_OBJECTS) add("global", "project", "pous", `Шаблоны содержат более ${MAX_DOCUMENT_OBJECTS} графических объектов.`);
    if (requirements.cards > MAX_DOCUMENT_CARDS) add("global", "project", "pous", `Шаблоны содержат более ${MAX_DOCUMENT_CARDS} карточек.`);

    const pouNames = new Map();
    const pouIds = new Map();
    const pouNumbers = new Map();
    const moduleIDs = new Map();
    const rawObjectNames = new Map();
    const derivedNames = new Map();

    for (const pou of state.pous) {
      if (!pou.name.trim()) add("pou", pou.id, "name", "Укажите имя POU.");
      else if (!isPouNameValid(pou.name)) add("pou", pou.id, "name", "Имя POU: буква или подчёркивание в начале, затем буквы, цифры и подчёркивания.");
      registerDuplicate(pouNames, canonical(pou.name), pou, "pou", "name", "Имя POU должно быть уникальным.", add);
      if (pou.description.length > 500) add("pou", pou.id, "description", "Описание POU не должно быть длиннее 500 символов.");
      if (!IO_CAPACITY[pou.io.type]) add("pou", pou.id, "ioType", "Выберите тип ввода-вывода AI, AO, DI или DO.");
      const defaultTemplate = templateByKey(pou.defaultTemplateKey);
      if (!defaultTemplate) add("pou", pou.id, "defaultTemplateKey", "Выберите общий шаблон элементов POU.");
      else if (!defaultTemplate.supported) add("pou", pou.id, "defaultTemplateKey", defaultTemplate.warnings[0] || "Общий шаблон не поддерживается генератором.");

      validateOptionalInteger(pou.pouId, "pou", pou.id, "pouId", "POU ID", add, { min: 1, max: 2147483647 });
      validateOptionalInteger(pou.groupId, "pou", pou.id, "groupId", "GroupID", add, { min: 1, max: 2147483647 });
      validateOptionalInteger(pou.pouNumber, "pou", pou.id, "pouNumber", "POUNum", add, { min: 1, max: 2147483647 });
      validateOptionalInteger(pou.page.width, "pou", pou.id, "width", "Ширина страницы", add, { min: 1, max: 1000000 });
      validateOptionalInteger(pou.page.height, "pou", pou.id, "height", "Высота страницы", add, { min: 1, max: 1000000 });
      validateOptionalInteger(pou.page.dparams, "pou", pou.id, "dparams", "DPARAMS", add, { min: 0, max: 2147483647 });
      validateOptionalInteger(pou.page.backgroundColor, "pou", pou.id, "backgroundColor", "Цвет фона", add, { min: 0, max: 2147483647 });
      validateOptionalInteger(pou.page.marginRight, "pou", pou.id, "marginRight", "Правое поле", add, { min: 0, max: 1000000 });
      validateOptionalInteger(pou.page.marginBottom, "pou", pou.id, "marginBottom", "Нижнее поле", add, { min: 0, max: 1000000 });
      if (pou.pouId) registerDuplicate(pouIds, pou.pouId, pou, "pou", "pouId", "POU ID должен быть уникальным.", add);
      if (pou.pouNumber) registerDuplicate(pouNumbers, pou.pouNumber, pou, "pou", "pouNumber", "POUNum должен быть уникальным.", add);

      if (!pou.io.modules.length) add("pou", pou.id, "modules", "Добавьте в POU хотя бы один физический модуль.");
      for (const module of pou.io.modules) {
        validateRequiredInteger(module.moduleId, "module", module.uid, "moduleId", "ID физического модуля", add, { min: 0, max: MAX_TRANSPORT_ID });
        const moduleKey = safeText(module.moduleId).trim();
        if (moduleKey) registerDuplicate(moduleIDs, moduleKey, { id: module.uid }, "module", "moduleId", "ID физического модуля должен быть уникальным во всём XML.", add);
        if (module.bindingPrefix.length > 160) add("module", module.uid, "bindingPrefix", "Card.Info base не должен быть длиннее 160 символов.");
        else if (module.bindingPrefix && !isObjectNameValid(module.bindingPrefix)) add("module", module.uid, "bindingPrefix", "Card.Info base содержит недопустимые символы.");
        if (["DI", "DO"].includes(pou.io.type) && module.instanceName && !isObjectNameValid(module.instanceName)) add("module", module.uid, "instanceName", "Имя экземпляра содержит недопустимые символы.");
        if (!module.signals.length) add("module", module.uid, "signals", "Добавьте в физический модуль хотя бы один сигнал.");
        if (module.signals.length > capacityFor(pou)) add("module", module.uid, "signals", `Модуль ${pou.io.type} вмещает не более ${capacityFor(pou)} сигналов.`);

        for (const signal of module.signals) {
          const selectedTemplate = templateByKey(effectiveTemplateKey(pou, signal));
          if (!selectedTemplate) add("signal", signal.id, "templateKey", "Выберите общий или индивидуальный шаблон.");
          else if (!selectedTemplate.supported) add("signal", signal.id, "templateKey", selectedTemplate.warnings[0] || "Выбранный шаблон пока не поддерживается генератором.");
          else if (!selectedTemplate.ioCapabilities.includes(pou.io.type)) add("signal", signal.id, "templateKey", `Шаблон «${selectedTemplate.name}» не имеет подтверждённой физической привязки ${pou.io.type}. Выберите совместимый override.`);
          if (!signal.objectName.trim()) add("signal", signal.id, "objectName", "Укажите имя объекта.");
          else if (!isObjectNameValid(signal.objectName)) add("signal", signal.id, "objectName", "Допустимы буквы, цифры, точки, дефисы и подчёркивания.");
          registerDuplicate(rawObjectNames, canonical(signal.objectName), signal, "signal", "objectName", "Имя объекта должно быть уникальным во всём XML.", add);
          if (signal.description.length > 500) add("signal", signal.id, "description", "Описание не должно быть длиннее 500 символов.");
          if (signal.klPath.length > 500) add("signal", signal.id, "klPath", "KLPath не должен быть длиннее 500 символов.");
          validateRequiredInteger(signal.offsetX, "signal", signal.id, "offsetX", "Координата X", add, { min: -10000, max: 100000 });
          validateRequiredInteger(signal.offsetY, "signal", signal.id, "offsetY", "Координата Y", add, { min: -10000, max: 100000 });

          if (requirePreview) {
            if (signal.preview.state === "error") add("signal", signal.id, "objectName", signal.preview.error || "Не удалось проверить итоговые имена.");
            else if (signal.preview.state !== "success") add("signal", signal.id, "objectName", "Итоговые имена не проверены.");
            for (const name of signal.preview.objectNames || []) {
              registerDuplicate(derivedNames, canonical(name), signal, "signal", "objectName", `Итоговое имя «${name}» повторяется в другом сигнале.`, add);
            }
          }
        }
      }
    }
    return deduplicateErrors(errors);
  }

  function registerDuplicate(map, key, entity, kind, field, message, add) {
    if (!key) return;
    if (map.has(key)) {
      const previous = map.get(key);
      add(kind, entity.id, field, message);
      add(kind, previous.id, field, message);
    } else {
      map.set(key, entity);
    }
  }

  function validateOptionalInteger(raw, kind, id, field, label, add, { min = 1, max = Number.MAX_SAFE_INTEGER } = {}) {
    if (safeText(raw).trim() === "") return;
    const value = Number(raw);
    if (!Number.isSafeInteger(value) || value < min || value > max) add(kind, id, field, `${label}: требуется целое число от ${min} до ${max}.`);
  }

  function validateRequiredInteger(raw, kind, id, field, label, add, { min = Number.MIN_SAFE_INTEGER, max = Number.MAX_SAFE_INTEGER } = {}) {
    const value = Number(raw);
    if (safeText(raw).trim() === "" || !Number.isSafeInteger(value) || value < min || value > max) add(kind, id, field, `${label}: требуется целое число от ${min} до ${max}.`);
  }

  function validateTransportRangeEnd(raw, count, field, label, add) {
    if (safeText(raw).trim() === "" || count === 0) return;
    const start = Number(raw);
    if (!Number.isSafeInteger(start) || start < 1 || start > MAX_TRANSPORT_ID) return;
    if (start > MAX_TRANSPORT_ID - count + 1) {
      add("global", "project", field, `${label} выходит за signed 32-bit с учётом выбранных шаблонов.`);
    }
  }

  async function runWithConcurrency(items, limit, worker) {
    let nextIndex = 0;
    const run = async () => {
      while (nextIndex < items.length) {
        const index = nextIndex;
        nextIndex += 1;
        await worker(items[index], index);
      }
    };
    const workerCount = Math.min(Math.max(1, limit), items.length);
    await Promise.all(Array.from({ length: workerCount }, run));
  }

  function deduplicateErrors(errors) {
    const seen = new Set();
    return errors.filter((error) => {
      const key = `${error.kind}|${error.id}|${error.field}|${error.message}`;
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    });
  }

  function presentValidation(errors) {
    state.lastErrors = errors;
    const first = errors[0];
    if (first?.kind === "pou") state.expandedPouId = first.id;
    if (first?.kind === "module") {
      const owner = moduleOwner(first.id);
      state.expandedPouId = owner?.pou.id || state.expandedPouId;
      state.activePouId = owner?.pou.id || state.activePouId;
      state.expandedModuleId = first.id;
      state.activeModuleId = first.id;
    }
    if (first?.kind === "signal") {
      const owner = signalOwnerByID(first.id);
      state.expandedPouId = owner?.pou.id || state.expandedPouId;
      state.activePouId = owner?.pou.id || state.activePouId;
      state.expandedModuleId = owner?.module.uid || state.expandedModuleId;
      state.activeModuleId = owner?.module.uid || state.activeModuleId;
      state.expandedSignalId = first.id;
    }
    renderPous();

    for (const error of errors) markFieldError(error);
    const strong = document.createElement("strong");
    strong.textContent = `${errors.length} ${plural(errors.length, "ошибка", "ошибки", "ошибок")} в проекте`;
    const list = document.createElement("ul");
    for (const error of errors.slice(0, 6)) {
      const item = document.createElement("li");
      item.textContent = error.message;
      list.append(item);
    }
    if (errors.length > 6) {
      const item = document.createElement("li");
      item.textContent = `И ещё ${errors.length - 6}…`;
      list.append(item);
    }
    elements.validationSummary.replaceChildren(strong, list);
    elements.validationSummary.hidden = false;
    updateProjectUi();
    focusFirstError(first);
  }

  function markFieldError(error) {
    let root = document;
    if (error.kind === "pou") root = elements.pouList.querySelector(`[data-pou-id="${error.id}"]`) || document;
    if (error.kind === "module") root = elements.pouList.querySelector(`[data-module-id="${error.id}"]`) || document;
    if (error.kind === "signal") root = elements.pouList.querySelector(`[data-signal-id="${error.id}"]`) || document;
    let input = null;
    if (error.kind === "global") {
      input = { fileName: elements.fileName, t11Start: elements.t11Start, cardStart: elements.cardStart }[error.field] || null;
    } else {
      input = root.querySelector(`[data-field="${error.field}"], [data-page-field="${error.field}"], [data-io-field="${error.field === "ioType" ? "type" : error.field}"], [data-module-field="${error.field}"]`);
    }
    if (input) input.setAttribute("aria-invalid", "true");
    const message = root.querySelector?.(`[data-error-for="${error.field}"]`);
    if (message && !message.textContent) message.textContent = error.message;
  }

  function focusFirstError(error) {
    if (!error) return;
    window.requestAnimationFrame(() => {
      let input = null;
      if (error.kind === "global") input = { fileName: elements.fileName, t11Start: elements.t11Start, cardStart: elements.cardStart }[error.field] || null;
      if (error.kind === "pou") {
        input = elements.pouList.querySelector(`[data-pou-id="${error.id}"] [data-field="${error.field}"], [data-pou-id="${error.id}"] [data-page-field="${error.field}"]`);
        if (!input && error.field === "ioType") input = elements.pouList.querySelector(`[data-pou-id="${error.id}"] [data-io-field="type"]`);
      }
      if (error.kind === "module") {
        input = elements.pouList.querySelector(`[data-module-id="${error.id}"] [data-module-field="${error.field}"]`);
        if (!input && error.field === "signals") input = elements.pouList.querySelector(`[data-module-id="${error.id}"] .add-signal`);
      }
      if (error.kind === "signal") input = elements.pouList.querySelector(`[data-signal-id="${error.id}"] [data-field="${error.field}"]`);
      input?.scrollIntoView({ behavior: "smooth", block: "center" });
      input?.focus({ preventScroll: true });
    });
  }

  function clearValidationUi() {
    state.lastErrors = [];
    elements.validationSummary.hidden = true;
    elements.validationSummary.replaceChildren();
    for (const input of document.querySelectorAll('[aria-invalid="true"]')) input.removeAttribute("aria-invalid");
    for (const message of document.querySelectorAll(".field-error")) message.textContent = "";
  }

  function buildGeneratePayload() {
    const payload = {
      fileName: normalizeRequestedFileName(elements.fileName.value),
      pous: state.pous.map((pou) => {
        const result = {
          name: pou.name.trim(),
          defaultTemplateKey: pou.defaultTemplateKey,
          page: {},
          io: {
            type: pou.io.type,
            modules: pou.io.modules.map((module) => ({
              id: Number(module.moduleId),
              bindingPrefix: module.bindingPrefix.trim(),
              instanceName: ["DI", "DO"].includes(pou.io.type) ? module.instanceName.trim() : "",
              signals: module.signals.map((signal) => ({
                templateKey: signal.templateKey,
                objectName: signal.objectName.trim(),
                nameMode: signal.nameMode,
                description: signal.description.trim(),
                klPath: signal.klPath.trim(),
                offsetX: Number(signal.offsetX),
                offsetY: Number(signal.offsetY),
              })),
            })),
          },
        };
        if (pou.description.trim()) result.description = pou.description.trim();
        addOptionalNumber(result, "pouId", pou.pouId);
        addOptionalNumber(result, "groupId", pou.groupId);
        addOptionalNumber(result, "pouNumber", pou.pouNumber);
        addOptionalNumber(result.page, "width", pou.page.width);
        addOptionalNumber(result.page, "height", pou.page.height);
        addOptionalString(result.page, "dparams", pou.page.dparams);
        addOptionalString(result.page, "backgroundColor", pou.page.backgroundColor);
        addOptionalNumber(result.page, "marginRight", pou.page.marginRight);
        addOptionalNumber(result.page, "marginBottom", pou.page.marginBottom);
        return result;
      }),
    };
    addOptionalNumber(payload, "t11Start", elements.t11Start.value);
    addOptionalNumber(payload, "cardStart", elements.cardStart.value);
    return payload;
  }

  function addOptionalNumber(target, key, raw) {
    if (safeText(raw).trim() !== "") target[key] = Number(raw);
  }

  function addOptionalString(target, key, raw) {
    const value = safeText(raw).trim();
    if (value !== "") target[key] = value;
  }

  function updateProjectUi() {
    const signalCount = allSignals().length;
    const moduleCount = state.pous.reduce((sum, pou) => sum + pou.io.modules.length, 0);
    elements.projectSummary.textContent = `${state.pous.length} POU · ${moduleCount} мод. · ${signalCount} сигн.`;
    const active = activePou();
    const module = activeModule(active);
    elements.catalogTarget.textContent = active
      ? `Цель: ${active.name || "POU без имени"}${module ? ` · модуль ${module.moduleId || "—"}` : ""}`
      : "Новая POU будет создана автоматически";

    let health;
    if (!state.pous.length) health = { state: "empty", text: "Проект пуст" };
    else if (state.lastErrors.length) health = { state: "error", text: `${state.lastErrors.length} ${plural(state.lastErrors.length, "ошибка", "ошибки", "ошибок")}` };
    else if (state.pous.some((pou) => pouStatus(pou).state !== "ready")) health = { state: "draft", text: "Есть незаполненные данные" };
    else health = { state: "ready", text: "Готов к генерации" };
    elements.projectHealth.dataset.state = health.state;
    elements.projectHealth.textContent = health.text;
    elements.addPou.disabled = state.generating || state.pous.length >= MAX_DOCUMENT_POUS;
    elements.generateButton.disabled = state.generating || !state.pous.length;
  }

  function setGenerateLoading(loading) {
    elements.projectForm.inert = loading;
    elements.catalogPanel.inert = loading;
    elements.projectForm.setAttribute("aria-busy", String(loading));
    elements.reloadAll.disabled = loading;
    elements.addPou.disabled = loading || state.pous.length >= MAX_DOCUMENT_POUS;
    elements.generateButton.disabled = loading || !state.pous.length;
    elements.generateButton.dataset.loading = String(loading);
    elements.generateButton.setAttribute("aria-busy", String(loading));
    elements.generateButtonLabel.textContent = loading ? "Формирование…" : "Сформировать XML";
  }

  async function refreshLibraries() {
    if (state.generating || elements.reloadAll.disabled) return;
    setButtonBusy(elements.reloadAll, true);
    setConnectionState("checking", "Обновляю библиотеки…");
    try {
      await apiRequest(API.refresh, { method: "POST", body: JSON.stringify({}) });
      await Promise.all([
        loadTemplates({ announceErrors: false }),
        loadOutputs({ announceErrors: false }),
      ]);
      showToast("Библиотеки перечитаны.", "success");
    } catch (error) {
      setConnectionState("offline", "Ошибка обновления");
      showToast(errorMessage(error), "error", 9000);
    } finally {
      setButtonBusy(elements.reloadAll, false);
    }
  }

  async function loadOutputs({ announceErrors = false } = {}) {
    state.outputsLoading = true;
    elements.outputList.setAttribute("aria-busy", "true");
    setButtonBusy(elements.reloadOutputs, true);
    renderOutputs();
    let failure = "";
    try {
      const payload = await apiRequest(API.outputs);
      state.outputs = Array.isArray(payload.files) ? payload.files : [];
      state.outputs.sort((left, right) => dateValue(right.createdAt) - dateValue(left.createdAt));
    } catch (error) {
      state.outputs = [];
      failure = errorMessage(error);
      if (announceErrors) showToast(failure, "error");
    } finally {
      state.outputsLoading = false;
      elements.outputList.setAttribute("aria-busy", "false");
      setButtonBusy(elements.reloadOutputs, false);
      renderOutputs(failure);
    }
  }

  function renderOutputs(error = "") {
    if (state.outputsLoading && !error) {
      const loading = document.createElement("div");
      loading.className = "loading-message";
      loading.textContent = "Загружаю список файлов…";
      elements.outputList.replaceChildren(loading);
      elements.outputList.hidden = false;
      elements.outputEmpty.hidden = true;
      return;
    }
    if (error) {
      const message = document.createElement("div");
      message.className = "loading-message error-message";
      message.textContent = error;
      elements.outputList.replaceChildren(message);
      elements.outputList.hidden = false;
      elements.outputEmpty.hidden = true;
      return;
    }

    const fragment = document.createDocumentFragment();
    for (const file of state.outputs) {
      const node = elements.outputItemTemplate.content.firstElementChild.cloneNode(true);
      const name = safeText(file.name, "result.xml");
      node.querySelector(".output-name").textContent = name;
      node.querySelector(".output-meta").textContent = [formatBytes(file.size), formatDate(file.createdAt)].filter(Boolean).join(" · ");
      const link = node.querySelector(".button-download");
      link.href = safeDownloadUrl(file.url, name);
      link.download = name;
      link.setAttribute("aria-label", `Скачать ${name}`);
      fragment.append(node);
    }
    elements.outputList.replaceChildren(fragment);
    elements.outputList.hidden = state.outputs.length === 0;
    elements.outputEmpty.hidden = state.outputs.length !== 0;
  }

  function safeDownloadUrl(rawUrl, fileName) {
    const fallback = `/api/output/${encodeURIComponent(fileName)}`;
    if (!rawUrl) return fallback;
    try {
      const url = new URL(String(rawUrl), window.location.origin);
      if (url.origin !== window.location.origin || !["http:", "https:"].includes(url.protocol)) return fallback;
      return `${url.pathname}${url.search}${url.hash}`;
    } catch {
      return fallback;
    }
  }

  function setConnectionState(status, text) {
    elements.connectionStatus.dataset.state = status;
    elements.connectionStatusText.textContent = text;
  }

  function setFormStatus(text, status = "") {
    elements.formStatus.textContent = text;
    if (status) elements.formStatus.dataset.state = status;
    else elements.formStatus.removeAttribute("data-state");
  }

  function setButtonBusy(button, busy) {
    button.disabled = busy;
    button.setAttribute("aria-busy", String(busy));
    button.classList.toggle("is-loading", busy);
  }

  function showToast(message, status = "info", duration = 5200) {
    const toast = document.createElement("div");
    toast.className = "toast";
    toast.dataset.state = status;
    toast.setAttribute("role", status === "error" ? "alert" : "status");
    const icon = document.createElement("span");
    icon.className = "toast-icon";
    icon.setAttribute("aria-hidden", "true");
    icon.textContent = status === "success" ? "✓" : status === "error" || status === "warning" ? "!" : "i";
    const text = document.createElement("span");
    text.className = "toast-message";
    text.textContent = safeText(message);
    const close = document.createElement("button");
    close.className = "toast-close";
    close.type = "button";
    close.setAttribute("aria-label", "Закрыть уведомление");
    close.textContent = "×";
    const dismiss = () => {
      if (!toast.isConnected || toast.classList.contains("is-leaving")) return;
      toast.classList.add("is-leaving");
      window.setTimeout(() => toast.remove(), 180);
    };
    close.addEventListener("click", dismiss);
    toast.append(icon, text, close);
    elements.toastRegion.append(toast);
    window.setTimeout(dismiss, duration);
  }

  function activePou() {
    return pouById(state.activePouId) || state.pous[0] || null;
  }

  function pouById(id) {
    return state.pous.find((item) => item.id === id) || null;
  }

  function activeModule(pou = activePou()) {
    return moduleById(pou, state.activeModuleId) || pou?.io.modules[0] || null;
  }

  function moduleById(pou, id) {
    return pou?.io.modules.find((item) => item.uid === id) || null;
  }

  function signalById(module, id) {
    return module?.signals.find((item) => item.id === id) || null;
  }

  function moduleOwner(moduleUid) {
    for (const pou of state.pous) {
      const module = moduleById(pou, moduleUid);
      if (module) return { pou, module };
    }
    return null;
  }

  function signalOwner(signal) {
    for (const pou of state.pous) {
      for (const module of pou.io.modules) {
        if (module.signals.includes(signal)) return { pou, module };
      }
    }
    return null;
  }

  function signalOwnerByID(signalId) {
    for (const pou of state.pous) {
      for (const module of pou.io.modules) {
        const signal = signalById(module, signalId);
        if (signal) return { pou, module, signal };
      }
    }
    return null;
  }

  function allPouSignals(pou) {
    return (pou?.io.modules || []).flatMap((module) => module.signals);
  }

  function templateByKey(key) {
    return state.templates.find((item) => item.key === key) || null;
  }

  function allSignals() {
    return state.pous.flatMap((pou) => pou.io.modules.flatMap((module) => module.signals.map((signal, channel) => ({ pou, module, signal, channel }))));
  }

  function uid(prefix) {
    return `${prefix}-${nextUid++}`;
  }

  function uniquePouName(base) {
    return uniqueName(base, state.pous.map((item) => item.name));
  }

  function uniqueObjectName(base) {
    return uniqueName(base, allSignals().map((item) => item.signal.objectName));
  }

  function uniqueName(base, existing) {
    const values = new Set(existing.map(canonical).filter(Boolean));
    let candidate = safeText(base).trim() || "COPY";
    let index = 2;
    while (values.has(canonical(candidate))) candidate = `${base}_${index++}`;
    return candidate;
  }

  function copyName(value, existing) {
    return uniqueName(`${safeText(value).trim() || "ITEM"}_COPY`, existing);
  }

  function canonical(value) {
    return safeText(value).trim().normalize("NFKC").toLocaleUpperCase("ru-RU");
  }

  function isPouNameValid(value) {
    const text = safeText(value).trim();
    return Array.from(text).length <= 160 && /^[\p{L}_][\p{L}\p{N}_]*$/u.test(text);
  }

  function isObjectNameValid(value) {
    const text = safeText(value).trim();
    return text.length > 0 && Array.from(text).length <= 160 && /^[\p{L}\p{N}_.-]+$/u.test(text);
  }

  function focusAfterRender(selector, select = false) {
    window.requestAnimationFrame(() => {
      const input = document.querySelector(selector);
      input?.scrollIntoView({ behavior: "smooth", block: "center" });
      input?.focus({ preventScroll: true });
      if (select && typeof input?.select === "function") input.select();
    });
  }

  function normalizeRequestedFileName(value) {
    const stripped = stripXmlExtension(safeText(value).trim());
    return `${sanitizeFilePart(stripped) || "scheme"}.xml`;
  }

  function stripXmlExtension(value) {
    return safeText(value).replace(/\.xml$/iu, "");
  }

  function sanitizeFilePart(value) {
    return safeText(value)
      .replace(/[<>:"/\\|?*\u0000-\u001f]/gu, "_")
      .replace(/\s+/gu, "_")
      .replace(/_+/gu, "_")
      .replace(/[. ]+$/gu, "")
      .slice(0, 170);
  }

  function safeText(value, fallback = "") {
    return value === null || value === undefined ? fallback : String(value);
  }

  function shortWarning(value) {
    const text = safeText(value).replace(/\s+/gu, " ").trim();
    return text.length > 92 ? `${text.slice(0, 89)}…` : text;
  }

  function formatBackendError(error) {
    if (typeof error === "string") return error;
    if (!error || typeof error !== "object") return "Не удалось прочитать одну из библиотек.";
    const prefix = error.file ? `${error.file}: ` : "";
    return `${prefix}${error.error || error.message || "ошибка разбора библиотеки"}`;
  }

  function errorMessage(error) {
    return safeText(error?.message || error, "Неизвестная ошибка.");
  }

  function finiteNumber(value, fallback = 0) {
    const number = Number(value);
    return Number.isFinite(number) ? number : fallback;
  }

  function positiveNumber(value, fallback = 0) {
    const number = finiteNumber(value, fallback);
    return number > 0 ? number : fallback;
  }

  function nonNegativeInteger(value) {
    return Math.max(0, Math.trunc(finiteNumber(value, 0)));
  }

  function integerOr(value, fallback) {
    const number = Number(value);
    return Number.isSafeInteger(number) ? number : fallback;
  }

  function formatInteger(value) {
    return new Intl.NumberFormat("ru-RU", { maximumFractionDigits: 0 }).format(finiteNumber(value, 0));
  }

  function formatBytes(value) {
    const bytes = Number(value);
    if (!Number.isFinite(bytes) || bytes < 0) return "";
    if (bytes < 1024) return `${bytes} Б`;
    const units = ["КБ", "МБ", "ГБ"];
    let amount = bytes / 1024;
    let index = 0;
    while (amount >= 1024 && index < units.length - 1) {
      amount /= 1024;
      index += 1;
    }
    return `${new Intl.NumberFormat("ru-RU", { maximumFractionDigits: amount < 10 ? 1 : 0 }).format(amount)} ${units[index]}`;
  }

  function formatDate(value) {
    const date = new Date(value);
    if (!value || Number.isNaN(date.getTime())) return "";
    return new Intl.DateTimeFormat("ru-RU", {
      day: "2-digit", month: "2-digit", year: "numeric", hour: "2-digit", minute: "2-digit",
    }).format(date);
  }

  function dateValue(value) {
    const date = new Date(value).getTime();
    return Number.isFinite(date) ? date : 0;
  }

  function plural(number, one, few, many) {
    const absolute = Math.abs(number) % 100;
    const last = absolute % 10;
    if (absolute > 10 && absolute < 20) return many;
    if (last > 1 && last < 5) return few;
    if (last === 1) return one;
    return many;
  }

  init();
})();
