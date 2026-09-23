const { test } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const { File } = require("node:buffer");

// Minimal DOM boundary for the tab controller and async upload flow. This does
// not replace rendered browser/layout checks and requires no npm dependencies.
class Element {
  constructor() {
    this.listeners = new Map();
    this.attributes = new Map();
    this.children = [];
    this.dataset = {};
    this.value = "";
    this.hidden = false;
    this.files = [];
    this.classes = new Set();
    this.classList = { toggle: (name, on) => on ? this.classes.add(name) : this.classes.delete(name) };
  }
  set textContent(value) { this.text = String(value); this.children = []; }
  get textContent() { return (this.text || "") + this.children.map((item) => item.textContent).join(""); }
  setAttribute(key, value) { this.attributes.set(key, value); }
  getAttribute(key) { return this.attributes.get(key); }
  append(...items) { this.children.push(...items.flatMap((item) => item.fragment ? item.children : [item])); }
  replaceChildren(...items) { this.text = ""; this.children = []; this.append(...items); }
  addEventListener(name, callback) { this.listeners.set(name, [...(this.listeners.get(name) || []), callback]); }
  async fire(name, properties = {}) {
    const event = { target: this, preventDefault() {}, ...properties };
    await Promise.all((this.listeners.get(name) || []).map((fn) => fn(event)));
  }
  focus() { this.focused = true; }
  closest(selector) { return selector === "label" ? this.label || null : this.details || null; }
}

const nativeContext = {
  version: "29", project: "native-project", controllerTypeName: "TENIX-CPU715",
  controllerId: "189300", resourceId: "637", groupId: "19498", pouNumber: "36",
};

function plan(tag = "_3107_TV_64101A") {
  return {
    groups: [{ key: "FCS/A11", fcs: "3000_D_SC_B01", prefix: "A11", pouName: "AO_A11",
      modules: [{ name: "A11_00", mainModule: "A11_00", redundantModule: "", channels: Array.from({ length: 4 }, (_, channel) => ({
        channel, tag: channel ? `_3000_D_SC_B01_A11_00_${channel}` : tag, reserve: channel !== 0,
        sourceRow: channel ? 0 : 2, min: "0", max: "100",
      })) }] }],
    rowCount: 1, groupCount: 1, moduleCount: 1, channelCount: 4, reserveCount: 3, duplicateCount: 0, uniqueTagCount: 1, warnings: [],
  };
}

const response = (data, ok = true) => ({ ok, status: ok ? 200 : 400, json: async () => data });
const settled = () => new Promise((resolve) => setImmediate(resolve));
const deferred = () => { let resolve; const promise = new Promise((done) => { resolve = done; }); return { promise, resolve }; };

function batchResult(fcsNames = ["3000_D_SC_B01"], baseName = "custom") {
  const files = fcsNames.map((fcs) => {
    const fileName = `${baseName}_${fcs}.xml`;
    return { fcs, fileName, url: `/api/output/${encodeURIComponent(fileName)}`, baseName,
      summary: { pouCount: 1, signalCount: 4, cards: 4, blocks: 4 }, warnings: [] };
  });
  return { files, summary: { pouCount: files.length, signalCount: files.length * 4, cards: files.length * 4, blocks: files.length * 4 }, warnings: [] };
}

async function setup(handler, profileResponse = response({ context: nativeContext, description: "Native AN_v1" })) {
  const elements = new Map();
  const get = (id) => {
    if (!elements.has(id)) elements.set(id, new Element());
    return elements.get(id);
  };
  const tabs = [get("scheme-tab"), get("temporary-tab")];
  tabs[0].setAttribute("aria-controls", "main-content");
  tabs[1].setAttribute("aria-controls", "temporary-content");
  const context = Object.keys(nativeContext).map((key) => {
    const element = new Element();
    element.dataset.temporaryContext = key;
    element.label = new Element();
    return element;
  });
  get("temporary-diagnostic-resource").value = "1";
  const events = [];
  const calls = [];
  const sandbox = {
    document: {
      querySelectorAll: (selector) => selector === ".workspace-tab" ? tabs : context,
      querySelector: () => get("skip-link"), getElementById: get,
      createElement: () => new Element(),
      createDocumentFragment: () => Object.assign(new Element(), { fragment: true }),
    },
    window: { location: { href: "http://localhost:3000/", origin: "http://localhost:3000" }, dispatchEvent: (event) => events.push(event.type) },
    fetch: async (url, options) => {
      calls.push({ url, options });
      if (url.endsWith("/profile")) return profileResponse;
      return handler(url, options);
    },
    FormData, AbortController, URL, Event, setTimeout, clearTimeout,
  };
  vm.runInNewContext(fs.readFileSync(path.join(__dirname, "temporary.js"), "utf8"), sandbox);
  await settled();
  return { get, tabs, context, events, calls };
}

async function choose(ui, file = new File(["source"], "map.txt")) {
  ui.get("temporary-file").files = file ? [file] : [];
  await ui.get("temporary-file").fire("change");
  return file;
}

async function mode(ui, value, selectAll = false) {
  ui.get("temporary-mode").value = value;
  await ui.get("temporary-mode").fire("change");
  if (selectAll) await ui.get(value === "diagnostic" ? "temporary-diagnostic-select-all" : "temporary-st-select-all").fire("click");
}

function pouControl(ui, index = 0) {
  const row = ui.get("temporary-st-pous").children.flatMap((plc) => plc.children[1].children)[index];
  const details = row.children[1];
  return {
    row, details, summary: details.children[0],
    selected: row.children[0].children[0],
    count: details.children[1].children[0].children[1],
    ids: details.children[2].children.map((label) => label.children[1]),
  };
}

function plcControl(ui, index = 0) {
  const section = ui.get("temporary-st-pous").children[index];
  return { section, selected: section.children[0].children[0], count: section.children[0].children[2] };
}

function multiplePOUs() {
  const preview = plan();
  const group = (fcs, prefix) => {
    const source = plan(`_${fcs}_${prefix}_SIGNAL`).groups[0];
    Object.assign(source, { fcs, prefix, key: `${fcs}/${prefix}`, pouName: `AO_${prefix}` });
    source.modules[0].name = `${prefix}_00`;
    source.modules[0].channels.forEach((channel) => {
      if (channel.reserve) channel.tag = `_${fcs}_${prefix}_00_${channel.channel}`;
    });
    return source;
  };
  preview.groups = [group("FCS1", "A11"), group("FCS1", "A12"), group("FCS2", "A11")];
  Object.assign(preview, { groupCount: 3, moduleCount: 3, channelCount: 12, reserveCount: 9 });
  return preview;
}

