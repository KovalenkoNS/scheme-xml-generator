// AI/AO/DI/DO acceptance uses the real built app, upload APIs and installed XML library.
// Synthetic two-PLC tables make source isolation observable; saved XML and screenshots stay local.
'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const net = require('node:net');
const { createHash } = require('node:crypto');
const { spawn, spawnSync } = require('node:child_process');
const { until, browser, evaluate, upload, screenshot } = require('../support/chromium.cjs');
const root = path.resolve(__dirname, '../..');
const output = path.join(root, '.artifacts/directions-browser', `run-${Date.now()}-${process.pid}`);
const evidence = { checks: [], directions: {}, errors: [], apiDiagnostics: [], scope: 'AI + AO + DI + DO. Actual saved XML and browser, no SCADA import.' };
let app, chromium, base;

// ev probes the isolated browser and surfaces any page exception as a failed check.
const ev = expression => {
  // Test probes import the actual component; production exposes no automation globals.
  if (!/GeneratorWorkspace|GeneratedPreviewParser/.test(expression)) return evaluate(chromium.cdp, expression);
  const probe = expression.replace(/!!window\.GeneratorWorkspace\?\.state\.loaded/g, 'workspaceState.loaded').replace(/GeneratorWorkspace\.state/g, 'workspaceState').replace(/GeneratedPreviewParser/g, 'savedXMLParser');
  return evaluate(chromium.cdp, `(async()=>{const {state:workspaceState}=await import('/shell/state.js');const savedXMLParser=await import('/preview/parser.js');return (${probe});})()`);
};

// field applies a user-visible control value using the same event as manual interaction.
async function field(selector, value, event = 'input') {
  await ev(`{const node=document.querySelector(${JSON.stringify(selector)});if(!node)throw Error('Missing control: '+${JSON.stringify(selector)});node.value=${JSON.stringify(String(value))};node.dispatchEvent(new Event(${JSON.stringify(event)}));}`);
}

// check records a completed acceptance step; errors never turn into a skipped direction.
async function check(name, task) { await task(); evidence.checks.push(name); console.log('PASS ' + name); }

// loadSource starts a fresh workspace with actual fixtures and selects PLC A, leaving PLC B out.
async function loadSource(kind, extension = 'xlsx', stem = kind) {
  await ev('localStorage.clear()');
  const token = String(Date.now());
  await chromium.cdp.send('Page.navigate', { url: base + '/?matrix=' + token + '#local' });
  await until('workspace catalogue', () => ev(`location.search === '?matrix=${token}' && !!window.GeneratorWorkspace?.state.loaded && GeneratorWorkspace.state.catalog.templates.length > 0`));
  await upload(chromium.cdp, '#source-files', [path.join(output, 'fixtures', `${stem}.${extension}`)]);
  await until(`${kind} parsed`, () => ev(`!document.querySelector("#plc").disabled && GeneratorWorkspace.state.sources[0]?.file.name === '${stem}.${extension}' && !GeneratorWorkspace.state.sources[0].loading`));
  assert.deepEqual(await ev('[...document.querySelector("#plc").options].map(o=>o.value).filter(Boolean)'), ['VERIFY_SC_A', 'VERIFY_SC_B']);
  await field('#plc', 'VERIFY_SC_A', 'change');
  await field('#cpu', kind === 'DO' ? 'TENIX-CPU850' : 'TENIX-CPU715', 'change');
}

// generate waits for the actual HTTP/file workflow and returns its saved same-origin XML.
async function generate() {
  await ev('document.querySelector("#generate").click()');
  await until('generation complete', () => ev('!document.querySelector("#workspace-form").inert'), 60000);
  assert.equal(await ev('document.querySelectorAll("#result-list a").length'), 1, await ev('document.querySelector("#result-list").textContent'));
  const url = await ev('document.querySelector("#result-list a").href');
  const response = await fetch(url); assert.equal(response.status, 200);
  return { url, xml: await response.text() };
}

