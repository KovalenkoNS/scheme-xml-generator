const { test } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const { File } = require("node:buffer");

// A small DOM boundary exercises uploads, selection and async state transitions.
class Element {
  constructor() { this.listeners = new Map(); this.attributes = new Map(); this.children = []; this.value = ""; this.files = []; }
  set textContent(value) { this.text = String(value); this.children = []; }
  get textContent() { return (this.text || "") + this.children.map((item) => item.textContent).join(""); }
  setAttribute(key, value) { this.attributes.set(key, value); }
  getAttribute(key) { return this.attributes.get(key); }
  append(...items) { this.children.push(...items.flatMap((item) => item.fragment ? item.children : [item])); }
  replaceChildren(...items) { this.text = ""; this.children = []; this.append(...items); }
  addEventListener(name, callback) { this.listeners.set(name, [...(this.listeners.get(name) || []), callback]); }
  async fire(name, properties = {}) {
    await Promise.all((this.listeners.get(name) || []).map((fn) => fn({ target: this, preventDefault() {}, ...properties })));
  }
  focus() { this.focused = true; }
}

const response = (data, ok = true) => ({ ok, status: ok ? 200 : 400, json: async () => data });
const deferred = () => { let resolve; const promise = new Promise((done) => { resolve = done; }); return { promise, resolve }; };
function plan(names = ["FCS1", "FCS2"]) {
  return { controllers: names.map((name) => ({ key: `IO/${name}`, name, sourceFcs: name, cabinet: "Шкаф",
    moduleCount: 4, reserveCount: 3, objectCount: 7, types: { ai: 1, ao: 1, di: 1, do: 1 } })), warnings: [] };
}
function generated(names = ["RENAMED"]) {
  return { files: names.map((fcs) => ({ fcs, fileName: `TechObjects_${fcs}.xls`, url: `/api/output/TechObjects_${fcs}.xls`,
    summary: { moduleCount: 4, reserveCount: 3, objectCount: 7 } })), summary: { moduleCount: names.length * 4, reserveCount: names.length * 3, objectCount: names.length * 7 }, warnings: [] };
}

function setup(handler) {
  const elements = new Map();
  const get = (id) => {
    if (!elements.has(id)) elements.set(id, new Element());
    return elements.get(id);
  };
  const ui = (id) => get(`techobjects-${id}`);
  ui("filename").value = "TechObjects";
  ui("resource").value = "1";
  const calls = [];
  const events = [];
  const sandbox = {
    document: { getElementById: get, createElement: () => new Element(), createDocumentFragment: () => Object.assign(new Element(), { fragment: true }) },
    window: { location: { href: "http://localhost:3000/", origin: "http://localhost:3000" }, dispatchEvent: (event) => events.push(event.type) },
    fetch: (url, options) => { calls.push({ url, options }); return handler(url, options); },
    FormData, AbortController, URL, Event,
  };
  vm.runInNewContext(fs.readFileSync(path.join(__dirname, "techobjects.js"), "utf8"), sandbox);
  return { get: ui, calls, events };
}

async function choose(ui, file = new File(["source"], "IO.xlsx")) {
  ui.get("file").files = file ? [file] : [];
  await ui.get("file").fire("change");
  return file;
}
function control(ui, index = 0) {
  const section = ui.get("plcs").children[index];
  return { selected: section.children[0].children[0], name: section.children[2].children[1], section };
}

test("the XLS tool starts independently and requires explicit PLC selection", async () => {
  const ui = setup(() => response(plan()));
  assert.equal(ui.calls.length, 0);
  assert.equal(ui.get("generate").disabled, true);
  await choose(ui);
  assert.equal(ui.calls[0].url, "/api/techobjects/preview");
  assert.equal(ui.get("preview").hidden, false);
  assert.equal(ui.get("plcs").children.length, 2);
  assert.equal(control(ui).name.disabled, true);
  assert.equal(ui.get("generate").disabled, true);
  assert.match(ui.get("summary").textContent, /8 модулей · 6 резервов AI\/AO · 14 объектов/);
  assert.match(control(ui).section.textContent, /AI: 1 · AO: 1 · DI: 1 · DO: 1/);
  await ui.get("select-all").fire("click");
  assert.equal(ui.get("generate").disabled, false);
  assert.equal(ui.get("select-all").disabled, true);
  assert.equal(control(ui).name.disabled, false);
  await ui.get("clear-selection").fire("click");
  assert.equal(ui.get("generate").disabled, true);
  assert.equal(ui.get("clear-selection").disabled, true);
});

test("a stale preview cannot replace newer controllers or clear pending preview state", async () => {
  const first = deferred();
  const second = deferred();
  let count = 0;
  const ui = setup(() => ++count === 1 ? first.promise : second.promise);
  const old = choose(ui, new File(["old"], "old.xlsx"));
  const latest = choose(ui, new File(["new"], "new.xlsx"));
  first.resolve(response(plan(["OLD"])));
  await old;
  assert.equal(ui.calls[0].options.signal.aborted, true);
  assert.equal(ui.get("preview").getAttribute("aria-busy"), "true");
  assert.equal(ui.get("plcs").children.length, 0);
  second.resolve(response(plan(["NEW"])));
  await latest;
  assert.equal(control(ui).name.value, "NEW");
  assert.match(ui.get("status").textContent, /new\.xlsx/);
});