test("tabs keep the generator form in place and support keyboard navigation", async () => {
  const ui = await setup(() => assert.fail("No upload expected"));
  const originalField = new Element();
  originalField.value = "USER_POU";
  ui.get("main-content").append(originalField);
  await ui.tabs[0].fire("keydown", { key: "ArrowRight" });
  assert.equal(ui.get("main-content").hidden, true);
  assert.equal(ui.get("temporary-content").hidden, false);
  assert.equal(ui.tabs[1].getAttribute("aria-selected"), "true");
  assert.equal(ui.tabs[1].focused, true);
  await ui.tabs[1].fire("keydown", { key: "Home" });
  assert.equal(ui.get("main-content").hidden, false);
  assert.equal(ui.get("main-content").children[0], originalField);
  assert.equal(originalField.value, "USER_POU");
  assert.equal(ui.get("skip-link").getAttribute("href"), "#main-content");
});

test("preview renders channel zero, source rows and four slots without HTML interpolation", async () => {
  const ui = await setup(() => response(plan("<tag&literal>")));
  await choose(ui);
  const rows = ui.get("temporary-rows").children;
  assert.equal(rows.length, 4);
  assert.equal(rows[0].children[2].textContent, "0");
  assert.equal(rows[0].children[0].textContent, "AO_A113000_D_SC_B01");
  assert.equal(rows[0].children[3].textContent, "<tag&literal>");
  assert.equal(rows[0].children[5].textContent, "2");
  assert.equal(rows[1].children[5].textContent, "—");
  assert.equal(rows[1].children[6].textContent, "Резерв");
  assert.equal(ui.get("temporary-generate").disabled, false);
  assert.deepEqual(ui.context.map((input) => input.value), Object.values(nativeContext));
  assert.match(ui.get("temporary-summary").textContent, /^1 FCS \/ XML · 1 POU/);
});

test("duplicate positions are visibly empty, searchable by original tag and distinct from reserves", async () => {
  const duplicateTag = "_DUPLICATE<tag&literal>";
  const preview = plan();
  Object.assign(preview.groups[0].modules[0].channels[1], {
    tag: duplicateTag, reserve: false, duplicate: true,
    duplicateOf: "AO_A11 / A11_00 / 0", sourceRow: 8,
  });
  preview.duplicateCount = 1;
  preview.reserveCount = 2;
  const ui = await setup(() => response(preview));
  await choose(ui);
  const rows = ui.get("temporary-rows").children;
  assert.equal(rows.length, 4);
  const duplicate = rows[1];
  assert.equal(duplicate.children[2].textContent, "1");
  assert.equal(duplicate.children[3].textContent, "—");
  assert.equal(duplicate.children[4].textContent, "—");
  assert.equal(duplicate.children[5].textContent, "8");
  assert.equal(duplicate.children[6].textContent, "Повтор — пусто");
  assert.equal(duplicate.classes.has("temporary-duplicate-row"), true);
  assert.equal(duplicate.classes.has("temporary-reserve-row"), false);
  assert.equal(duplicate.textContent.includes(duplicateTag), false);
  assert.equal(duplicate.children[3].title.includes(duplicateTag), true);
  assert.match(duplicate.children[3].title, /Первое размещение: AO_A11 \/ A11_00 \/ 0/);
  assert.equal(rows[2].children[3].textContent, "_3000_D_SC_B01_A11_00_2");
  assert.equal(rows[2].children[6].textContent, "Резерв");
  assert.match(ui.get("temporary-summary").textContent, /2 резервов · 1 повторов — пусто$/);
  ui.get("temporary-search").value = duplicateTag;
  await ui.get("temporary-search").fire("input");
  await new Promise((resolve) => setTimeout(resolve, 150));
  assert.equal(ui.get("temporary-rows").children.length, 1);
  assert.equal(ui.get("temporary-rows").children[0].children[6].textContent, "Повтор — пусто");
  assert.equal(ui.get("temporary-search-empty").hidden, true);
});

test("a stale preview cannot overwrite the latest selection even if abort is ignored", async () => {
  const first = deferred();
  let count = 0;
  const ui = await setup(() => ++count === 1 ? first.promise : response(plan("_NEW")));
  const oldRequest = choose(ui, new File(["old"], "old.txt"));
  await choose(ui, new File(["new"], "new.txt"));
  first.resolve(response(plan("_OLD")));
  await oldRequest;
  assert.match(ui.get("temporary-status").textContent, /new\.txt/);
  assert.equal(ui.get("temporary-rows").children[0].children[3].textContent, "_NEW");
  assert.equal(ui.calls[1].options.signal.aborted, true);
});

test("generation uploads the exact selected bytes and string context, then notifies outputs", async () => {
  const generated = deferred();
  const ui = await setup((url) => url.endsWith("/preview") ? response(plan()) : generated.promise);
  const file = await choose(ui, new File(["exact\r\noriginal bytes"], "map.txt"));
  ui.get("temporary-filename").value = " custom ";
  ui.context.find((input) => input.dataset.temporaryContext === "groupId").value = "0";
  const generation = ui.get("temporary-form").fire("submit");
  assert.equal(ui.get("temporary-file").disabled, true);
  assert.equal(ui.get("temporary-generate").disabled, true);
  const upload = ui.calls.at(-1).options.body;
  assert.equal(await upload.get("file").text(), await file.text());
  assert.equal(upload.get("fileName"), "custom");
  assert.equal(JSON.parse(upload.get("context")).groupId, "0");
  generated.resolve(response(batchResult()));
  await generation;
  assert.equal(ui.get("temporary-result").hidden, false);
  const rows = ui.get("temporary-downloads").children;
  assert.equal(rows.length, 1);
  assert.equal(rows[0].children[1].href, "http://localhost:3000/api/output/custom_3000_D_SC_B01.xml");
  assert.equal(rows[0].children[1].download, "custom_3000_D_SC_B01.xml");
  assert.match(rows[0].children[0].textContent, /3000_D_SC_B01.*custom_3000_D_SC_B01\.xml · 1 POU/);
  assert.equal(ui.get("temporary-file").disabled, false);
  assert.deepEqual(ui.events, ["schemegen:outputs-changed"]);
});

test("batch generation displays all eight FCS files and summed statistics", async () => {
  const result = batchResult(Array.from({ length: 8 }, (_, index) => `3000_D_SC_B0${index + 1}`));
  const ui = await setup((url) => response(url.endsWith("/preview") ? plan() : result));
  await choose(ui);
  await ui.get("temporary-form").fire("submit");
  const rows = ui.get("temporary-downloads").children;
  assert.equal(rows.length, 8);
  assert.match(ui.get("temporary-result-name").textContent, /8/);
  assert.equal(ui.get("temporary-result-summary").textContent, "8 POU · 32 каналов · 32 карточек · 32 блоков");
  rows.forEach((row, index) => {
    assert.equal(row.children[0].children[0].textContent, result.files[index].fcs);
    assert.equal(row.children[1].href, `http://localhost:3000${result.files[index].url}`);
    assert.equal(row.children[1].download, result.files[index].fileName);
  });
  assert.match(ui.get("temporary-status").textContent, /соответствующий ПЛК/);
  assert.deepEqual(ui.events, ["schemegen:outputs-changed"]);
});