// verifyFBD checks a selected library template, source PLC isolation and exact saved-graph rendering.
async function verifyFBD(kind, templateID) {
  await loadSource(kind, kind === 'AO' ? 'txt' : 'xlsx');
  const key = await ev(`GeneratorWorkspace.state.catalog.templates.find(t=>t.libraryFile==='all_lb_sinopec.xml' && t.id===${JSON.stringify(templateID)} && t.supported)?.key`);
  assert.ok(key, `Required explicit library fixture template ${kind}/${templateID} is unavailable`);
  await field(`[aria-label="${kind} · библиотечный шаблон"]`, key, 'change');
  await field('[data-output=fbd] [aria-label="GroupID"]', '10');
  await field('[data-output=fbd] [aria-label="Первый POUNum"]', '100');
  if (kind === 'DO') {
    await field('[data-output=fbd] .module-grid input', '17');
    await ev('document.querySelector("[data-output=fbd] .check-label input").click()');
  }
  const saved = await generate();
  assert.ok(saved.xml.includes(`_VERIFY_${kind}_A`)); assert.ok(!saved.xml.includes(`_VERIFY_${kind}_B`));
  fs.writeFileSync(path.join(output, `${kind}-fbd.xml`), saved.xml);
  const expected = await ev(`GeneratedPreviewParser.parseXML(${JSON.stringify(saved.xml)}).pages.map(p=>({name:p.name,blocks:p.blocks.length,links:p.links.length}))`);
  assert.ok(expected.length > 0 && expected[0].blocks > 0);
  await ev('document.querySelector(".result-preview").click()');
  await until('saved graph visible', () => ev('!!document.querySelector(".gp-dialog")?.open && document.querySelectorAll(".gp-block").length > 0'));
  assert.equal(await ev('document.querySelectorAll(".gp-block").length'), expected[0].blocks);
  assert.equal(await ev('document.querySelectorAll(".gp-link").length'), expected[0].links);
  assert.ok(Number((await ev('document.querySelector(".gp-zoom").textContent')).replace('%', '')) >= 100);
  if (kind === 'DO') {
    assert.equal(await ev('document.querySelectorAll(".gp-module").length'), 1);
    assert.equal(await ev('document.querySelectorAll(".gp-not").length'), 32);
  }
  await field('.gp-navigation input', `_VERIFY_${kind}_A`);
  assert.ok(await ev('document.querySelectorAll(".gp-search-match").length > 0'));
  await screenshot(chromium.cdp, path.join(output, `${kind}-preview.png`));
  evidence.directions[kind] = { templateID, key, savedFile: `${kind}-fbd.xml`, pages: expected, previewScale: await ev('document.querySelector(".gp-zoom").textContent') };
}

// verifyHMI exercises all four physical module directions through the supported CPU715 profile.
async function verifyHMI(kind) {
  await loadSource(kind);
  await field('#cpu', 'TENIX-CPU715', 'change');
  await ev('document.querySelector("input[name=output][value=fbd]").click();document.querySelector("input[name=output][value=hmi]").click()');
  const saved = await generate();
  assert.ok(!saved.xml.includes('VERIFY_SC_B'));
  fs.writeFileSync(path.join(output, `${kind}-hmi.xml`), saved.xml);
  const model = await ev(`(()=>{const model=GeneratedPreviewParser.parseXML(${JSON.stringify(saved.xml)});return {kind:model.kind,pages:model.pages.length,primitives:model.pages.reduce((n,p)=>n+p.primitives.length,0)};})()`);
  assert.equal(model.kind, 'HMI'); assert.ok(model.pages > 0 && model.primitives > 0);
  await ev('document.querySelector(".result-preview").click()');
  await until('saved HMI visible', () => ev('!!document.querySelector(".gp-dialog")?.open && document.querySelectorAll(".gp-primitive").length > 0'));
  await screenshot(chromium.cdp, path.join(output, `${kind}-hmi-preview.png`));
  evidence.directions[kind].hmi715 = model;
}

