// End-to-end workspace checks against a built generator and real local fixtures.
// Every generated file/state change is confined to this test's .artifacts root.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const net = require('node:net');
const { spawn } = require('node:child_process');
const { until, browser, evaluate, upload, screenshot } = require('../support/chromium.cjs');
const root = path.resolve(__dirname, '../..');
const output = path.join(root, '.artifacts/workspace-browser', `run-${Date.now()}-${process.pid}`);
const checks = [], errors = [], apiDiagnostics = []; let app, chromium;
// Ev runs a browser assertion against the test-owned page.
const ev = expression => evaluate(chromium.cdp, expression);
// Check records evidence only after the complete browser assertion succeeds.
async function check(name, body) { await body(); checks.push(name); console.log(`PASS ${name}`); }
// Main exercises the installed shell against real parsing/generation APIs in an isolated data root.
async function main() {
  fs.mkdirSync(path.join(output, 'libraries'), { recursive: true });
  fs.copyFileSync(path.join(root, 'libraries/Library AD3_v2.xml'), path.join(output, 'libraries/Library AD3_v2.xml'));
  const listener = net.createServer(); await new Promise(resolve => listener.listen(0, '127.0.0.1', resolve)); const port = listener.address().port; await new Promise(resolve => listener.close(resolve));
  const url = `http://127.0.0.1:${port}`;
  app = spawn(process.env.GENERATOR_EXE || path.join(root, 'XmlSchemeGenerator.exe'), ['-root', output, '-port', String(port), '-no-browser'], { windowsHide: true, stdio: 'ignore' });
  await until('generator API', async () => { try { return (await fetch(url + '/api/health')).ok; } catch { return false; } });
  chromium = await browser(output);
  chromium.cdp.listeners.push(message => {
    if (message.method === 'Runtime.exceptionThrown') errors.push(message.params);
    if (message.method === 'Log.entryAdded' && message.params.entry.level === 'error' && !message.params.entry.url?.endsWith('/favicon.ico')) {
      // Alternate source parsers may reject an input; assertions check their visible result.
      // Failed static module loads are always fatal and remain distinct from API diagnostics.
      const entry = message.params.entry;
      (entry.url && new URL(entry.url).pathname.startsWith('/api/') ? apiDiagnostics : errors).push(entry);
    }
  });
  await chromium.cdp.send('Page.navigate', { url });
  await until('workspace loaded', () => ev('document.getElementById("library-count").textContent === "1"'));
  await check('two pages; main is honest about disconnected Host and never offers fake DB generation', async () => {
    assert.equal(await ev('document.querySelectorAll("nav [data-page]").length'), 2);
    assert.equal(await ev('document.getElementById("generate").disabled'), true);
    assert.equal(await ev('document.getElementById("remote-source").hidden'), false);
    await screenshot(chromium.cdp, path.join(output, 'main.png'));
  });
  await check('library dialog exposes real types and template search, then closes with Escape', async () => {
    await ev('document.getElementById("library-open").click()');
    assert.equal(await ev('document.getElementById("library-dialog").open'), true);
    assert.ok(await ev('document.querySelectorAll(".type-item").length > 0'));
    await ev('document.getElementById("library-search").value="no-such-type-123";document.getElementById("library-search").dispatchEvent(new Event("input"))');
    assert.equal(await ev('document.querySelectorAll(".type-item").length'), 0);
    await chromium.cdp.send('Input.dispatchKeyEvent', { type: 'keyDown', key: 'Escape', code: 'Escape', windowsVirtualKeyCode: 27 });
    await chromium.cdp.send('Input.dispatchKeyEvent', { type: 'keyUp', key: 'Escape', code: 'Escape', windowsVirtualKeyCode: 27 });
    await until('dialog closed', () => ev('!document.getElementById("library-dialog").open'));
  });
  await ev('location.hash="local"'); await until('local page', () => ev('!document.getElementById("local-source").hidden'));
  await upload(chromium.cdp, '#source-files', [path.join(root, 'tests/fixtures/skzmap/ai.xlsx')]);
  await until('real workbook parsed', () => ev('!document.getElementById("plc").disabled && document.getElementById("generate").disabled === false'));
  await check('local workbook provides PLCs without domain selectors or separate mode pages', async () => {
    assert.ok(await ev('document.getElementById("plc").options.length > 0'));
    assert.equal(await ev('document.querySelectorAll("[data-output=fbd]").length'), 1);
    assert.ok(await ev('!document.querySelector("[name=system-domain]")'));
    await screenshot(chromium.cdp, path.join(output, 'local.png'));
  });
  await check('missing explicit library selection produces an actionable failure, no native fallback', async () => {
    await ev('document.getElementById("generate").click()');
    await until('validation result', () => ev('document.getElementById("result-list").textContent.includes("выберите поддержанный шаблон")'));
    assert.equal(await ev('document.querySelectorAll("#result-list a").length'), 0);
  });
  await check('real FBD generation uses the selected catalog template and the saved XML is downloadable', async () => {
    const template = (await (await fetch(url + '/api/templates')).json()).templates.find(item => item.id === '19963'); assert.ok(template);
    await ev(`document.querySelector('[aria-label="AI · библиотечный шаблон"]').value=${JSON.stringify(template.key)};document.querySelector('[aria-label="AI · библиотечный шаблон"]').dispatchEvent(new Event('change'))`);
    await ev(`for (const [label,value] of [['GroupID','10'],['Первый POUNum','100']]) { const field=document.querySelector('[data-output=fbd] [aria-label="'+label+'"]'); field.value=value; field.dispatchEvent(new Event('input')); }`);
    await ev('document.getElementById("generate").click()');
    await until('FBD request completed', () => ev('!document.getElementById("workspace-form").inert'), 60000);
    assert.equal(await ev('document.querySelectorAll("#result-list a").length'), 1, await ev('document.getElementById("result-list").textContent'));
    const target = await ev('document.querySelector("#result-list a").href');
    const response = await fetch(target); assert.equal(response.status, 200); const xml = await response.text();
    assert.match(xml, /<OnePOU/); assert.match(xml, /<Block/); assert.doesNotMatch(xml, /<STCODE/);
    fs.writeFileSync(path.join(output, 'generated-fbd.xml'), xml);
    await screenshot(chromium.cdp, path.join(output, 'generated.png'));
  });
  await check('result opens the saved XML in the integrated graphical viewer', async () => {
    await ev('document.querySelector(".result-preview").click()');
    await until('saved FBD visible', () => ev('document.querySelectorAll(".gp-block").length > 0'), 60000);
    assert.ok(await ev('document.querySelector(".gp-dialog").open'));
    await screenshot(chromium.cdp, path.join(output, 'integrated-preview.png'));
    await chromium.cdp.send('Input.dispatchKeyEvent', { type: 'keyDown', key: 'Escape', code: 'Escape', windowsVirtualKeyCode: 27 });
    await chromium.cdp.send('Input.dispatchKeyEvent', { type: 'keyUp', key: 'Escape', code: 'Escape', windowsVirtualKeyCode: 27 });
    await until('preview closed', () => ev('!document.querySelector(".gp-dialog").open'));
  });
  await check('ST refuses absent physical IDs while a selected supported FBD remains available', async () => {
    await ev('document.querySelector("input[value=st]").click();document.getElementById("generate").click()');
    await until('partial result', () => ev('document.getElementById("status").textContent.includes("частичный")'), 60000);
    assert.match(await ev('document.getElementById("result-list").textContent'), /ModuleID/);
    assert.equal(await ev('document.querySelectorAll("#result-list a").length'), 1);
  });
  await check('mobile layout keeps controls inside the viewport', async () => {
    await chromium.cdp.send('Emulation.setDeviceMetricsOverride', { width: 430, height: 900, deviceScaleFactor: 1, mobile: true });
    assert.ok(await ev('document.documentElement.scrollWidth <= innerWidth + 1'));
    await screenshot(chromium.cdp, path.join(output, 'mobile.png'));
  });
  assert.deepEqual(errors, []);
}
main().catch(error => { errors.push(error.stack); process.exitCode = 1; console.error(error); }).finally(() => {
  fs.writeFileSync(path.join(output, 'evidence.json'), JSON.stringify({ checks, errors, apiDiagnostics, scope: 'Real generator, real XLSX and XML library, isolated output. No SCADA import.' }, null, 2));
  chromium?.cdp.close(); chromium?.processHandle.kill(); app?.kill(); console.log(output);
});