test("AO map totals distinguish 512 positions from 309 blocks and 203 empty duplicates", async () => {
  const preview = plan();
  Object.assign(preview, {
    groupCount: 21, moduleCount: 128, channelCount: 512, reserveCount: 102,
    duplicateCount: 203, uniqueTagCount: 207, rowCount: 410,
  });
  const result = batchResult(Array.from({ length: 8 }, (_, index) => `3000_D_SC_B0${index + 1}`));
  result.summary = { pouCount: 21, signalCount: 512, cards: 309, blocks: 309, skippedDuplicateCount: 203 };
  const ui = await setup((url) => response(url.endsWith("/preview") ? preview : result));
  await choose(ui);
  assert.match(ui.get("temporary-summary").textContent, /512 каналов · 102 резервов · 203 повторов — пусто$/);
  await ui.get("temporary-form").fire("submit");
  assert.equal(ui.get("temporary-downloads").children.length, 8);
  assert.equal(ui.get("temporary-result-summary").textContent, "21 POU · 512 каналов · 309 карточек · 309 блоков · 203 повторов — пусто");
});

test("fallback result counts never count skipped duplicate positions as blocks or cards", async () => {
  const preview = plan();
  preview.duplicateCount = 1;
  const result = batchResult();
  result.summary = {};
  const ui = await setup((url) => response(url.endsWith("/preview") ? preview : result));
  await choose(ui);
  await ui.get("temporary-form").fire("submit");
  assert.equal(ui.get("temporary-result-summary").textContent, "1 POU · 4 каналов · 3 карточек · 3 блоков · 1 повторов — пусто");
});

test("download names render literally and links use the encoded same-origin output path", async () => {
  const result = batchResult(["<FCS&literal>"], "name <&>");
  const ui = await setup((url) => response(url.endsWith("/preview") ? plan() : result));
  await choose(ui);
  await ui.get("temporary-form").fire("submit");
  const row = ui.get("temporary-downloads").children[0];
  assert.equal(row.children[0].children[0].textContent, "<FCS&literal>");
  assert.equal(row.children[0].children[1].textContent, "name <&>_<FCS&literal>.xml · 1 POU");
  assert.equal(row.children[1].href, `http://localhost:3000${result.files[0].url}`);
  assert.equal(row.children[1].download, "name <&>_<FCS&literal>.xml");
});

test("any invalid download prevents the complete batch from being exposed", async () => {
  const invalidURLs = [
    "https://elsewhere.example/api/output/custom_FCS2.xml",
    "javascript:alert(1)",
    "/api/temporary/ao/generate",
    "/api/output/other.xml",
    "/api/output/custom_FCS2.xml?redirect=elsewhere",
    "/api/output/%ZZ.xml",
  ];
  for (const url of invalidURLs) {
    const result = batchResult(["FCS1", "FCS2"]);
    result.files[1].url = url;
    const ui = await setup((route) => response(route.endsWith("/preview") ? plan() : result));
    await choose(ui);
    await ui.get("temporary-form").fire("submit");
    assert.equal(ui.get("temporary-result").hidden, true, url);
    assert.equal(ui.get("temporary-downloads").children.length, 0, url);
    assert.equal(ui.get("temporary-errors").hidden, false, url);
    assert.equal(ui.get("temporary-file").disabled, false, url);
    assert.deepEqual(ui.events, [], url);
  }
});

test("an empty batch is a visible error rather than a successful generation", async () => {
  const ui = await setup((url) => response(url.endsWith("/preview") ? plan() : { files: [], summary: {} }));
  await choose(ui);
  await ui.get("temporary-form").fire("submit");
  assert.equal(ui.get("temporary-result").hidden, true);
  assert.match(ui.get("temporary-errors").textContent, /не вернул XML-файлы/);
  assert.deepEqual(ui.events, []);
});

test("a parse failure clears the old plan and exposes the row-specific error", async () => {
  let count = 0;
  const ui = await setup(() => ++count === 1 ? response(plan()) : response({ error: "Строка 8: неверный канал" }, false));
  await choose(ui);
  await choose(ui, new File(["invalid"], "invalid.txt"));
  assert.equal(ui.get("temporary-preview").hidden, true);
  assert.equal(ui.get("temporary-rows").children.length, 0);
  assert.equal(ui.get("temporary-generate").disabled, true);
  assert.match(ui.get("temporary-errors").textContent, /Строка 8/);
  const before = ui.calls.length;
  await ui.get("temporary-form").fire("submit");
  assert.equal(ui.calls.length, before);
});

test("oversized files never upload and invalid collapsed context opens for correction", async () => {
  const ui = await setup(() => assert.fail("No upload expected"));
  await choose(ui, { size: 2 * 1024 * 1024 + 1, name: "huge.txt" });
  assert.equal(ui.calls.length, 1);
  assert.match(ui.get("temporary-errors").textContent, /2 МБ/);
  const details = new Element();
  const input = ui.context[0];
  input.details = details;
  await ui.get("temporary-form").fire("invalid", { target: input });
  assert.equal(details.open, true);
});

test("ST restores repeated tags and ranges without changing default FBD holes", async () => {
  const preview = plan();
  Object.assign(preview.groups[0].modules[0].channels[1], { tag: preview.groups[0].modules[0].channels[0].tag, reserve: false, duplicate: true, duplicateOf: "AO_A11 / A11_00 / 0" });
  preview.duplicateCount = 1;
  const ui = await setup(() => response(preview));
  await choose(ui);
  assert.equal(ui.get("temporary-mode").value, "fbd");
  assert.equal(ui.get("temporary-rows").children[1].children[3].textContent, "—");
  assert.equal(pouControl(ui).ids[0].disabled, true);
  await mode(ui, "st", true);
  assert.equal(ui.get("temporary-st-settings").hidden, false);
  assert.equal(ui.get("temporary-rows").children[0].children[0].textContent, "AO_A11_channels3000_D_SC_B01");
  const repeated = ui.get("temporary-rows").children[1];
  assert.equal(repeated.children[3].textContent, "_3107_TV_64101A");
  assert.match(repeated.children[3].title, /^_IO_QU0_1.ValueDINT/);
  assert.equal(repeated.children[4].textContent, "0 … 100");
  assert.equal(repeated.children[6].textContent, "Повтор в карте — включён в ST");
  assert.equal(repeated.classes.has("temporary-duplicate-row"), false);
  assert.match(ui.get("temporary-summary").textContent, /4 присваиваний · 1 повторов — включены в ST/);
  await mode(ui, "fbd");
  assert.equal(ui.get("temporary-rows").children[1].children[3].textContent, "—");
  assert.equal(ui.get("temporary-st-settings").hidden, true);
});

