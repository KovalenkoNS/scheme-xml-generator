const { test } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const { File } = require("node:buffer");

class Element {
  constructor() { this.listeners = new Map(); this.attributes = new Map(); this.children = []; this.dataset = {}; this.value = ""; this.files = []; }
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

const nativeContext = { version: "29", project: "SOGO", controllerTypeName: "TENIX-CPU850", controllerId: "189311", resourceId: "647", groupId: "19912", pouNumber: "4" };
const response = (data, ok = true) => ({ ok, status: ok ? 200 : 400, json: async () => data });
const deferred = () => { let resolve; const promise = new Promise((done) => { resolve = done; }); return { promise, resolve }; };
const settled = () => new Promise((resolve) => setImmediate(resolve));
function plan(scs = ["SCS1", "SCS2"]) {
  return { groups: scs.map((name, index) => ({ key: `${name}/${index}`, scs: name, kind: index ? "DO" : "AI", prefix: index ? "A3" : "A1", pouName: index ? "DO_A3" : "AI_A1",
    modules: [{ name: index ? "A3-00" : "A1-00", type: index ? "DO32P" : "AI16H", objectType: index ? "D32V" : "AD3_v2",
      channels: [index ? 17 : 0, index ? 19 : 3].map((channel) => ({ channel, tag: `_SOURCE_${channel}`, reserve: false, sourceRow: channel + 2 })) }] })), warnings: [] };
}
function generated(names = ["SCS1"]) {
  return { files: names.map((fcs) => ({ fcs, fileName: `SKZ_${fcs}.xml`, url: `/api/output/SKZ_${fcs}.xml`, summary: { pouCount: 1, signalCount: 2, assignmentCount: 2 } })),
    summary: { pouCount: names.length, signalCount: names.length * 2, assignmentCount: names.length * 2 }, warnings: [] };
}

async function setup(handler, profileHandler = () => response({ context: nativeContext, description: "Native PLC 850" })) {
  const elements = new Map();
  const get = (id) => { if (!elements.has(id)) elements.set(id, new Element()); return elements.get(id); };
  const ui = (id) => get(`skz-${id}`);
  const context = Object.keys(nativeContext).map((key) => { const input = new Element(); input.dataset.skzContext = key; return input; });
  ui("filename").value = "SKZ_import";
  const calls = [], events = [];
  vm.runInNewContext(fs.readFileSync(path.join(__dirname, "skz.js"), "utf8"), {
    document: { getElementById: get, querySelectorAll: () => context, createElement: () => new Element(), createDocumentFragment: () => Object.assign(new Element(), { fragment: true }) },
    window: { location: { href: "http://localhost:3000/", origin: "http://localhost:3000" }, dispatchEvent: (event) => events.push(event.type) },
    fetch: (url, options) => { calls.push({ url, options }); return url.endsWith("/profile") ? profileHandler() : handler(url, options); },
    FormData, AbortController, URL, Event,
  });
  await settled();
  return { get: ui, context, calls, events };
}

async function choose(ui, file = new File(["source"], "AI.xlsx")) {
  ui.get("file").files = file ? [file] : [];
  await ui.get("file").fire("change");
  return file;
}
function control(ui, index = 0) {
  const section = ui.get("groups").children[index];
  return { selected: section.children[0].children[0], count: section.children[2].children[1], panel: section.children[3], ids: section.children[3].children[1].children.map((label) => label.children[1]) };
}
async function mode(ui, value) { ui.get("mode").value = value; await ui.get("mode").fire("change"); }
async function physicalID(ui, value, index = 0) { const input = control(ui, index).ids[0]; input.value = value; await input.fire("input"); }
async function moduleCount(ui, value, index = 0) { const input = control(ui, index).count; input.value = String(value); await input.fire("input"); }

test("ST requires selected groups and explicit ModuleID, retaining sparse channel numbers", async () => {
  const ui = await setup(() => response(plan()));
  assert.equal(ui.get("mode").value, "st");
  assert.deepEqual(ui.context.map((input) => input.value), Object.values(nativeContext));
  await choose(ui);
  assert.equal(control(ui).ids[0].value, "");
  assert.equal(control(ui).ids[0].disabled, true);
  assert.equal(control(ui).count.disabled, true);
  await ui.get("select-all").fire("click");
  assert.equal(ui.get("generate").disabled, true);
  assert.match(ui.get("settings-status").textContent, /ModuleID/);
  assert.deepEqual(ui.get("rows").children.map((row) => row.children[2].textContent), ["0", "3", "17", "19"]);
  await physicalID(ui, "0");
  await physicalID(ui, "0", 1);
  assert.equal(ui.get("generate").disabled, false);
  assert.match(ui.get("mode-note").textContent, /D32V\._NN/);
  await mode(ui, "fbd");
  assert.equal(control(ui).panel.hidden, true);
  assert.equal(control(ui).ids[0].disabled, true);
  await mode(ui, "st");
  assert.equal(control(ui).ids[0].value, "0");
});

test("ST uploads exact source, selected POU and physical zero ID without inferred IDs", async () => {
  const pending = deferred();
  const ui = await setup((url) => url.endsWith("/preview") ? response(plan()) : pending.promise);
  const file = await choose(ui, new File(["exact\r\nXLSX source"], "AI.xlsx"));
  control(ui).selected.checked = true;
  await control(ui).selected.fire("change");
  await physicalID(ui, "0");
  ui.get("filename").value = " SKZ_custom.xml ";
  const operation = ui.get("form").fire("submit");
  for (const key of ["file", "filename", "mode", "generate", "select-all", "clear-selection"]) assert.equal(ui.get(key).disabled, true, key);
  assert.equal(ui.context.every((input) => input.disabled), true);
  assert.equal(control(ui).ids[0].disabled, true);
  assert.equal(control(ui, 1).selected.disabled, true);
  const call = ui.calls.at(-1);
  assert.equal(call.url, "/api/skz/generate");
  const body = call.options.body;
  assert.equal(await body.get("file").text(), await file.text());
  assert.equal(body.get("fileName"), "SKZ_custom");
  assert.deepEqual(JSON.parse(body.get("config")), { kind: "st", pous: [{ groupKey: "SCS1/0", moduleCount: 1, moduleIds: [0] }] });
  assert.deepEqual(JSON.parse(body.get("context")), nativeContext);
  await ui.get("form").fire("submit");
  assert.equal(ui.calls.length, 3);
  pending.resolve(response(generated()));
  await operation;
  assert.equal(ui.get("result").hidden, false);
  assert.match(ui.get("result-summary").textContent, /1 POU ST · 2 каналов · 2 присваиваний/);
  assert.equal(ui.get("downloads").children[0].children[1].href, "http://localhost:3000/api/output/SKZ_SCS1.xml");
  assert.equal(control(ui).ids[0].disabled, false);
  assert.deepEqual(ui.events, ["schemegen:outputs-changed"]);
});

test("FBD requires no physical IDs and posts an empty list while preserving DO source tags", async () => {
  const ui = await setup((url) => url.endsWith("/preview") ? response(plan()) : response(generated(["SCS2"])));
  await choose(ui);
  control(ui, 1).selected.checked = true;
  await control(ui, 1).selected.fire("change");
  await mode(ui, "fbd");
  assert.equal(ui.get("generate").disabled, false);
  assert.match(ui.get("mode-note").textContent, /BOOL-теги DO.*iNN/);
  assert.equal(ui.get("rows").children[0].children[3].textContent, "_SOURCE_17");
  await ui.get("form").fire("submit");
  assert.deepEqual(JSON.parse(ui.calls.at(-1).options.body.get("config")), { kind: "fbd", pous: [{ groupKey: "SCS2/1", moduleCount: 1, moduleIds: [] }] });
  assert.match(ui.get("result-summary").textContent, /FBD/);
});

test("duplicate physical IDs within one SCS and invalid integer forms block ST", async () => {
  const ui = await setup(() => response(plan(["SCS1", "SCS1"])));
  await choose(ui);
  await ui.get("select-all").fire("click");
  await physicalID(ui, "0");
  await physicalID(ui, "0", 1);
  assert.equal(ui.get("generate").disabled, true);
  assert.match(ui.get("settings-status").textContent, /повторяется/);
  for (const value of ["", "-1", "1.5", "1e2", "2147483648"]) {
    await physicalID(ui, value, 1);
    assert.equal(ui.get("generate").disabled, true);
  }
  control(ui, 1).selected.checked = false;
  await control(ui, 1).selected.fire("change");
  assert.equal(ui.get("generate").disabled, false);
  assert.equal(ui.calls.length, 2);
});

test("a stale preview cannot overwrite a newer file or clear its pending state", async () => {
  const first = deferred(), second = deferred();
  let count = 0;
  const ui = await setup(() => ++count === 1 ? first.promise : second.promise);
  const old = choose(ui, new File(["old"], "old.xlsx"));
  const latest = choose(ui, new File(["new"], "new.xlsx"));
  first.resolve(response(plan(["OLD"])));
  await old;
  assert.equal(ui.get("preview").getAttribute("aria-busy"), "true");
  assert.equal(ui.calls[1].options.signal.aborted, true);
  second.resolve(response(plan(["NEW"])));
  await latest;
  assert.match(ui.get("groups").textContent, /NEW/);
  assert.doesNotMatch(ui.get("groups").textContent, /OLD/);
});

test("profile failure keeps preview usable and retry preserves source selection and IDs", async () => {
  let count = 0;
  const ui = await setup(() => response(plan()), () => ++count === 1 ? response({error:"profile unavailable"}, false) : response({context:nativeContext}));
  await choose(ui);
  await ui.get("select-all").fire("click");
  await physicalID(ui, "7");
  await physicalID(ui, "8", 1);
  assert.equal(ui.get("generate").disabled, true);
  assert.equal(ui.get("retry-profile").hidden, false);
  await ui.get("retry-profile").fire("click");
  assert.equal(ui.get("generate").disabled, false);
  assert.equal(control(ui).ids[0].value, "7");
  assert.equal(control(ui).selected.checked, true);
});

test("replacing or clearing the source resets physical IDs and selection", async () => {
  const ui = await setup(() => response(plan()));
  await choose(ui);
  await ui.get("select-all").fire("click");
  await physicalID(ui, "12");
  await choose(ui, new File(["other"], "DO.xlsx"));
  assert.equal(control(ui).ids[0].value, "");
  assert.equal(control(ui).selected.checked, false);
  await choose(ui, null);
  assert.equal(ui.get("preview").hidden, true);
  assert.equal(ui.get("generate").disabled, true);
});

test("invalid input files and incomplete SCADA context never generate", async () => {
  const ui = await setup(() => response(plan()));
  for (const file of [{name:"input.txt",size:12},{name:"input.xlsx",size:0},{name:"input.xlsx",size:17*1024*1024}]) {
    await choose(ui,file);
    assert.equal(ui.get("generate").disabled,true);
  }
  assert.equal(ui.calls.length,1);
  await choose(ui);
  await ui.get("select-all").fire("click");
  await mode(ui,"fbd");
  const groupID = ui.context.find((input) => input.dataset.skzContext === "groupId");
  groupID.value = "";
  await groupID.fire("input");
  assert.equal(ui.get("generate").disabled,true);
  await ui.get("form").fire("submit");
  assert.equal(ui.get("context").open,true);
  assert.equal(ui.calls.length,2);
});

test("all numeric context fields and the CPU type are validated before generation", async () => {
  const ui = await setup(() => response(plan(["SCS1", "SCS1"])));
  await choose(ui);
  await ui.get("select-all").fire("click");
  await mode(ui,"fbd");
  for (const key of ["version", "controllerId", "resourceId", "groupId", "pouNumber"]) {
    const input = ui.context.find((element) => element.dataset.skzContext === key);
    input.value = "1.5";
    await input.fire("input");
    assert.equal(ui.get("generate").disabled,true,key);
    input.value = nativeContext[key];
    await input.fire("input");
  }
  const number = ui.context.find((element) => element.dataset.skzContext === "pouNumber");
  number.value = "2147483647";
  await number.fire("input");
  assert.match(ui.get("settings-status").textContent,/Диапазон POUNum/);
  number.value = "0";
  await number.fire("input");
  const project = ui.context.find((element) => element.dataset.skzContext === "project");
  project.value = "";
  await project.fire("input");
  assert.equal(ui.get("generate").disabled,false);
  const cpu = ui.context.find((element) => element.dataset.skzContext === "controllerTypeName");
  cpu.value = "TENIX-CPU715";
  await cpu.fire("input");
  assert.equal(ui.get("generate").disabled,true);
  assert.match(ui.get("settings-status").textContent,/TENIX-CPU850/);
});

test("a late profile response retains the current workbook error", async () => {
  const pending = deferred();
  const ui = await setup(() => response({error:"Missing SCS at row 12"},false), () => pending.promise);
  await choose(ui);
  assert.match(ui.get("errors").textContent,/Missing SCS at row 12/);
  pending.resolve(response({context:nativeContext}));
  await settled();
  assert.match(ui.get("errors").textContent,/Missing SCS at row 12/);
  assert.equal(ui.get("generate").disabled,true);
});

test("AI and DO use their native context defaults without replacing manual overrides", async () => {
  let preview = { groups:[plan().groups[1]], warnings:[] };
  const contexts = { AI:nativeContext, DO:{...nativeContext,groupId:"19913",pouNumber:"47"} };
  const ui = await setup(() => response(preview), () => response({context:nativeContext,contexts}));
  const group = ui.context.find((input) => input.dataset.skzContext === "groupId");
  const number = ui.context.find((input) => input.dataset.skzContext === "pouNumber");
  await choose(ui,new File(["DO"],"DO.xlsx"));
  assert.equal(group.value,"19913");
  assert.equal(number.value,"47");
  group.value="900";
  await group.fire("input");
  preview = { groups:[plan().groups[0]], warnings:[] };
  await choose(ui,new File(["AI"],"AI.xlsx"));
  assert.equal(group.value,"900");
  assert.equal(number.value,"4");
  number.value="100";
  await number.fire("input");
  preview = { groups:[plan().groups[1]], warnings:[] };
  await choose(ui,new File(["DO again"],"DO.xlsx"));
  assert.equal(group.value,"900");
  assert.equal(number.value,"100");
});

test("AI ST and full FBD use separate POUNum defaults while manual settings persist", async () => {
  const contexts = {AI:nativeContext, DO:{...nativeContext,groupId:"19913",pouNumber:"47"}};
  const fbdContexts = {AI:{...nativeContext,pouNumber:"8"},DO:contexts.DO};
  const ui = await setup(() => response({groups:[plan().groups[0]],warnings:[]}), () => response({context:nativeContext,contexts,fbdContexts}));
  await choose(ui);
  const number = ui.context.find((input) => input.dataset.skzContext === "pouNumber");
  const group = ui.context.find((input) => input.dataset.skzContext === "groupId");
  assert.equal(number.value,"4");
  await mode(ui,"fbd");
  assert.equal(number.value,"8");
  assert.equal(group.value,"19912");
  assert.match(ui.get("mode-note").textContent,/полный фрагмент AD3_v2 с привязкой к тегам AI из карты/);
  assert.doesNotMatch(ui.get("mode-note").textContent,/MOS\/SRV|внутренни.*связ/);
  await mode(ui,"st");
  assert.equal(number.value,"4");
  number.value="100";
  await number.fire("input");
  group.value="900";
  await group.fire("input");
  await mode(ui,"fbd");
  assert.equal(number.value,"100");
  assert.equal(group.value,"900");
  await mode(ui,"st");
  assert.equal(number.value,"100");
  assert.equal(group.value,"900");
});

test("stale generation after a mode edit cannot publish results", async () => {
  const pending = deferred();
  const ui = await setup((url) => url.endsWith("/preview") ? response(plan()) : pending.promise);
  await choose(ui);
  await ui.get("select-all").fire("click");
  await mode(ui,"fbd");
  const operation = ui.get("form").fire("submit");
  await mode(ui,"st"); // A synthetic edit also invalidates the in-flight result.
  pending.resolve(response(generated()));
  await operation;
  assert.equal(ui.get("result").hidden,true);
  assert.deepEqual(ui.events,[]);
});

test("invalid download response is rejected and controls unlock for retry", async () => {
  const result = generated();
  result.files[0].url = "https://example.com/api/output/SKZ_SCS1.xml";
  const ui = await setup((url) => url.endsWith("/preview") ? response(plan()) : response(result));
  await choose(ui);
  await ui.get("select-all").fire("click");
  await mode(ui,"fbd");
  await ui.get("form").fire("submit");
  assert.equal(ui.get("result").hidden,true);
  assert.match(ui.get("errors").textContent,/недопустимый адрес/);
  assert.equal(ui.get("file").disabled,false);
  assert.equal(ui.get("generate").disabled,false);
  assert.deepEqual(ui.events,[]);
});

test("extra AI modules preserve sparse source channels and append reserves after the highest suffix", async () => {
  const preview = plan(["SCS1"]);
  const first = preview.groups[0].modules[0];
  first.name = "A1-02";
  preview.groups[0].modules.push({...first,name:"A1-14",channels:[{channel:7,tag:"_LAST",reserve:false,sourceRow:20}]});
  const ui = await setup(() => response(preview));
  await choose(ui);
  await ui.get("select-all").fire("click");
  assert.equal(control(ui).count.value,"2");
  assert.equal(control(ui).count.min,"2");
  await moduleCount(ui,3);
  assert.equal(control(ui).ids.length,3);
  assert.equal(control(ui).ids[2].value,"");
  assert.match(control(ui).ids[2].getAttribute("aria-label"),/A1-15/);
  const rows = ui.get("rows").children;
  assert.equal(rows.length,19);
  assert.deepEqual(rows.slice(0,3).map((row)=>row.children[2].textContent),["0","3","7"]);
  assert.equal(rows[3].children[3].textContent,"_SCS1_A1_15_0");
  assert.equal(rows[18].children[3].textContent,"_SCS1_A1_15_15");
  assert.equal(rows[3].children[5].textContent,"—");
  assert.match(ui.get("selection-summary").textContent,/3 модулей · 19 каналов · 16 резервов/);
  assert.match(ui.get("summary").textContent,/2 модулей · 3 каналов · 0 резервов/);
});

test("module IDs survive shrink and regrow, mode switches and deselection; added IDs are never guessed", async () => {
  const ui = await setup(()=>response(plan(["SCS1"])));
  await choose(ui);
  await ui.get("select-all").fire("click");
  await physicalID(ui,"7");
  await moduleCount(ui,3);
  let inputs = control(ui).ids;
  assert.deepEqual(inputs.map((input)=>input.value),["7","",""]);
  assert.equal(ui.get("generate").disabled,true);
  inputs[1].value="7";await inputs[1].fire("input");
  assert.match(ui.get("settings-status").textContent,/повторяется/);
  inputs[1].value="8";await inputs[1].fire("input");
  inputs[2].value="9";await inputs[2].fire("input");
  await moduleCount(ui,1);
  assert.equal(control(ui).ids.length,1);
  assert.equal(ui.get("generate").disabled,false);
  await mode(ui,"fbd");
  assert.equal(control(ui).count.disabled,false);
  await ui.get("clear-selection").fire("click");
  assert.equal(control(ui).count.disabled,true);
  await ui.get("select-all").fire("click");
  await moduleCount(ui,3);
  await mode(ui,"st");
  assert.deepEqual(control(ui).ids.map((input)=>input.value),["7","8","9"]);
  assert.equal(control(ui).ids[1],inputs[1]);
  assert.equal(ui.get("generate").disabled,false);
  await moduleCount(ui,4);
  assert.equal(control(ui).ids[3].value,"");
  assert.equal(ui.get("generate").disabled,true);
});

test("moduleCount is submitted in ST and FBD with only currently enabled physical IDs", async () => {
  const ui = await setup((url)=>url.endsWith("/preview")?response(plan(["SCS1"])):response(generated()));
  await choose(ui);
  await ui.get("select-all").fire("click");
  await moduleCount(ui,2);
  for(const [index,input] of control(ui).ids.entries()){input.value=String(index);await input.fire("input");}
  await ui.get("form").fire("submit");
  assert.deepEqual(JSON.parse(ui.calls.at(-1).options.body.get("config")),{kind:"st",pous:[{groupKey:"SCS1/0",moduleCount:2,moduleIds:[0,1]}]});
  await mode(ui,"fbd");
  await ui.get("form").fire("submit");
  assert.deepEqual(JSON.parse(ui.calls.at(-1).options.body.get("config")),{kind:"fbd",pous:[{groupKey:"SCS1/0",moduleCount:2,moduleIds:[]}]});
  await mode(ui,"st");
  await moduleCount(ui,1);
  await ui.get("form").fire("submit");
  assert.deepEqual(JSON.parse(ui.calls.at(-1).options.body.get("config")),{kind:"st",pous:[{groupKey:"SCS1/0",moduleCount:1,moduleIds:[0]}]});
});

test("module-count bounds and effective selected channel limits prevent generation in both modes", async () => {
  const ui = await setup(()=>response(plan()));
  await choose(ui);
  await ui.get("select-all").fire("click");
  await mode(ui,"fbd");
  for(const value of ["",0,-1,"1.5","1e2",4097]){
    await moduleCount(ui,value);
    assert.equal(ui.get("generate").disabled,true,String(value));
    assert.match(ui.get("settings-status").textContent,/количество модулей/);
    assert.equal(ui.get("rows").children.length,0);
  }
  await moduleCount(ui,1);
  await moduleCount(ui,128,1); // 2 AI channels plus 4066 DO channels, still only 2 FBD POUs.
  assert.equal(ui.get("generate").disabled,false);
  await moduleCount(ui,3); // An additional 32 AI channels exceeds the shared cap.
  assert.equal(ui.get("generate").disabled,true);
  assert.match(ui.get("settings-status").textContent,/4096.*каналов/);
  assert.equal(ui.get("rows").children.length,0);
  await mode(ui,"st");
  assert.match(ui.get("settings-status").textContent,/4096.*каналов/);
  control(ui).selected.checked=false;await control(ui).selected.fire("change");
  await mode(ui,"fbd");
  assert.equal(ui.get("generate").disabled,false);
  assert.equal(ui.get("rows").children.length,4066);
});

test("AI FBD shows one POU per module, including added modules, while ST and DO retain their groups", async () => {
  const preview = plan(["SCS1", "scs1"]);
  const result = generated(["scs1"]);
  delete result.summary;
  delete result.files[0].summary;
  const ui = await setup((url) => url.endsWith("/preview") ? response(preview) : response(result));
  await choose(ui);
  await ui.get("select-all").fire("click");
  await moduleCount(ui, 3);
  await moduleCount(ui, 2, 1);
  for (const index of [0, 1]) {
    for (const [offset, input] of control(ui, index).ids.entries()) {
      input.value = String(index * 10 + offset);
      await input.fire("input");
    }
  }
  assert.match(ui.get("summary").textContent, /1 ПЛК · 2 групп/);
  assert.match(ui.get("selection-summary").textContent, /1 ПЛК · 2 POU · 5 модулей/);
  assert.equal(ui.get("rows").children[0].children[0].textContent, "AI_A1_channels / SCS1");
  await mode(ui, "fbd");
  assert.equal(ui.get("generate").disabled, false);
  assert.match(ui.get("selection-summary").textContent, /1 ПЛК · 4 POU · 5 модулей/);
  assert.match(ui.get("mode-note").textContent, /Каждый модуль AI получает отдельную POU/);
  assert.deepEqual(ui.get("rows").children.map((row) => row.children[0].textContent).filter(Boolean),
    ["AI_A1_00 / SCS1", "AI_A1_01 / SCS1", "AI_A1_02 / SCS1", "DO_A3 / scs1", "DO_A3 / scs1"]);
  ui.get("search").value = "AI_A1_01";
  await ui.get("search").fire("input");
  assert.equal(ui.get("rows").children.length, 16);
  assert.equal(ui.get("rows").children[0].children[0].textContent, "AI_A1_01 / SCS1");
  await ui.get("form").fire("submit");
  assert.match(ui.get("result-summary").textContent, /^4 POU FBD/);
  assert.match(ui.get("downloads").children[0].children[0].textContent, /4 POU/);
  assert.deepEqual(JSON.parse(ui.calls.at(-1).options.body.get("config")), {
    kind: "fbd", pous: [
      { groupKey: "SCS1/0", moduleCount: 3, moduleIds: [] },
      { groupKey: "scs1/1", moduleCount: 2, moduleIds: [] },
    ],
  });
  await mode(ui, "st");
  assert.match(ui.get("selection-summary").textContent, /1 ПЛК · 2 POU · 5 модулей/);
  assert.deepEqual(control(ui).ids.map((input) => input.value), ["0", "1", "2"]);
  await mode(ui, "fbd");
  await moduleCount(ui, 2);
  assert.match(ui.get("selection-summary").textContent, /1 ПЛК · 3 POU · 4 модулей/);
});

test("POUNum bounds include every AI FBD module and are checked independently for each PLC", async () => {
  const preview = plan();
  const ui = await setup(() => response(preview));
  await choose(ui);
  await ui.get("select-all").fire("click");
  await mode(ui, "fbd");
  await moduleCount(ui, 3);
  const number = ui.context.find((input) => input.dataset.skzContext === "pouNumber");
  number.value = "2147483646";
  await number.fire("input");
  assert.equal(ui.get("generate").disabled, true);
  assert.match(ui.get("settings-status").textContent, /Диапазон POUNum/);
  await ui.get("form").fire("submit");
  assert.equal(number.focused, true);
  assert.equal(ui.calls.length, 2);
  number.value = "2147483645";
  await number.fire("input");
  assert.equal(ui.get("generate").disabled, false); // Three AI POU in SCS1, one DO POU in SCS2.
  preview.groups[1].scs = "scs1";
  await choose(ui, new File(["same controller"], "same.xlsx"));
  await ui.get("select-all").fire("click");
  await moduleCount(ui, 3);
  assert.equal(ui.get("generate").disabled, true); // Four POU now share one PLC number range.
  assert.match(ui.get("settings-status").textContent, /Диапазон POUNum/);
  await mode(ui, "st");
  assert.match(ui.get("settings-status").textContent, /ModuleID/); // ST still creates two POU.
});

test("the 128 POU limit counts expanded AI FBD modules across all selected PLCs", async () => {
  const ui = await setup(() => response(plan()));
  await choose(ui);
  await ui.get("select-all").fire("click");
  await mode(ui, "fbd");
  await moduleCount(ui, 127);
  assert.equal(ui.get("generate").disabled, false);
  assert.match(ui.get("selection-summary").textContent, /128 POU/);
  await moduleCount(ui, 128);
  assert.equal(ui.get("generate").disabled, true);
  assert.match(ui.get("settings-status").textContent, /до 128 POU.*129/);
  assert.equal(ui.get("rows").children.length, 0);
  await ui.get("form").fire("submit");
  assert.equal(ui.calls.length, 2);
  control(ui, 1).selected.checked = false;
  await control(ui, 1).selected.fire("change");
  assert.equal(ui.get("generate").disabled, false);
  await moduleCount(ui, 129);
  assert.equal(ui.get("generate").disabled, true);
  await mode(ui, "st");
  assert.match(ui.get("settings-status").textContent, /ModuleID/);
  assert.match(ui.get("selection-summary").textContent, /1 POU · 129 модулей/);
});

test("extra DO modules expose 32 D32V positions without creating individual BOOL tags", async () => {
  const source = plan();
  const preview = {groups:[source.groups[1]],warnings:[]};
  const ui = await setup(()=>response(preview));
  await choose(ui);
  await ui.get("select-all").fire("click");
  await mode(ui,"fbd");
  await moduleCount(ui,2);
  const rows=ui.get("rows").children;
  assert.equal(rows.length,34);
  assert.deepEqual(rows.slice(0,2).map((row)=>row.children[2].textContent),["17","19"]);
  assert.equal(rows[2].children[3].textContent,"_SCS2_A3_01._00");
  assert.equal(rows[33].children[3].textContent,"_SCS2_A3_01._31");
  for(const row of rows.slice(2)){
    assert.equal(row.children[4].textContent,"D32V");
    assert.equal(row.children[6].textContent,"Резерв D32V");
  }
  assert.match(ui.get("selection-summary").textContent,/2 модулей · 34 каналов · 32 резервов/);
});

test("extra suffixes extend beyond 99 and new files reset saved counts and hidden IDs", async () => {
  const preview=plan(["SCS1"]);
  preview.groups[0].modules[0].name="A1-99";
  const ui=await setup(()=>response(preview));
  await choose(ui);
  await ui.get("select-all").fire("click");
  await moduleCount(ui,2);
  assert.match(control(ui).ids[1].getAttribute("aria-label"),/A1-100/);
  assert.equal(ui.get("rows").children[2].children[3].textContent,"_SCS1_A1_100_0");
  control(ui).ids[1].value="123";await control(ui).ids[1].fire("input");
  await choose(ui,new File(["new"],"new.xlsx"));
  assert.equal(control(ui).count.value,"1");
  await ui.get("select-all").fire("click");
  await moduleCount(ui,2);
  assert.equal(control(ui).ids[1].value,"");
});

test("suffix overflow is rejected before allocating additional module controls", async () => {
  const preview=plan(["SCS1"]);
  preview.groups[0].modules[0].name="A1-4094";
  const ui=await setup(()=>response(preview));
  await choose(ui);
  await ui.get("select-all").fire("click");
  await mode(ui,"fbd");
  await moduleCount(ui,2);
  assert.match(control(ui).ids[1].getAttribute("aria-label"),/A1-4095/);
  assert.equal(ui.get("generate").disabled,false);
  await moduleCount(ui,3);
  assert.equal(control(ui).ids.length,2);
  assert.equal(ui.get("generate").disabled,true);
  assert.match(ui.get("settings-status").textContent,/A1-4096.*4095/);
  await moduleCount(ui,4096);
  assert.equal(control(ui).ids.length,2);
  assert.equal(ui.get("rows").children.length,0);
  await ui.get("form").fire("submit");
  assert.equal(control(ui).count.focused,true);
  assert.equal(ui.calls.length,2);
});

test("new modules cannot collide with an unselected source POU in the same SCS", async () => {
  const preview=plan(["SCS1","scs1"]);
  preview.groups[1].prefix="A1";
  preview.groups[1].pouName="DO_A1";
  preview.groups[1].modules[0].name="A1_01";
  const ui=await setup(()=>response(preview));
  await choose(ui);
  control(ui).selected.checked=true;await control(ui).selected.fire("change");
  await mode(ui,"fbd");
  await moduleCount(ui,2);
  assert.equal(ui.get("generate").disabled,true);
  assert.match(ui.get("settings-status").textContent,/A1-01.*уже есть в XLSX.*DO_A1/);
  assert.equal(control(ui,1).selected.checked,false);
  await ui.get("form").fire("submit");
  assert.equal(control(ui).count.focused,true);
  await moduleCount(ui,1);
  assert.equal(ui.get("generate").disabled,false);
});

test("additional module names must be unique among selected expansions, with independent SCS namespaces", async () => {
  const preview=plan(["SCS1","SCS1"]);
  preview.groups[1].prefix="A1";
  preview.groups[1].pouName="DO_A1";
  preview.groups[1].modules[0].name="A1-00";
  const ui=await setup(()=>response(preview));
  await choose(ui);
  await ui.get("select-all").fire("click");
  await mode(ui,"fbd");
  await moduleCount(ui,2);
  await moduleCount(ui,2,1);
  assert.equal(ui.get("generate").disabled,true);
  assert.match(ui.get("settings-status").textContent,/A1-01.*повторяется.*AI_A1.*DO_A1/);
  control(ui,1).selected.checked=false;await control(ui,1).selected.fire("change");
  assert.equal(ui.get("generate").disabled,false);
  preview.groups[1].scs="SCS2";
  await choose(ui,new File(["independent PLC"],"other.xlsx"));
  await ui.get("select-all").fire("click");
  await moduleCount(ui,2);
  await moduleCount(ui,2,1);
  assert.equal(ui.get("generate").disabled,false);
});
