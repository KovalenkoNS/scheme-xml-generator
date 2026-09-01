(() => {
  "use strict";

  const API = Object.freeze({
    templates: "/api/templates",
    refresh: "/api/refresh",
    previewName: "/api/preview-name",
    generate: "/api/generate",
    outputs: "/api/outputs",
  });

  const SVG_NS = "http://www.w3.org/2000/svg";
  const NAME_PREVIEW_DELAY = 320;

  const elements = {
    connectionStatus: document.querySelector("#connection-status"),
    connectionStatusText: document.querySelector("#connection-status .status-text"),
    reloadAll: document.querySelector("#reload-all"),
    templateTotal: document.querySelector("#template-total"),
    templateSearch: document.querySelector("#template-search"),
    clearSearch: document.querySelector("#clear-search"),
    librarySummary: document.querySelector("#library-summary"),
    templateList: document.querySelector("#template-list"),
    templateEmpty: document.querySelector("#template-empty"),
    templateCardTemplate: document.querySelector("#template-card-template"),
    templateDimensions: document.querySelector("#template-dimensions"),
    previewSvg: document.querySelector("#scheme-preview"),
    previewContent: document.querySelector("#preview-content"),
    previewPlaceholder: document.querySelector("#preview-placeholder"),
    selectedTemplateMeta: document.querySelector("#selected-template-meta"),
    selectedTemplateName: document.querySelector("#selected-template-name"),
    selectedTemplateSource: document.querySelector("#selected-template-source"),
    selectedTemplateDescription: document.querySelector("#selected-template-description"),
    selectedTemplateStats: document.querySelector("#selected-template-stats"),
    selectedTemplateWarnings: document.querySelector("#selected-template-warnings"),
    generatorForm: document.querySelector("#generator-form"),
    generatorFields: document.querySelector("#generator-fields"),
    objectName: document.querySelector("#object-name"),
    objectNameError: document.querySelector("#object-name-error"),
    pouName: document.querySelector("#pou-name"),
    pouNameError: document.querySelector("#pou-name-error"),
    nameMode: document.querySelector("#name-mode"),
    namePreview: document.querySelector("#name-preview"),
    namePreviewStatus: document.querySelector("#name-preview-status"),
    derivedNames: document.querySelector("#derived-names"),
    description: document.querySelector("#object-description"),
    klPath: document.querySelector("#kl-path"),
    offsetX: document.querySelector("#offset-x"),
    offsetY: document.querySelector("#offset-y"),
    fileName: document.querySelector("#file-name"),
    t11Start: document.querySelector("#t11-start"),
    cardStart: document.querySelector("#card-start"),
    pouId: document.querySelector("#pou-id"),
    pouGroupId: document.querySelector("#pou-group-id"),
    pouNumber: document.querySelector("#pou-number"),
    formStatus: document.querySelector("#form-status"),
    generateButton: document.querySelector("#generate-button"),
    generateButtonLabel: document.querySelector("#generate-button .button-label"),
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
    selectedKey: null,
    currentNamePreview: null,
    namePreviewTimer: null,
    namePreviewController: null,
    fileNameDirty: false,
    pouNameDirty: false,
    suggestedPouName: "",
    generating: false,
  };

  function init() {
    bindEvents();
    Promise.allSettled([loadTemplates(), loadOutputs()]);
  }

  function bindEvents() {
    elements.reloadAll.addEventListener("click", refreshLibraries);
    elements.reloadOutputs.addEventListener("click", () => loadOutputs({ announceErrors: true }));

    elements.templateSearch.addEventListener("input", () => {
      elements.clearSearch.hidden = elements.templateSearch.value.length === 0;
      renderTemplateList();
    });

    elements.clearSearch.addEventListener("click", () => {
      elements.templateSearch.value = "";
      elements.clearSearch.hidden = true;
      renderTemplateList();
      elements.templateSearch.focus();
    });

    elements.templateList.addEventListener("keydown", handleTemplateKeyboardNavigation);

    elements.objectName.addEventListener("input", () => {
      validateObjectName(false);
      updateSuggestedFileName();
      scheduleNamePreview();
    });

    elements.objectName.addEventListener("blur", () => validateObjectName(true));
    elements.pouName.addEventListener("input", () => {
      state.pouNameDirty = elements.pouName.value.trim() !== state.suggestedPouName;
      validatePouName(false);
    });
    elements.pouName.addEventListener("blur", () => validatePouName(true));
    elements.nameMode.addEventListener("change", scheduleNamePreview);

    elements.fileName.addEventListener("input", () => {
      state.fileNameDirty = elements.fileName.value.trim().length > 0;
    });

    elements.fileName.addEventListener("blur", () => {
      elements.fileName.value = stripXmlExtension(elements.fileName.value.trim());
      state.fileNameDirty = elements.fileName.value.length > 0;
    });

    elements.offsetX.addEventListener("input", renderSelectedTemplate);
    elements.offsetY.addEventListener("input", renderSelectedTemplate);
    elements.generatorForm.addEventListener("submit", generateXml);
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
      if (error && error.name === "AbortError") {
        throw error;
      }
      throw new Error("Сервер приложения недоступен.");
    }

    const contentType = response.headers.get("content-type") || "";
    let payload = null;

    if (response.status !== 204) {
      try {
        payload = contentType.includes("application/json")
          ? await response.json()
          : await response.text();
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

  async function loadTemplates({ preserveSelection = true, announceErrors = true } = {}) {
    elements.templateList.setAttribute("aria-busy", "true");
    setConnectionState("checking", "Читаю библиотеки…");

    try {
      const payload = await apiRequest(API.templates);
      const previousKey = preserveSelection ? state.selectedKey : null;
      state.libraries = Array.isArray(payload.libraries) ? payload.libraries : [];
      state.templates = Array.isArray(payload.templates)
        ? payload.templates.map(normalizeTemplate).filter((item) => item.key)
        : [];

      const stillExists = previousKey && state.templates.some((item) => item.key === previousKey);
      state.selectedKey = stillExists ? previousKey : state.templates[0]?.key || null;

      renderLibrarySummary(payload.errors);
      renderTemplateList();
      renderSelectedTemplate();
      setConnectionState("online", "Сервер подключён");

      const scanErrors = Array.isArray(payload.errors) ? payload.errors : [];
      if (announceErrors && scanErrors.length > 0) {
        showToast(formatBackendError(scanErrors[0]), "error", 8000);
      }
    } catch (error) {
      state.libraries = [];
      state.templates = [];
      state.selectedKey = null;
      renderLibrarySummary([]);
      renderTemplateList();
      renderSelectedTemplate();
      setConnectionState("offline", "Нет связи с сервером");
      if (announceErrors) {
        showToast(errorMessage(error), "error", 8000);
      }
    } finally {
      elements.templateList.setAttribute("aria-busy", "false");
    }
  }

  function normalizeTemplate(raw) {
    const preview = raw && typeof raw.preview === "object" && raw.preview ? raw.preview : {};
    return {
      ...raw,
      key: String(raw?.key ?? ""),
      libraryFile: String(raw?.libraryFile ?? ""),
      ownerId: String(raw?.ownerId ?? ""),
      ownerName: String(raw?.ownerName ?? ""),
      id: String(raw?.id ?? ""),
      name: String(raw?.name || `Шаблон ${raw?.id ?? ""}`),
      description: String(raw?.description || "Без описания"),
      width: finiteNumber(raw?.width, finiteNumber(preview.width, 0)),
      height: finiteNumber(raw?.height, finiteNumber(preview.height, 0)),
      primitiveCount: nonNegativeInteger(raw?.primitiveCount),
      blockCount: nonNegativeInteger(raw?.blockCount),
      linkCount: nonNegativeInteger(raw?.linkCount),
      graphicCount: nonNegativeInteger(raw?.graphicCount),
      cardCount: nonNegativeInteger(raw?.cardCount),
      warnings: Array.isArray(raw?.warnings) ? raw.warnings.map(String) : [],
      preview: {
        width: finiteNumber(preview.width, finiteNumber(raw?.width, 0)),
        height: finiteNumber(preview.height, finiteNumber(raw?.height, 0)),
        blocks: Array.isArray(preview.blocks) ? preview.blocks : [],
        lines: Array.isArray(preview.lines) ? preview.lines : [],
      },
    };
  }

  function renderLibrarySummary(errors = []) {
    elements.librarySummary.replaceChildren();
    const libraryCount = state.libraries.length;
    const templateCount = state.templates.length;
    elements.templateTotal.textContent = String(templateCount);

    if (libraryCount === 0) {
      elements.librarySummary.textContent = "Нет прочитанных библиотек";
      return;
    }

    const versions = [...new Set(state.libraries.map((item) => item.version).filter(Boolean).map(String))];
    const strong = document.createElement("strong");
    strong.textContent = `${libraryCount} ${plural(libraryCount, "библиотека", "библиотеки", "библиотек")}`;
    elements.librarySummary.append(strong);

    const details = [` · ${templateCount} ${plural(templateCount, "шаблон", "шаблона", "шаблонов")}`];
    if (versions.length > 0) {
      details.push(` · версия ${versions.join(", ")}`);
    }
    if (Array.isArray(errors) && errors.length > 0) {
      details.push(` · ошибок: ${errors.length}`);
    }
    elements.librarySummary.append(document.createTextNode(details.join("")));
  }

  function renderTemplateList() {
    const query = elements.templateSearch.value.trim().toLocaleLowerCase("ru-RU");
    const filtered = state.templates.filter((template) => {
      if (!query) return true;
      return [template.name, template.description, template.libraryFile, template.ownerName, template.id]
        .some((value) => String(value).toLocaleLowerCase("ru-RU").includes(query));
    });

    const fragment = document.createDocumentFragment();
    for (const template of filtered) {
      const node = elements.templateCardTemplate.content.firstElementChild.cloneNode(true);
      node.dataset.key = template.key;
      node.setAttribute("aria-selected", String(template.key === state.selectedKey));
      node.setAttribute("aria-label", `${template.name}, ${template.primitiveCount} примитивов`);
      node.querySelector(".template-card-name").textContent = template.name;
      node.querySelector(".template-card-id").textContent = template.id ? `#${template.id}` : "";
      node.querySelector(".template-card-description").textContent = template.description;
      node.querySelector(".template-card-library").textContent = template.libraryFile || template.ownerName || "Библиотека";
      node.querySelector(".template-card-count").textContent = `${template.primitiveCount} прим.`;
      node.addEventListener("click", () => selectTemplate(template.key));
      fragment.append(node);
    }

    elements.templateList.replaceChildren(fragment);
    elements.templateList.hidden = filtered.length === 0;
    elements.templateEmpty.hidden = filtered.length !== 0;
  }

  function selectTemplate(key) {
    if (!state.templates.some((item) => item.key === key)) return;
    state.selectedKey = key;
    state.currentNamePreview = null;

    for (const card of elements.templateList.querySelectorAll(".template-card")) {
      card.setAttribute("aria-selected", String(card.dataset.key === key));
    }

    renderSelectedTemplate();
    updateSuggestedFileName();
    scheduleNamePreview();
  }

  function selectedTemplate() {
    return state.templates.find((item) => item.key === state.selectedKey) || null;
  }

  function renderSelectedTemplate() {
    const template = selectedTemplate();
    elements.previewContent.replaceChildren();

    if (!template) {
      elements.previewSvg.setAttribute("hidden", "");
      elements.previewPlaceholder.hidden = false;
      elements.previewPlaceholder.querySelector("strong").textContent = "Выберите шаблон слева";
      elements.previewPlaceholder.querySelector("p").textContent = "Здесь появится компоновка блоков и связей.";
      elements.selectedTemplateMeta.hidden = true;
      elements.templateDimensions.hidden = true;
      elements.generatorFields.disabled = true;
      clearNamePreview("Сначала выберите шаблон");
      return;
    }

    elements.generatorFields.disabled = false;
    updateSuggestedPouName(template);
    elements.selectedTemplateMeta.hidden = false;
    elements.selectedTemplateName.textContent = template.name;
    elements.selectedTemplateSource.textContent = [template.libraryFile, template.id ? `ID ${template.id}` : ""]
      .filter(Boolean)
      .join(" · ");
    elements.selectedTemplateDescription.textContent = template.description;
    renderTemplateStats(template);
    renderTemplateWarnings(template.warnings);
    renderSvgPreview(template);
  }

  function renderTemplateStats(template) {
    const stats = [
      ["Примитивы", template.primitiveCount],
      ["Блоки", template.blockCount],
      ["Связи", template.linkCount],
      ["Графика", template.graphicCount],
      ["Карточки", template.cardCount],
    ];

    const fragment = document.createDocumentFragment();
    for (const [label, value] of stats) {
      const chip = document.createElement("span");
      chip.className = "stat-chip";
      const name = document.createTextNode(`${label} `);
      const count = document.createElement("strong");
      count.textContent = String(value);
      chip.append(name, count);
      fragment.append(chip);
    }
    elements.selectedTemplateStats.replaceChildren(fragment);
  }

  function renderTemplateWarnings(warnings) {
    if (!warnings.length) {
      elements.selectedTemplateWarnings.hidden = true;
      elements.selectedTemplateWarnings.textContent = "";
      return;
    }
    elements.selectedTemplateWarnings.textContent = warnings.join(" · ");
    elements.selectedTemplateWarnings.hidden = false;
  }

  function renderSvgPreview(template) {
    const preview = template.preview;
    const blocks = preview.blocks.map(normalizePreviewBlock).filter(Boolean);
    const lines = preview.lines.map(normalizePreviewLine).filter((line) => line.length >= 2);
    const width = positiveNumber(preview.width, positiveNumber(template.width, inferPreviewWidth(blocks, lines)));
    const height = positiveNumber(preview.height, positiveNumber(template.height, inferPreviewHeight(blocks, lines)));
    const offsetX = finiteNumber(elements.offsetX.value, 0);
    const offsetY = finiteNumber(elements.offsetY.value, 0);

    elements.templateDimensions.textContent = `${formatInteger(width)} × ${formatInteger(height)} · ${signed(offsetX)}, ${signed(offsetY)}`;
    elements.templateDimensions.hidden = false;

    if (blocks.length === 0 && lines.length === 0) {
      elements.previewSvg.setAttribute("hidden", "");
      elements.previewPlaceholder.hidden = false;
      elements.previewPlaceholder.querySelector("strong").textContent = "Предпросмотр недоступен";
      elements.previewPlaceholder.querySelector("p").textContent = "Метаданные шаблона загружены без геометрии.";
      return;
    }

    const padding = Math.max(24, Math.min(width, height) * 0.035);
    const minX = Math.min(0, offsetX) - padding;
    const minY = Math.min(0, offsetY) - padding;
    const maxX = Math.max(width, offsetX + width) + padding;
    const maxY = Math.max(height, offsetY + height) + padding;
    elements.previewSvg.setAttribute("viewBox", `${minX} ${minY} ${Math.max(1, maxX - minX)} ${Math.max(1, maxY - minY)}`);
    elements.previewSvg.removeAttribute("hidden");
    elements.previewPlaceholder.hidden = true;

    const group = document.createElementNS(SVG_NS, "g");
    group.setAttribute("transform", `translate(${offsetX} ${offsetY})`);

    for (const points of lines) {
      const polyline = document.createElementNS(SVG_NS, "polyline");
      polyline.classList.add("preview-line");
      polyline.setAttribute("points", points.map((point) => `${point.x},${point.y}`).join(" "));
      group.append(polyline);
    }

    const fontSize = Math.max(10, Math.min(23, Math.max(width, height) / 74));
    for (const block of blocks) {
      const blockGroup = document.createElementNS(SVG_NS, "g");
      blockGroup.classList.add("preview-block");
      blockGroup.dataset.kind = blockKind(block.type);

      const rect = document.createElementNS(SVG_NS, "rect");
      rect.setAttribute("x", String(block.x));
      rect.setAttribute("y", String(block.y));
      rect.setAttribute("width", String(block.width));
      rect.setAttribute("height", String(block.height));
      rect.setAttribute("rx", String(Math.min(5, block.width / 8, block.height / 8)));
      blockGroup.append(rect);

      if (block.label && block.width >= fontSize * 2.2 && block.height >= fontSize * 1.15) {
        const text = document.createElementNS(SVG_NS, "text");
        text.setAttribute("x", String(block.x + block.width / 2));
        text.setAttribute("y", String(block.y + block.height / 2));
        text.setAttribute("dy", "0.35em");
        text.setAttribute("text-anchor", "middle");
        text.setAttribute("font-size", String(fontSize));
        text.textContent = truncateSvgLabel(block.label, block.width, fontSize);
        blockGroup.append(text);
      }

      if (block.label) {
        const title = document.createElementNS(SVG_NS, "title");
        title.textContent = block.label;
        blockGroup.append(title);
      }
      group.append(blockGroup);
    }

    elements.previewContent.replaceChildren(group);
  }

  function normalizePreviewBlock(raw) {
    if (!raw || typeof raw !== "object") return null;
    const width = positiveNumber(raw.width, 0);
    const height = positiveNumber(raw.height, 0);
    if (!width || !height) return null;
    return {
      x: finiteNumber(raw.x, 0),
      y: finiteNumber(raw.y, 0),
      width,
      height,
      label: String(raw.label || ""),
      type: Number(raw.type),
    };
  }

  function normalizePreviewLine(raw) {
    if (!Array.isArray(raw)) return [];
    return raw
      .filter((point) => point && typeof point === "object")
      .map((point) => ({ x: Number(point.x), y: Number(point.y) }))
      .filter((point) => Number.isFinite(point.x) && Number.isFinite(point.y));
  }

  function inferPreviewWidth(blocks, lines) {
    const values = blocks.map((block) => block.x + block.width);
    for (const line of lines) {
      values.push(...line.map((point) => point.x));
    }
    return Math.max(600, ...values, 0);
  }

  function inferPreviewHeight(blocks, lines) {
    const values = blocks.map((block) => block.y + block.height);
    for (const line of lines) {
      values.push(...line.map((point) => point.y));
    }
    return Math.max(400, ...values, 0);
  }

  function blockKind(type) {
    if (type === 37) return "object";
    if (type === 31 || type === 35) return "field";
    if (type === 34) return "constant";
    return "function";
  }

  function truncateSvgLabel(value, width, fontSize) {
    const maxChars = Math.max(2, Math.floor(width / (fontSize * 0.61)));
    if (value.length <= maxChars) return value;
    return `${value.slice(0, Math.max(1, maxChars - 1))}…`;
  }

  function scheduleNamePreview() {
    clearTimeout(state.namePreviewTimer);
    state.namePreviewController?.abort();
    state.currentNamePreview = null;

    if (!state.selectedKey) {
      clearNamePreview("Сначала выберите шаблон");
      return;
    }

    if (!validateObjectName(false)) {
      clearNamePreview(elements.objectName.value.trim() ? "Проверьте имя" : "Введите имя");
      return;
    }

    setNamePreviewLoading();
    state.namePreviewTimer = window.setTimeout(requestNamePreview, NAME_PREVIEW_DELAY);
  }

  async function requestNamePreview() {
    const objectName = elements.objectName.value.trim();
    if (!state.selectedKey || !objectName || !validateObjectName(false)) return;

    const controller = new AbortController();
    state.namePreviewController = controller;

    try {
      const payload = await apiRequest(API.previewName, {
        method: "POST",
        signal: controller.signal,
        body: JSON.stringify({
          templateKey: state.selectedKey,
          objectName,
          nameMode: elements.nameMode.value,
        }),
      });

      if (controller.signal.aborted) return;
      state.currentNamePreview = payload;
      renderNamePreview(payload);
    } catch (error) {
      if (error && error.name === "AbortError") return;
      state.currentNamePreview = null;
      elements.namePreview.dataset.state = "error";
      elements.namePreviewStatus.removeAttribute("data-state");
      elements.namePreviewStatus.textContent = errorMessage(error);
      elements.derivedNames.replaceChildren();
      setFormStatus("Не удалось проверить итоговые имена.", "error");
    } finally {
      if (state.namePreviewController === controller) {
        state.namePreviewController = null;
      }
    }
  }

  function renderNamePreview(payload) {
    const names = Array.isArray(payload.objectNames)
      ? payload.objectNames.map((item) => typeof item === "string" ? item : item?.name || item?.info || "").filter(Boolean)
      : [];
    const fragment = document.createDocumentFragment();

    for (const name of names) {
      const chip = document.createElement("span");
      chip.className = "derived-name";
      chip.title = name;
      chip.textContent = name;
      fragment.append(chip);
    }

    elements.derivedNames.replaceChildren(fragment);
    elements.namePreview.removeAttribute("data-state");
    elements.namePreviewStatus.removeAttribute("data-state");

    const pieces = [];
    if (payload.baseName) pieces.push(`база: ${payload.baseName}`);
    if (payload.matchedPrefix) pieces.push(`суффикс: ${payload.matchedPrefix}`);
    if (names.length > 0) pieces.push(`${names.length} ${plural(names.length, "объект", "объекта", "объектов")}`);
    elements.namePreviewStatus.textContent = pieces.join(" · ") || "Имена проверены";
    setFormStatus("Имена рассчитаны. Можно сформировать XML.", "success");
  }

  function setNamePreviewLoading() {
    elements.namePreview.removeAttribute("data-state");
    elements.namePreviewStatus.dataset.state = "loading";
    elements.namePreviewStatus.textContent = "Проверяю…";
    elements.derivedNames.replaceChildren();
  }

  function clearNamePreview(status) {
    state.currentNamePreview = null;
    elements.namePreview.removeAttribute("data-state");
    elements.namePreviewStatus.removeAttribute("data-state");
    elements.namePreviewStatus.textContent = status;
    elements.derivedNames.replaceChildren();
  }

  function validateObjectName(showError) {
    const value = elements.objectName.value.trim();
    let message = "";

    if (!value) {
      message = "Укажите имя объекта.";
    } else if (value.length > 160) {
      message = "Имя не должно быть длиннее 160 символов.";
    } else if (!/^[\p{L}\p{N}_.-]+$/u.test(value)) {
      message = "Допустимы буквы, цифры, точки, дефисы и подчёркивания.";
    }

    elements.objectName.setAttribute("aria-invalid", String(Boolean(message)));
    elements.objectNameError.textContent = message;
    elements.objectNameError.hidden = !message || !showError;
    return !message;
  }

  function validatePouName(showError) {
    const value = elements.pouName.value.trim();
    let message = "";

    if (!value) {
      message = "Укажите имя программного модуля POU.";
    } else if (Array.from(value).length > 160) {
      message = "Имя POU не должно быть длиннее 160 символов.";
    } else if (!/^[\p{L}_]/u.test(value)) {
      message = "Первый символ имени POU должен быть буквой или подчёркиванием.";
    } else if (!/^[\p{L}_][\p{L}\p{N}_]*$/u.test(value)) {
      message = "После первого символа допустимы только буквы, цифры и подчёркивания.";
    }

    elements.pouName.setAttribute("aria-invalid", String(Boolean(message)));
    elements.pouNameError.textContent = message;
    elements.pouNameError.hidden = !message || !showError;
    return !message;
  }

  function updateSuggestedPouName(template) {
    if (!template) return;
    const currentValue = elements.pouName.value.trim();
    const mayReplace = !state.pouNameDirty || !currentValue || currentValue === state.suggestedPouName;
    const suggestion = suggestPouName(template);
    state.suggestedPouName = suggestion;

    if (mayReplace) {
      elements.pouName.value = suggestion;
      state.pouNameDirty = false;
      validatePouName(false);
    }
  }

  function suggestPouName(template) {
    const source = String(template.name || template.id || "SCHEME").normalize("NFKC");
    const suffix = source
      .replace(/[^\p{L}\p{N}_]+/gu, "_")
      .replace(/_+/gu, "_")
      .replace(/^_+|_+$/gu, "") || "SCHEME";
    return Array.from(`POU_${suffix}`).slice(0, 160).join("");
  }

  function updateSuggestedFileName() {
    if (state.fileNameDirty) return;
    const objectName = elements.objectName.value.trim();
    const template = selectedTemplate();
    if (!objectName || !template) {
      elements.fileName.value = "";
      return;
    }
    elements.fileName.value = sanitizeFilePart(`${objectName}__${template.name}`);
  }

  async function generateXml(event) {
    event.preventDefault();
    if (state.generating) return;

    const template = selectedTemplate();
    const validName = validateObjectName(true);
    const validPouName = validatePouName(true);
    if (!template) {
      showToast("Сначала выберите шаблон.", "error");
      return;
    }

    if (!validName || !validPouName || !elements.generatorForm.checkValidity()) {
      elements.generatorForm.reportValidity();
      setFormStatus("Исправьте отмеченные поля.", "error");
      return;
    }

    let payload;
    try {
      payload = buildGeneratePayload();
    } catch (error) {
      setFormStatus(errorMessage(error), "error");
      showToast(errorMessage(error), "error");
      return;
    }

    state.generating = true;
    setGenerateLoading(true);
    setFormStatus("Собираю и проверяю XML…");

    try {
      const result = await apiRequest(API.generate, {
        method: "POST",
        body: JSON.stringify(payload),
      });

      const resultName = String(result.fileName || payload.fileName || "XML-файл");
      setFormStatus(`Готово: ${resultName}`, "success");
      showToast(`Файл «${resultName}» успешно создан.`, "success");

      const warnings = Array.isArray(result.warnings) ? result.warnings : [];
      if (warnings.length > 0) {
        showToast(warnings.join(" · "), "warning", 9000);
      }

      await loadOutputs({ announceErrors: false });
    } catch (error) {
      setFormStatus(errorMessage(error), "error");
      showToast(errorMessage(error), "error", 9000);
    } finally {
      state.generating = false;
      setGenerateLoading(false);
    }
  }

  function buildGeneratePayload() {
    const offsetX = requiredInteger(elements.offsetX, "Смещение X");
    const offsetY = requiredInteger(elements.offsetY, "Смещение Y");
    const payload = {
      templateKey: state.selectedKey,
      objectName: elements.objectName.value.trim(),
      pouName: elements.pouName.value.trim(),
      nameMode: elements.nameMode.value,
      description: elements.description.value.trim(),
      klPath: elements.klPath.value.trim(),
      offsetX,
      offsetY,
      fileName: normalizeRequestedFileName(elements.fileName.value),
    };

    addOptionalInteger(payload, "t11Start", elements.t11Start, "Начальный T11ID");
    addOptionalInteger(payload, "cardStart", elements.cardStart, "Начальный cardId");
    addOptionalInteger(payload, "pouId", elements.pouId, "POU ID");
    addOptionalInteger(payload, "pouGroupId", elements.pouGroupId, "POU GroupID");
    addOptionalInteger(payload, "pouNumber", elements.pouNumber, "POUNum");
    return payload;
  }

  function requiredInteger(input, label) {
    const value = Number(input.value);
    if (!Number.isSafeInteger(value)) {
      throw new Error(`${label}: требуется целое число.`);
    }
    return value;
  }

  function addOptionalInteger(target, key, input, label) {
    const raw = input.value.trim();
    if (!raw) return;
    const value = Number(raw);
    if (!Number.isSafeInteger(value) || value <= 0) {
      throw new Error(`${label}: требуется положительное целое число.`);
    }
    target[key] = value;
  }

  function setGenerateLoading(loading) {
    elements.generateButton.disabled = loading;
    elements.generateButton.dataset.loading = String(loading);
    elements.generateButton.setAttribute("aria-busy", String(loading));
    elements.generateButtonLabel.textContent = loading ? "Формирование…" : "Сформировать XML";
  }

  async function refreshLibraries() {
    if (elements.reloadAll.disabled) return;
    setButtonBusy(elements.reloadAll, true);
    setConnectionState("checking", "Обновляю библиотеки…");

    try {
      await apiRequest(API.refresh, { method: "POST", body: JSON.stringify({}) });
      await Promise.all([
        loadTemplates({ preserveSelection: true, announceErrors: false }),
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
    elements.outputList.setAttribute("aria-busy", "true");
    setButtonBusy(elements.reloadOutputs, true);

    try {
      const payload = await apiRequest(API.outputs);
      state.outputs = Array.isArray(payload.files) ? payload.files : [];
      state.outputs.sort((left, right) => dateValue(right.createdAt) - dateValue(left.createdAt));
      renderOutputs();
    } catch (error) {
      state.outputs = [];
      renderOutputs(errorMessage(error));
      if (announceErrors) showToast(errorMessage(error), "error");
    } finally {
      elements.outputList.setAttribute("aria-busy", "false");
      setButtonBusy(elements.reloadOutputs, false);
    }
  }

  function renderOutputs(error = "") {
    const fragment = document.createDocumentFragment();

    if (error) {
      const message = document.createElement("div");
      message.className = "output-loading";
      message.textContent = error;
      fragment.append(message);
    } else {
      for (const file of state.outputs) {
        const node = elements.outputItemTemplate.content.firstElementChild.cloneNode(true);
        const name = String(file.name || "result.xml");
        node.querySelector(".output-name").textContent = name;
        node.querySelector(".output-meta").textContent = [formatBytes(file.size), formatDate(file.createdAt)]
          .filter(Boolean)
          .join(" · ");

        const link = node.querySelector(".button-download");
        const safeUrl = safeDownloadUrl(file.url, name);
        link.href = safeUrl;
        link.download = name;
        link.setAttribute("aria-label", `Скачать ${name}`);
        fragment.append(node);
      }
    }

    elements.outputList.replaceChildren(fragment);
    elements.outputList.hidden = !error && state.outputs.length === 0;
    elements.outputEmpty.hidden = Boolean(error) || state.outputs.length !== 0;
  }

  function safeDownloadUrl(rawUrl, fileName) {
    const fallback = `/api/output/${encodeURIComponent(fileName)}`;
    if (!rawUrl) return fallback;
    try {
      const url = new URL(String(rawUrl), window.location.origin);
      if (url.origin !== window.location.origin || !["http:", "https:"].includes(url.protocol)) {
        return fallback;
      }
      return `${url.pathname}${url.search}${url.hash}`;
    } catch {
      return fallback;
    }
  }

  function handleTemplateKeyboardNavigation(event) {
    if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) return;
    const cards = [...elements.templateList.querySelectorAll(".template-card")];
    if (!cards.length) return;

    event.preventDefault();
    const currentIndex = cards.indexOf(document.activeElement);
    let nextIndex;
    if (event.key === "Home") nextIndex = 0;
    else if (event.key === "End") nextIndex = cards.length - 1;
    else if (event.key === "ArrowDown") nextIndex = Math.min(cards.length - 1, Math.max(0, currentIndex + 1));
    else nextIndex = Math.max(0, currentIndex < 0 ? 0 : currentIndex - 1);

    cards[nextIndex].focus();
    selectTemplate(cards[nextIndex].dataset.key);
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
    icon.textContent = status === "success" ? "✓" : status === "error" ? "!" : status === "warning" ? "!" : "i";

    const text = document.createElement("span");
    text.className = "toast-message";
    text.textContent = String(message);

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

  function normalizeRequestedFileName(value) {
    const stripped = stripXmlExtension(value.trim());
    if (!stripped) return "";
    return `${sanitizeFilePart(stripped)}.xml`;
  }

  function stripXmlExtension(value) {
    return value.replace(/\.xml$/iu, "");
  }

  function sanitizeFilePart(value) {
    return value
      .replace(/[<>:"/\\|?*\u0000-\u001f]/gu, "_")
      .replace(/\s+/gu, "_")
      .replace(/_+/gu, "_")
      .replace(/[. ]+$/gu, "")
      .slice(0, 170);
  }

  function safeText(value, fallback = "") {
    return value === null || value === undefined ? fallback : String(value);
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
    const number = Math.trunc(finiteNumber(value, 0));
    return Math.max(0, number);
  }

  function signed(value) {
    const number = finiteNumber(value, 0);
    return number >= 0 ? `+${formatInteger(number)}` : formatInteger(number);
  }

  function formatInteger(value) {
    return new Intl.NumberFormat("ru-RU", { maximumFractionDigits: 0 }).format(value);
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
      day: "2-digit",
      month: "2-digit",
      year: "numeric",
      hour: "2-digit",
      minute: "2-digit",
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