test("ST uploads selected POUs and exact numeric IDs, including zero and per-FCS restarts", async () => {
  const generated = deferred();
  const ui = await setup((url) => url.endsWith("/preview") ? response(multiplePOUs()) : generated.promise);
  const original = await choose(ui);
  await mode(ui, "st", true);
  assert.equal(pouControl(ui, 0).ids[0].value, "0");
  assert.equal(pouControl(ui, 1).ids[0].value, "1");
  assert.equal(pouControl(ui, 2).ids[0].value, "0");
  const excluded = pouControl(ui, 1);
  excluded.selected.checked = false;
  await excluded.selected.fire("change");
  excluded.ids[0].value = "garbage";
  await excluded.ids[0].fire("input");
  assert.equal(excluded.ids[0].disabled, true);
  const generation = ui.get("temporary-form").fire("submit");
  const call = ui.calls.at(-1);
  assert.equal(call.url, "/api/temporary/ao/generate-st");
  assert.deepEqual(JSON.parse(call.options.body.get("st")), { pous: [
    { groupKey: "FCS1/A11", moduleCount: 1, moduleIds: [0] },
    { groupKey: "FCS2/A11", moduleCount: 1, moduleIds: [0] },
  ] });
  assert.equal(await call.options.body.get("file").text(), await original.text());
  assert.equal(ui.get("temporary-mode").disabled, true);
  assert.equal(pouControl(ui, 0).selected.disabled, true);
  assert.equal(pouControl(ui, 0).count.disabled, true);
  assert.equal(pouControl(ui, 0).ids[0].disabled, true);
  assert.equal(plcControl(ui, 0).selected.disabled, true);
  assert.equal(ui.get("temporary-st-select-all").disabled, true);
  assert.equal(ui.get("temporary-st-clear-selection").disabled, true);
  const result = batchResult(["FCS1", "FCS2"], "AO_import_ST");
  result.kind = "st";
  result.summary = { pouCount: 2, assignmentCount: 8, repeatedAssignmentCount: 0, cards: 0, blocks: 0 };
  generated.resolve(response(result));
  await generation;
  assert.equal(ui.get("temporary-result-summary").textContent, "2 POU ST · 8 присваиваний · 0 повторов — включены в ST");
  assert.equal(ui.get("temporary-downloads").children.length, 2);
  assert.equal(pouControl(ui, 0).ids[0].disabled, false);
  assert.equal(pouControl(ui, 1).ids[0].disabled, true);
});

test("ST rejects duplicate IDs within one FCS and reveals the erroneous POU", async () => {
  const ui = await setup(() => response(multiplePOUs()));
  await choose(ui);
  await mode(ui, "st", true);
  const wrong = pouControl(ui, 1);
  wrong.ids[0].value = "0";
  await wrong.ids[0].fire("input");
  const before = ui.calls.length;
  await ui.get("temporary-form").fire("submit");
  assert.equal(ui.calls.length, before);
  assert.match(ui.get("temporary-errors").textContent, /FCS1: ID 0 повторяется у A11_00 и A12_00/);
  assert.equal(wrong.details.open, true);
  assert.equal(wrong.ids[0].focused, true);
});

test("ST extra modules append after highest suffix, preserve manual IDs and survive mode switches", async () => {
  const preview = multiplePOUs();
  const source = preview.groups[0].modules[0];
  source.name = "A11_03";
  const ui = await setup(() => response(preview));
  await choose(ui);
  await mode(ui, "st", true);
  let control = pouControl(ui);
  control.ids[0].value = "41";
  await control.ids[0].fire("input");
  control.count.value = "3";
  await control.count.fire("change");
  control = pouControl(ui);
  assert.deepEqual(control.ids.map((id) => id.dataset.module), ["A11_03", "A11_04", "A11_05"]);
  // FCS1/A12 retains ID1, so extras take the free 0 and 2. Manual ID41 stays untouched.
  assert.deepEqual(control.ids.map((id) => id.value), ["41", "0", "2"]);
  assert.equal(ui.get("temporary-st-extra-note").hidden, false);
  const extra = ui.get("temporary-rows").children[4];
  assert.equal(extra.children[3].textContent, "_FCS1_A11_04_0");
  assert.equal(extra.children[6].textContent, "Резерв");
  await mode(ui, "fbd");
  assert.equal(ui.get("temporary-rows").children.length, 12);
  await mode(ui, "st");
  assert.deepEqual(pouControl(ui).ids.map((id) => id.value), ["41", "0", "2"]);
  assert.equal(ui.get("temporary-rows").children.length, 20);
  control.count.value = "1";
  await control.count.fire("change");
  assert.deepEqual(pouControl(ui).ids.map((id) => id.value), ["41"]);
  assert.equal(ui.get("temporary-st-extra-note").hidden, true);
  control.count.value = "0";
  await control.count.fire("change");
  assert.match(ui.get("temporary-errors").textContent, /Модули из TXT удалять нельзя/);
  assert.equal(pouControl(ui).ids.length, 1);
  const before = ui.calls.length;
  await ui.get("temporary-form").fire("submit");
  assert.equal(ui.calls.length, before);
});

test("ST invalid IDs and empty POU selection never generate; FBD ignores hidden ST errors", async () => {
  const ui = await setup((url) => response(url.endsWith("/preview") ? plan() : batchResult()));
  await choose(ui);
  await mode(ui, "st", true);
  const control = pouControl(ui);
  for (const invalid of ["", "-1", "2.5", "1e2", "2147483648", "NaN"]) {
    control.ids[0].value = invalid;
    await control.ids[0].fire("input");
    const before = ui.calls.length;
    await ui.get("temporary-form").fire("submit");
    assert.equal(ui.calls.length, before, invalid);
    assert.match(ui.get("temporary-errors").textContent, /ID должен быть целым числом/);
  }
  control.selected.checked = false;
  await control.selected.fire("change");
  const before = ui.calls.length;
  await ui.get("temporary-form").fire("submit");
  assert.equal(ui.calls.length, before);
  assert.match(ui.get("temporary-errors").textContent, /Выберите хотя бы одну POU/);
  await mode(ui, "fbd");
  await ui.get("temporary-form").fire("submit");
  assert.equal(ui.calls.at(-1).url, "/api/temporary/ao/generate");
  assert.equal(ui.calls.at(-1).options.body.has("st"), false);
  assert.equal(ui.get("temporary-result").hidden, false);
});

test("selecting another file resets ST settings while retaining the chosen generation mode", async () => {
  const ui = await setup(() => response(plan()));
  await choose(ui);
  await mode(ui, "st", true);
  pouControl(ui).ids[0].value = "41";
  await pouControl(ui).ids[0].fire("input");
  await choose(ui, new File(["another"], "new.txt"));
  assert.equal(ui.get("temporary-mode").value, "st");
  assert.equal(pouControl(ui).ids[0].value, "0");
  assert.equal(pouControl(ui).ids[0].disabled, true);
  assert.equal(pouControl(ui).count.value, "1");
  assert.equal(pouControl(ui).selected.checked, false);
  assert.equal(ui.get("temporary-generate").disabled, true);
  assert.equal(plcControl(ui).selected.indeterminate, false);
  assert.equal(plcControl(ui).selected.checked, false);
});