test("clearing the upload invalidates an in-flight preview", async () => {
  const pending = deferred();
  const ui = setup(() => pending.promise);
  const upload = choose(ui);
  await choose(ui, null);
  pending.resolve(response(plan()));
  await upload;
  assert.equal(ui.get("plcs").children.length, 0);
  assert.equal(ui.get("preview").hidden, true);
  assert.equal(ui.get("generate").disabled, true);
});

test("generation sends only selected PLCs, exact source bytes and integer resource, with controls locked", async () => {
  const pending = deferred();
  const ui = setup((url) => url.endsWith("/preview") ? response(plan()) : pending.promise);
  const file = await choose(ui, new File(["exact\r\nIO bytes"], "source.xlsx"));
  const plc = control(ui);
  plc.selected.checked = true;
  await plc.selected.fire("change");
  plc.name.value = " RENAMED ";
  ui.get("resource").value = "7";
  ui.get("filename").value = " TechObjects.xls ";
  const operation = ui.get("form").fire("submit");
  for (const name of ["file", "filename", "resource", "generate", "select-all", "clear-selection"]) assert.equal(ui.get(name).disabled, true, name);
  assert.equal(plc.name.disabled, true);
  assert.equal(plc.selected.disabled, true);
  assert.equal(control(ui, 1).selected.disabled, true);
  assert.equal(ui.calls.at(-1).url, "/api/techobjects/generate");
  const body = ui.calls.at(-1).options.body;
  assert.equal(await body.get("file").text(), await file.text());
  assert.equal(body.get("fileName"), "TechObjects");
  assert.deepEqual(JSON.parse(body.get("objects")), { controllers: [{ key: "IO/FCS1", name: "RENAMED" }], resourceNumber: 7 });
  assert.equal(body.get("context"), null);
  assert.equal(body.get("diagnostic"), null);
  await ui.get("form").fire("submit");
  assert.equal(ui.calls.length, 2);
  pending.resolve(response(generated()));
  await operation;
  assert.equal(ui.get("result").hidden, false);
  const link = ui.get("downloads").children[0].children[1];
  assert.equal(link.href, "http://localhost:3000/api/output/TechObjects_RENAMED.xls");
  assert.equal(link.download, "TechObjects_RENAMED.xls");
  assert.equal(ui.get("file").disabled, false);
  assert.equal(plc.name.disabled, false);
  assert.match(ui.get("result-summary").textContent, /4 модулей · 3 резервов AI\/AO · 7 объектов/);
  assert.deepEqual(ui.events, ["schemegen:outputs-changed"]);
});

test("invalid source files are rejected before upload", async () => {
  const ui = setup(() => assert.fail("No upload expected"));
  for (const file of [{ name: "_diag.xls", size: 200 }, { name: "IO.xlsx", size: 17 * 1024 * 1024 }, { name: "IO.xlsx", size: 0 }]) {
    await choose(ui, file);
    assert.equal(ui.get("errors").hidden, false);
    assert.equal(ui.get("generate").disabled, true);
  }
  assert.equal(ui.calls.length, 0);
});

test("duplicate PLC names and invalid resource numbers prevent generation", async () => {
  const ui = setup(() => response(plan()));
  await choose(ui);
  await ui.get("select-all").fire("click");
  control(ui, 1).name.value = "fcs1";
  await ui.get("form").fire("submit");
  assert.match(ui.get("errors").textContent, /повторяется/);
  assert.equal(control(ui, 1).name.focused, true);
  control(ui, 1).name.value = "FCS2";
  for (const value of ["0", "-1", "1.5", "1e2", "2147483648", ""]) {
    ui.get("resource").value = value;
    await ui.get("form").fire("submit");
    assert.match(ui.get("errors").textContent, /Номер ресурса/);
  }
  assert.equal(ui.calls.length, 1);
});

test("generation errors unlock controls and preserve the selected PLCs for retry", async () => {
  const ui = setup((url) => url.endsWith("/preview") ? response(plan()) : response({ error: "Недоступна папка результатов" }, false));
  await choose(ui);
  await ui.get("select-all").fire("click");
  await ui.get("form").fire("submit");
  assert.equal(ui.get("generate").disabled, false);
  assert.equal(ui.get("file").disabled, false);
  assert.equal(ui.get("resource").disabled, false);
  assert.equal(control(ui).selected.checked, true);
  assert.equal(ui.get("result").hidden, true);
  assert.match(ui.get("errors").textContent, /Недоступна папка результатов/);
  assert.deepEqual(ui.events, []);
});

test("download responses must refer to local XLS output files", async () => {
  const result = generated();
  result.files[0].url = "https://example.com/api/output/TechObjects_RENAMED.xls";
  const ui = setup((url) => url.endsWith("/preview") ? response(plan()) : response(result));
  await choose(ui);
  await ui.get("select-all").fire("click");
  await ui.get("form").fire("submit");
  assert.equal(ui.get("result").hidden, true);
  assert.match(ui.get("errors").textContent, /недопустимый адрес/);
  assert.deepEqual(ui.events, []);
});