// verifyST checks actual physical assignments and exact saved STCODE for every direction.
async function verifyST(kind) {
  await loadSource(kind, kind === 'AO' ? 'txt' : 'xlsx', kind === 'AO' ? 'AO' : `ST-${kind}`);
  const cpu = kind === 'AO' ? 'TENIX-CPU715' : 'TENIX-CPU850';
  await field('#cpu', cpu, 'change');
  await ev('document.querySelector("input[name=output][value=fbd]").click();document.querySelector("input[name=output][value=st]").click()');
  await field('[data-output=st] .module-grid input', '17');
  await field('[data-output=st] [aria-label="GroupID"]', '10');
  await field('[data-output=st] [aria-label="Первый POUNum"]', '200');
  const saved = await generate();
  assert.ok(saved.xml.includes(['DI', 'DO'].includes(kind) ? '_VERIFY_SC_A_A11_02' : `_VERIFY_${kind}_A`));
  assert.ok(!saved.xml.includes(`_VERIFY_${kind}_B`) && !saved.xml.includes('_VERIFY_SC_B'));
  fs.writeFileSync(path.join(output, `${kind}-st.xml`), saved.xml);
  const expected = await ev(`new DOMParser().parseFromString(${JSON.stringify(saved.xml)},'application/xml').getElementsByTagName('STCODE')[0].textContent`);
  assert.equal((expected.match(/:=/g) || []).length, { AI: 2, AO: 4, DI: 33, DO: 32 }[kind]);
  if (kind === 'DI') assert.ok(expected.includes('_VERIFY_SC_A_A11_02.i31 := _IO_I17_DI32_31_VAL.Measurement;'));
  if (kind === 'DO') assert.ok(expected.includes('_IO_Q17_DO32P_31_VAL.Measurement := _VERIFY_SC_A_A11_02._31;'));
  await ev('document.querySelector(".result-preview").click()');
  await until('saved ST visible', () => ev('!!document.querySelector(".gp-dialog")?.open && !!document.querySelector(".gp-code")?.textContent'));
  assert.equal(await ev('document.querySelector(".gp-code").textContent'), expected);
  await screenshot(chromium.cdp, path.join(output, `${kind}-st-preview.png`));
  evidence.directions[kind].st = { cpu, assignments: (expected.match(/:=/g) || []).length, exactSavedCode: true };
}

// main runs the complete direction matrix without accessing Host or modifying installation data.
async function main() {
  fs.mkdirSync(path.join(output, 'libraries'), { recursive: true });
  const fixture = spawnSync('python', [path.join(root, 'tests/support/matrix_fixtures.py'), path.join(output, 'fixtures')], { encoding: 'utf8', windowsHide: true });
  assert.equal(fixture.status, 0, fixture.stderr);
  fs.copyFileSync(path.join(root, 'libraries/all_lb_sinopec.xml'), path.join(output, 'libraries/all_lb_sinopec.xml'));
  const listener = net.createServer(); await new Promise(resolve => listener.listen(0, '127.0.0.1', resolve)); const port = listener.address().port; await new Promise(resolve => listener.close(resolve));
  base = `http://127.0.0.1:${port}`;
  const executable = process.env.GENERATOR_EXE || path.join(root, 'XmlSchemeGenerator.exe');
  evidence.build = { executable, sha256: createHash('sha256').update(fs.readFileSync(executable)).digest('hex') };
  evidence.librarySha256 = createHash('sha256').update(fs.readFileSync(path.join(output, 'libraries/all_lb_sinopec.xml'))).digest('hex');
  app = spawn(executable, ['-root', output, '-port', String(port), '-no-browser'], { windowsHide: true, stdio: 'ignore' });
  await until('generator API', async () => { try { return (await fetch(base + '/api/health')).ok; } catch { return false; } }, 60000);
  chromium = await browser(output);
  chromium.cdp.listeners.push(message => {
    if (message.method === 'Runtime.exceptionThrown') evidence.errors.push(message.params);
    if (message.method === 'Log.entryAdded' && message.params.entry.level === 'error' && !message.params.entry.url?.endsWith('/favicon.ico')) {
      const entry = message.params.entry;
      (entry.url && new URL(entry.url).pathname.startsWith('/api/') ? evidence.apiDiagnostics : evidence.errors).push(entry);
    }
  });
  await chromium.cdp.send('Page.navigate', { url: base });
  await until('initial workspace', () => ev('!!window.GeneratorWorkspace?.state.loaded'));
  for (const [kind, id] of [['AI', '17510'], ['AO', '17625'], ['DI', '18442'], ['DO', '12772']]) {
    await check(`${kind}: real source → selected PLC → library FBD → saved XML → readable preview`, () => verifyFBD(kind, id));
    await check(`${kind}: CPU715 diagnostic HMI from physical inventory`, () => verifyHMI(kind));
    await check(`${kind}: supported physical ST profile and exact saved-code preview`, () => verifyST(kind));
  }
  assert.deepEqual(evidence.errors, []);
}

main().catch(error => { evidence.errors.push(error.stack); console.error(error); process.exitCode = 1; }).finally(() => {
  fs.mkdirSync(output, { recursive: true }); fs.writeFileSync(path.join(output, 'evidence.json'), JSON.stringify(evidence, null, 2));
  chromium?.cdp.close(); chromium?.processHandle.kill(); app?.kill(); console.log(output);
});