test("stale ST generation cannot expose files after the source file changes", async () => {
  const generated = deferred();
  const ui = await setup((url) => url.endsWith("/preview") ? response(plan()) : generated.promise);
  await choose(ui);
  await mode(ui, "st", true);
  const generation = ui.get("temporary-form").fire("submit");
  // Real controls are disabled; this also covers programmatic changes and ignored cancellation.
  await choose(ui, new File(["new"], "new.txt"));
  generated.resolve(response(batchResult()));
  await generation;
  assert.equal(ui.get("temporary-result").hidden, true);
  assert.equal(ui.get("temporary-downloads").children.length, 0);
  assert.match(ui.get("temporary-status").textContent, /new.txt/);
  assert.deepEqual(ui.events, []);
  assert.equal(ui.get("temporary-generate").disabled, true);
});

test("ST summary fallback counts retained assignments, never FBD blocks or holes", async () => {
  const preview = plan();
  Object.assign(preview.groups[0].modules[0].channels[1], { tag: preview.groups[0].modules[0].channels[0].tag, duplicate: true, reserve: false });
  const result = batchResult();
  result.summary = {};
  const ui = await setup((url) => response(url.endsWith("/preview") ? preview : result));
  await choose(ui);
  await mode(ui, "st", true);
  await ui.get("temporary-form").fire("submit");
  assert.equal(ui.get("temporary-result-summary").textContent, "1 POU ST · 4 присваиваний · 1 повторов — включены в ST");
});

test("ST profile explains real physical IDs even when FBD profile arrives late, and restores on switching back", async () => {
  const profile = deferred();
  const fbdDescription = "Физические связи не создаются; служебные ID назначает SCADA.";
  const ui = await setup(() => assert.fail("No upload expected"), profile.promise);
  await mode(ui, "st");
  const stDescription = ui.get("temporary-profile-description").textContent;
  assert.match(stDescription, /Профиль ST создаёт присваивания физическим выходам/);
  assert.match(stDescription, /без графических экземпляров AN_v1/);
  assert.match(stDescription, /Физические ID.*реальные адреса.*_IO_QU41_0/);
  assert.match(stDescription, /записываются в ST без замены/);
  profile.resolve(response({ context: nativeContext, description: fbdDescription }));
  await settled();
  assert.equal(ui.get("temporary-profile-description").textContent, stDescription);
  await mode(ui, "fbd");
  assert.equal(ui.get("temporary-profile-description").textContent, fbdDescription);
  await mode(ui, "st");
  assert.equal(ui.get("temporary-profile-description").textContent, stDescription);
});

test("ST requires explicit selection and exposes POU checkboxes outside collapsed settings", async () => {
  const ui = await setup(() => response(multiplePOUs()));
  await choose(ui);
  assert.equal(ui.get("temporary-generate").disabled, false);
  await mode(ui, "st");
  assert.equal(ui.get("temporary-st-pous").children.length, 2);
  assert.equal(ui.get("temporary-generate").disabled, true);
  assert.equal(ui.get("temporary-st-selection-empty").hidden, false);
  assert.equal(ui.get("temporary-st-select-all").disabled, false);
  assert.equal(ui.get("temporary-st-clear-selection").disabled, true);
  assert.equal(ui.get("temporary-rows").children.length, 0);
  assert.equal(ui.get("temporary-st-selection-summary").textContent, "Выбрано 0 из 3 POU · 0 ПЛК → 0 XML");
  for (let index = 0; index < 3; index++) {
    const control = pouControl(ui, index);
    assert.equal(control.selected.checked, false);
    assert.equal(control.selected.disabled, false);
    assert.equal(control.ids[0].disabled, true);
    assert.equal(Boolean(control.details.open), false);
    assert.equal(control.row.children[0].children[0], control.selected);
    assert.notEqual(control.row.children[0], control.details);
  }
  const before = ui.calls.length;
  await ui.get("temporary-form").fire("submit");
  assert.equal(ui.calls.length, before);
  assert.match(ui.get("temporary-errors").textContent, /Выберите хотя бы одну POU/);
});

test("ST PLC checkboxes select only their own POUs; partial/all/clear selections preserve IDs", async () => {
  const ui = await setup(() => response(multiplePOUs()));
  await choose(ui);
  await mode(ui, "st");
  const first = pouControl(ui, 0);
  first.selected.checked = true;
  await first.selected.fire("change");
  first.ids[0].value = "41";
  await first.ids[0].fire("input");
  assert.equal(plcControl(ui, 0).selected.indeterminate, true);
  assert.equal(plcControl(ui, 0).selected.checked, false);
  assert.equal(plcControl(ui, 0).count.textContent, "1/2 POU");
  assert.equal(plcControl(ui, 1).selected.checked, false);
  assert.equal(pouControl(ui, 2).selected.checked, false);
  assert.equal(ui.get("temporary-generate").disabled, false);
  assert.equal(ui.get("temporary-rows").children.length, 4);
  plcControl(ui, 0).selected.checked = true;
  await plcControl(ui, 0).selected.fire("change");
  assert.equal(plcControl(ui, 0).selected.indeterminate, false);
  assert.equal(pouControl(ui, 1).selected.checked, true);
  assert.equal(pouControl(ui, 2).selected.checked, false);
  assert.equal(ui.get("temporary-st-selection-summary").textContent, "Выбрано 2 из 3 POU · 1 ПЛК → 1 XML");
  await mode(ui, "fbd");
  assert.equal(ui.get("temporary-rows").children.length, 12);
  await mode(ui, "st");
  assert.equal(ui.get("temporary-rows").children.length, 8);
  assert.equal(first.ids[0].value, "41");
  await ui.get("temporary-st-select-all").fire("click");
  assert.equal(ui.get("temporary-st-selection-summary").textContent, "Выбрано 3 из 3 POU · 2 ПЛК → 2 XML");
  assert.equal(plcControl(ui, 1).selected.checked, true);
  assert.equal(ui.get("temporary-st-select-all").disabled, true);
  await ui.get("temporary-st-clear-selection").fire("click");
  assert.equal(ui.get("temporary-generate").disabled, true);
  assert.equal(plcControl(ui, 0).selected.checked, false);
  assert.equal(plcControl(ui, 0).selected.indeterminate, false);
  assert.equal(first.ids[0].value, "41");
  assert.equal(first.ids[0].disabled, true);
});

test("ST selecting one PLC uploads only its group keys even with equal POU names elsewhere", async () => {
  const result = batchResult(["FCS2"], "AO_import_ST");
  result.kind = "st";
  result.summary = { pouCount: 1, assignmentCount: 4, repeatedAssignmentCount: 0 };
  const ui = await setup((url) => response(url.endsWith("/preview") ? multiplePOUs() : result));
  await choose(ui);
  await mode(ui, "st");
  plcControl(ui, 1).selected.checked = true;
  await plcControl(ui, 1).selected.fire("change");
  assert.equal(pouControl(ui, 0).selected.checked, false);
  assert.equal(pouControl(ui, 2).selected.checked, true);
  assert.equal(ui.get("temporary-rows").children[0].children[0].textContent, "AO_A11_channelsFCS2");
  await ui.get("temporary-form").fire("submit");
  assert.deepEqual(JSON.parse(ui.calls.at(-1).options.body.get("st")), {
    pous: [{ groupKey: "FCS2/A11", moduleCount: 1, moduleIds: [0] }],
  });
  assert.equal(ui.get("temporary-downloads").children.length, 1);
  assert.match(ui.get("temporary-downloads").textContent, /FCS2/);
  assert.doesNotMatch(ui.get("temporary-downloads").textContent, /FCS1/);
  await ui.get("temporary-st-clear-selection").fire("click");
  assert.equal(ui.get("temporary-result").hidden, true);
});

test("ST subset distinguishes source-map duplicates from repeats among selected POUs", async () => {
  const preview = multiplePOUs();
  Object.assign(preview.groups[1].modules[0].channels[0], { tag: preview.groups[0].modules[0].channels[0].tag, duplicate: true });
  const ui = await setup(() => response(preview));
  await choose(ui);
  await mode(ui, "st");
  pouControl(ui, 1).selected.checked = true;
  await pouControl(ui, 1).selected.fire("change");
  assert.match(ui.get("temporary-summary").textContent, /4 присваиваний · 0 повторов/);
  const row = ui.get("temporary-rows").children[0];
  assert.equal(row.children[3].textContent, preview.groups[0].modules[0].channels[0].tag);
  assert.equal(row.children[6].textContent, "Повтор в карте — включён в ST");
});

function diagnosticPLC(ui, index = 0) {
  const section = ui.get("temporary-diagnostic-plcs").children[index];
  return { section, selected: section.children[0].children[0], count: section.children[0].children[2], name: section.children[1].children[1], types: section.children[2], frames: section.children[3].children[1] };
}

function ioPlan() {
  const module = (name, type, capacity, tag) => ({
    name, type, capacity, rack: name.split("_")[0], slot: Number(name.split("_")[1]),
    channels: Array.from({ length: capacity }, (_, channel) => ({
      channel, tag: channel ? "" : tag, sourceTag: channel ? "" : tag.replace(/^_/, ""),
      objectType: type === "AI16H" ? "AD3_v2" : type === "AOC4H" ? "AN_v1" : "D32V_v1",
      reserve: channel !== 0, redundant: false, sourceRow: channel ? 0 : 20,
    })),
  });
  return {
    sheetName: "IO", rowCount: 99, moduleCount: 5, signalCount: 88, warnings: [],
    controllers: [
      { key: "FCS7:3000-D-SC-B07", sourceFcs: "FCS7", cabinet: "3000-D-SC-B07", name: "3000_D_SC_B07_1",
        racks: [{ name: "A11", panel: "front", order: 0 }, { name: "A12", panel: "back", order: 0 }],
        modules: [module("A11_00", "AOC4H", 4, "_AO"), module("A11_01", "AI16H", 16, "_AI_MAIN"), module("A12_00", "DI32", 32, "_DI"), module("A12_01", "DO32P", 32, "_DO")] },
      { key: "FCS8:3000-D-SC-B07", sourceFcs: "FCS8", cabinet: "3000-D-SC-B07", name: "3000_D_SC_B07_2",
        racks: [{ name: "A11", panel: "front", order: 0 }], modules: [module("A11_00", "AOC4H", 4, "_AO")] },
    ],
  };
}

async function chooseIO(ui, selectAll = false, file = new File(["xlsx source"], "Full_IO.xlsx")) {
  await mode(ui, "diagnostic");
  await choose(ui, file);
  if (selectAll) await ui.get("temporary-diagnostic-select-all").fire("click");
  return file;
}

function diagnosticResult(fcsNames, frameCounts) {
  const result = batchResult(fcsNames, "custom_diagnostic");
  result.kind = "diagnostic";
  result.files.forEach((file, index) => {
    file.summary = { frameCount: frameCounts[index], signalCount: 84, graphics: 100, cards: 88 };
  });
  const frameCount = frameCounts.reduce((sum, count) => sum + count, 0);
  result.summary = { frameCount, signalCount: 88, graphics: 120, cards: 100 };
  return result;
}

test("IO diagnostics require explicit PLC selection and show complete module and page inventory", async () => {
  const ui = await setup(() => response(ioPlan()));
  await chooseIO(ui);
  assert.equal(ui.calls.at(-1).url, "/api/temporary/diagnostic/preview");
  assert.equal(ui.get("temporary-diagnostic-settings").hidden, false);
  assert.equal(ui.get("temporary-st-settings").hidden, true);
  assert.equal(ui.get("temporary-group-heading").textContent, "Кадр / ПЛК");
  assert.equal(ui.get("temporary-scale-heading").textContent, "Тип модуля");
  assert.equal(ui.get("temporary-source-heading").textContent, "Строка Excel");
  assert.equal(ui.get("temporary-generate").disabled, true);
  assert.equal(ui.get("temporary-rows").children.length, 0);
  assert.equal(ui.get("temporary-diagnostic-plcs").children.length, 2);
  const first = diagnosticPLC(ui);
  assert.equal(first.count.textContent, "5 кадров");
  assert.equal(first.types.textContent, "AI 1 · AO 1 · DI 1 · DO 1");
  assert.equal(first.name.value, "3000_D_SC_B07_1");
  assert.equal(first.name.disabled, true);
  assert.match(first.frames.textContent, /Передняя панель.*AO_B07_1_A11_00_AOC4H.*AI_B07_1_A11_01_AI16H.*Задняя панель.*A12_00 · DI32 \(панель\)/);
  const before = ui.calls.length;
  await ui.get("temporary-form").fire("submit");
  assert.equal(ui.calls.length, before);
  assert.match(ui.get("temporary-errors").textContent, /Выберите хотя бы один ПЛК/);
  first.selected.checked = true;
  await first.selected.fire("change");
  assert.equal(first.name.disabled, false);
  assert.equal(ui.get("temporary-rows").children.length, 84);
  assert.match(ui.get("temporary-summary").textContent, /^1 ПЛК \/ XML · 5 кадров · 4 модулей · 84 каналов · AI 1 · AO 1 · DI 1 · DO 1/);
  assert.equal(diagnosticPLC(ui, 1).selected.checked, false);
  await ui.get("temporary-diagnostic-select-all").fire("click");
  assert.equal(ui.get("temporary-rows").children.length, 88);
  assert.match(ui.get("temporary-diagnostic-selection-summary").textContent, /2 из 2 ПЛК · 9 кадров → 2 XML/);
  assert.equal(diagnosticPLC(ui, 1).count.textContent, "4 кадров");
  assert.match(diagnosticPLC(ui, 1).frames.textContent, /Задняя панельПустая панель · сохраняется в XML/);
  await ui.get("temporary-diagnostic-clear-selection").fire("click");
  assert.equal(ui.get("temporary-generate").disabled, true);
  assert.equal(first.name.disabled, true);
  assert.equal(ui.get("temporary-rows").children.length, 0);
});

test("diagnostic rows use native module types, Excel rows, repeated tags, and edited PLC names safely", async () => {
  const preview = ioPlan();
  const controller = preview.controllers[0];
  controller.modules[0].channels[0].tag = "<TAG&literal>";
  controller.modules[1].channels[0].tag = "<TAG&literal>";
  controller.modules[1].channels[0].redundant = true;
  const ui = await setup(() => response(preview));
  await chooseIO(ui, true);
  const first = diagnosticPLC(ui);
  first.name.value = "_CUSTOM_PLC";
  await first.name.fire("input");
  const rows = ui.get("temporary-rows").children;
  assert.equal(rows[0].children[0].textContent, "AO__CUSTOM_PLC_A11_00_AOC4H_CUSTOM_PLC / Передняя панель / A11");
  assert.equal(rows[0].children[3].textContent, "<TAG&literal>");
  assert.equal(rows[0].children[3].children.length, 0);
  assert.equal(rows[0].children[4].textContent, "AOC4H");
  assert.equal(rows[0].children[5].textContent, "20");
  assert.equal(rows[1].children[3].textContent, "__CUSTOM_PLC_A11_00_1");
  assert.equal(rows[4].children[3].textContent, "<TAG&literal>");
  assert.equal(rows[4].children[6].textContent, "Резервированное подключение");
  assert.equal(rows[4].classes.has("temporary-duplicate-row"), false);
  assert.equal(rows[20].children[0].textContent, "Задняя панель · A12_00_CUSTOM_PLC / Задняя панель / A12");
  assert.equal(rows[84].children[0].textContent, "AO_B07_2_A11_00_AOC4H3000_D_SC_B07_2 / Передняя панель / A11");
  assert.match(first.frames.textContent, /AI__CUSTOM_PLC_A11_01_AI16H/);
  ui.get("temporary-search").value = "AI__CUSTOM_PLC_A11_01_AI16H";
  await ui.get("temporary-search").fire("input");
  await new Promise((resolve) => setTimeout(resolve, 150));
  assert.equal(ui.get("temporary-rows").children.length, 16);
});

test("diagnostic generation sends exact workbook, selected controller keys and edited names with narrow context", async () => {
  const generated = deferred();
  const ui = await setup((url) => url.endsWith("/preview") ? response(ioPlan()) : generated.promise);
  const file = await chooseIO(ui);
  ui.get("temporary-filename").value = " custom ";
  const selected = diagnosticPLC(ui);
  selected.selected.checked = true;
  await selected.selected.fire("change");
  selected.name.value = " CUSTOM_PLC ";
  await selected.name.fire("input");
  for (const input of ui.context) {
    const relevant = ["version", "project"].includes(input.dataset.temporaryContext);
    assert.equal(input.disabled, !relevant);
    assert.equal(input.label.hidden, !relevant);
  }
  ui.get("temporary-diagnostic-resource").value = " 2 ";
  const generation = ui.get("temporary-form").fire("submit");
  const request = ui.calls.at(-1);
  assert.equal(request.url, "/api/temporary/diagnostic/generate");
  assert.equal(await request.options.body.get("file").text(), await file.text());
  assert.equal(request.options.body.get("fileName"), "custom");
  assert.deepEqual(JSON.parse(request.options.body.get("context")), { version: "29", project: "native-project", resourceNumber: "2" });
  assert.deepEqual(JSON.parse(request.options.body.get("diagnostic")), { controllers: [{ key: "FCS7:3000-D-SC-B07", name: "CUSTOM_PLC" }] });
  assert.equal(request.options.body.get("st"), null);
  assert.equal(selected.selected.disabled, true);
  assert.equal(selected.name.disabled, true);
  assert.equal(ui.get("temporary-diagnostic-resource").disabled, true);
  assert.equal(ui.get("temporary-diagnostic-clear-selection").disabled, true);
  generated.resolve(response(diagnosticResult(["CUSTOM_PLC"], [5])));
  await generation;
  assert.equal(ui.get("temporary-result-summary").textContent, "5 кадров · 88 каналов · 120 графических элементов · 100 карточек привязки");
  assert.equal(ui.get("temporary-downloads").children[0].children[0].children[1].textContent, "custom_diagnostic_CUSTOM_PLC.xml · 5 кадров");
  assert.equal(ui.get("temporary-result").hidden, false);
  assert.equal(selected.name.disabled, false);
  assert.equal(ui.context.find((input) => input.dataset.temporaryContext === "pouNumber").disabled, true);
  assert.deepEqual(ui.events, ["schemegen:outputs-changed"]);
  await ui.get("temporary-diagnostic-select-all").fire("click");
  await ui.get("temporary-form").fire("submit");
  assert.deepEqual(JSON.parse(ui.calls.at(-1).options.body.get("diagnostic")), { controllers: [
    { key: "FCS7:3000-D-SC-B07", name: "CUSTOM_PLC" }, { key: "FCS8:3000-D-SC-B07", name: "3000_D_SC_B07_2" },
  ] });
});

test("AI/AO names inferred from IO are visibly flagged and searchable without flagging references or DI/DO", async () => {
  const preview = ioPlan();
  const modules = preview.controllers[0].modules;
  modules[0].channels[0].bindingSource = "io-rule";
  modules[1].channels[0].bindingSource = "reference";
  modules[2].channels[0].bindingSource = "io-rule";
  modules[3].channels[0].bindingSource = "io-rule";
  preview.controllers[1].modules[0].channels[0].bindingSource = "explicit";
  const ui = await setup(() => response(preview));
  await chooseIO(ui, true);
  const rows = ui.get("temporary-rows").children;
  assert.equal(rows[0].children[6].textContent, "Имя по IO · проверить");
  assert.equal(rows[0].children[6].children[0].className, "temporary-inferred-badge");
  assert.match(rows[0].children[3].title, /не подтверждено перекладкой AI\/AO/);
  assert.equal(rows[0].children[5].textContent, "20");
  assert.equal(rows[1].children[6].textContent, "Резерв");
  for (const index of [4, 20, 52, 84]) assert.equal(rows[index].children[6].textContent, "Сигнал");
  ui.get("temporary-search").value = "проверить";
  await ui.get("temporary-search").fire("input");
  await new Promise((resolve) => setTimeout(resolve, 150));
  assert.equal(ui.get("temporary-rows").children.length, 1);
  assert.equal(ui.get("temporary-rows").children[0].children[3].textContent, "_AO");
});

test("diagnostic PLC names reject invalid and case-insensitive duplicate identifiers only among selected PLCs", async () => {
  const ui = await setup((url) => response(url.endsWith("/preview") ? ioPlan() : diagnosticResult(["VALID"], [5])));
  await chooseIO(ui, true);
  const first = diagnosticPLC(ui);
  const second = diagnosticPLC(ui, 1);
  assert.equal(first.name.maxLength, 100);
  for (const value of ["", "3000-PLC", "Русский", "A/B", "A".repeat(101)]) {
    first.name.value = value;
    const before = ui.calls.length;
    await ui.get("temporary-form").fire("submit");
    assert.equal(ui.calls.length, before, value);
    assert.equal(first.name.focused, true);
  }
  first.name.value = "CUSTOM";
  second.name.value = "custom";
  await ui.get("temporary-form").fire("submit");
  assert.match(ui.get("temporary-errors").textContent, /повторяется/);
  assert.equal(second.name.focused, true);
  second.selected.checked = false;
  await second.selected.fire("change");
  second.name.value = "invalid name";
  await ui.get("temporary-form").fire("submit");
  assert.equal(ui.calls.at(-1).url, "/api/temporary/diagnostic/generate");
  assert.deepEqual(JSON.parse(ui.calls.at(-1).options.body.get("diagnostic")), { controllers: [{ key: "FCS7:3000-D-SC-B07", name: "CUSTOM" }] });
});

test("crossing TXT and XLSX modes clears incompatible files and stale previews; FBD/ST remains intact", async () => {
  const ui = await setup((url) => response(url.includes("/diagnostic/") ? ioPlan() : multiplePOUs()));
  await choose(ui);
  await mode(ui, "st", true);
  pouControl(ui).ids[0].value = "41";
  await pouControl(ui).ids[0].fire("input");
  await mode(ui, "fbd");
  await mode(ui, "st");
  assert.equal(pouControl(ui).ids[0].value, "41");
  await mode(ui, "diagnostic");
  assert.equal(ui.get("temporary-preview").hidden, true);
  assert.equal(ui.get("temporary-file").value, "");
  assert.equal(ui.get("temporary-file").accept.includes(".xlsx"), true);
  assert.equal(ui.get("temporary-generate").disabled, true);
  assert.equal(ui.get("temporary-st-pous").children.length, 0);
  assert.equal(ui.get("temporary-filename").value, "");
  await choose(ui, new File(["xlsx"], "IO.xlsx"));
  await ui.get("temporary-diagnostic-select-all").fire("click");
  assert.equal(ui.get("temporary-rows").children.length, 88);
  await mode(ui, "st");
  assert.equal(ui.get("temporary-preview").hidden, true);
  assert.equal(ui.get("temporary-diagnostic-plcs").children.length, 0);
  assert.equal(ui.get("temporary-diagnostic-resource").disabled, true);
  assert.equal(ui.context.every((input) => !input.disabled && !input.label.hidden), true);
  assert.equal(ui.get("temporary-file").accept.includes(".txt"), true);
});

test("file type and per-mode upload limits fail before network activity", async () => {
  const ui = await setup(() => response(ioPlan()));
  await mode(ui, "diagnostic");
  for (const file of [new File(["bad"], "map.txt"), new File(["bad"], "old.xls"), { name: "IO.xlsx", size: 16 * 1024 * 1024 + 1 }]) {
    const before = ui.calls.length;
    await choose(ui, file);
    assert.equal(ui.calls.length, before);
    assert.equal(ui.get("temporary-generate").disabled, true);
  }
  assert.match(ui.get("temporary-errors").textContent, /16 МБ/);
  const medium = new File([new Uint8Array(3 * 1024 * 1024)], "IO.XLSX");
  await choose(ui, medium);
  assert.equal(ui.calls.at(-1).url, "/api/temporary/diagnostic/preview");
  await mode(ui, "fbd");
  const before = ui.calls.length;
  await choose(ui, medium);
  assert.equal(ui.calls.length, before);
  assert.match(ui.get("temporary-errors").textContent, /txt.*tsv/);
});

test("a late TXT preview cannot overwrite a newer IO selection even when abort is ignored", async () => {
  const old = deferred();
  const ui = await setup((url) => url === "/api/temporary/ao/preview" ? old.promise : response(ioPlan()));
  const pending = choose(ui);
  await chooseIO(ui, true);
  old.resolve(response(plan("_OLD")));
  await pending;
  assert.equal(ui.calls[1].options.signal.aborted, true);
  assert.match(ui.get("temporary-status").textContent, /Full_IO.xlsx/);
  assert.equal(ui.get("temporary-rows").children.length, 88);
  assert.equal(diagnosticPLC(ui).selected.checked, true);
});

test("invalid diagnostic resource numbers do not generate and hidden settings never block FBD", async () => {
  const ui = await setup((url) => response(url.endsWith("/preview") ? url.includes("/diagnostic/") ? ioPlan() : plan() : batchResult()));
  await chooseIO(ui, true);
  const input = ui.get("temporary-diagnostic-resource");
  input.details = new Element();
  for (const value of ["", "0", "-1", "1.5", "1e2", "2147483648"]) {
    input.value = value;
    const before = ui.calls.length;
    await ui.get("temporary-form").fire("submit");
    assert.equal(ui.calls.length, before, value);
    assert.equal(input.details.open, true);
    assert.equal(input.focused, true);
    assert.match(ui.get("temporary-errors").textContent, /Номер ресурса/);
  }
  await mode(ui, "fbd");
  await choose(ui);
  await ui.get("temporary-form").fire("submit");
  assert.equal(ui.calls.at(-1).url, "/api/temporary/ao/generate");
  assert.equal(ui.calls.at(-1).options.body.get("diagnostic"), null);
  assert.equal(JSON.parse(ui.calls.at(-1).options.body.get("context")).resourceNumber, undefined);
  assert.equal(input.disabled, true);
});

test("stale diagnostic generation cannot expose an old batch after workbook changes", async () => {
  const generated = deferred();
  const ui = await setup((url) => url.endsWith("/preview") ? response(ioPlan()) : generated.promise);
  await chooseIO(ui, true);
  const generation = ui.get("temporary-form").fire("submit");
  await mode(ui, "fbd");
  assert.equal(ui.get("temporary-mode").value, "diagnostic");
  await choose(ui, new File(["new"], "new.xlsx"));
  generated.resolve(response(diagnosticResult(["3000_D_SC_B07_1"], [5])));
  await generation;
  assert.equal(ui.get("temporary-result").hidden, true);
  assert.equal(ui.get("temporary-downloads").children.length, 0);
  assert.deepEqual(ui.events, []);
  assert.equal(ui.get("temporary-generate").disabled, true);
  assert.equal(diagnosticPLC(ui).selected.checked, false);
});

test("diagnostic downloads list PLCs and use hierarchy totals when summary fields are absent", async () => {
  const result = diagnosticResult(["3000_D_SC_B07_1", "3000_D_SC_B07_2"], [5, 4]);
  result.summary = {};
  const ui = await setup((url) => response(url.endsWith("/preview") ? ioPlan() : result));
  await chooseIO(ui, true);
  await ui.get("temporary-form").fire("submit");
  assert.equal(ui.get("temporary-downloads").children.length, 2);
  assert.equal(ui.get("temporary-result-summary").textContent, "9 кадров · 88 каналов · — графических элементов · — карточек привязки");
  assert.match(ui.get("temporary-status").textContent, /панель оператора/);
});
