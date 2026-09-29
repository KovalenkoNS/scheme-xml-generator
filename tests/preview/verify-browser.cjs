// Real Chromium opens saved outputs produced by fixturegen; no user output or SCADA is changed.
'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const http = require('node:http');
const { until, browser: launchBrowser, evaluate: evaluatePage, screenshot: saveScreenshot } = require('../support/chromium.cjs');
const root = path.resolve(__dirname, '../..');
const fixtures = process.env.PREVIEW_FIXTURES || path.join(root, '.artifacts/preview-fixtures');
const artifact = path.join(root, '.artifacts/preview-browser', `run-${Date.now()}-${process.pid}`);
const evidence = { checks: [], screenshots: [], errors: [], requests: [] };
let server, browser, cdp;
// Evaluate forwards one preview assertion to the shared transport and propagates page errors.
const evaluate = expression => evaluatePage(cdp, expression);
// Check records a preview scenario only after every assertion has finished successfully.
async function check(name, task) { await task(); evidence.checks.push(name); console.log('PASS ' + name); }
// Screenshot captures only the isolated preview viewport and registers its evidence filename.
async function screenshot(name) { await saveScreenshot(cdp, path.join(artifact,name+'.png')); evidence.screenshots.push(name+'.png'); }
// Invoke the real preview module without adding a production global for the harness.
const open = name => evaluate(`import('/preview/viewer.js').then(viewer=>viewer.open({fileName:${JSON.stringify(name)},url:${JSON.stringify('/api/output/'+name)}})).then(value=>value?({kind:value.kind,pages:value.pages.length,blocks:value.pages[0].blocks.length,links:value.pages[0].links.length}):({error:document.querySelector('.gp-status')?.textContent}))`);

// Main serves generated test XML and canonical viewer modules, then verifies their actual browser behavior.
async function main() {
  fs.mkdirSync(artifact,{recursive:true});
  const files = new Map(['fbd.xml','st.xml','hmi.xml','module-do.xml'].map(name => [name,fs.readFileSync(path.join(fixtures,name),'utf8')]));
  files.set('bad.xml','<BufScadaPOUS><POUS>');
  files.set('injection.xml',`<BufScadaPOUS><POUS><OnePOU ID="1" NAME="&lt;img src=x onerror=alert(1)&gt;"><STCODE>&lt;script&gt;window.previewInjected=true&lt;/script&gt;</STCODE></OnePOU></POUS></BufScadaPOUS>`);
  files.set('doctype.xml','<!DOCTYPE root [<!ENTITY example SYSTEM="https://example.com/private">]><BufScadaPOUS/>');
  server=http.createServer((req,res)=>{
    const pathname=new URL(req.url,'http://127.0.0.1').pathname;
    res.setHeader('Content-Security-Policy',"default-src 'self'; script-src 'self'; style-src 'self'; object-src 'none'; connect-src 'self'");
    if(pathname.startsWith('/api/output/')){evidence.requests.push(pathname);const value=files.get(decodeURIComponent(pathname.slice('/api/output/'.length)));res.writeHead(value===undefined?404:200,{'Content-Type':'application/xml; charset=utf-8'});res.end(value||'');return;}
    if(pathname==='/'){res.writeHead(200,{'Content-Type':'text/html; charset=utf-8'});res.end('<!doctype html><html lang="ru"><head><meta charset="utf-8"><link rel="stylesheet" href="/preview/viewer.css"><script type="module" src="/preview/viewer.js"></script></head><body><button id="origin">Показать XML</button></body></html>');return;}
    if(!/^\/preview\/[a-z]+\.(js|css)$/.test(pathname)){res.writeHead(404);res.end();return;}
    const file=path.join(root,'web',pathname);res.writeHead(200,{'Content-Type':pathname.endsWith('.css')?'text/css':'text/javascript'});fs.createReadStream(file).pipe(res);
  });
  await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));const base=`http://127.0.0.1:${server.address().port}`;
  const launched=await launchBrowser(artifact);browser=launched.processHandle;cdp=launched.cdp;
  cdp.listeners.push(message=>{if(message.method==='Runtime.exceptionThrown')evidence.errors.push(message.params.exceptionDetails);});
  await cdp.send('Runtime.enable');await cdp.send('Page.enable');await cdp.send('Emulation.setDeviceMetricsOverride',{width:1400,height:950,deviceScaleFactor:1,mobile:false});await cdp.send('Page.navigate',{url:base});await until('viewer scripts',()=>evaluate('import("/preview/viewer.js").then(viewer=>typeof viewer.open === "function")'));
  await check('actual library FBD keeps saved blocks, links, NOT-to-D32 connection and POU selection',async()=>{
    await evaluate('document.querySelector("#origin").focus()');const result=await open('fbd.xml');assert.equal(result.kind,'FBD');assert.equal(result.pages,2);assert.equal(result.blocks,3);assert.equal(await evaluate('document.querySelectorAll(".gp-block").length'),result.blocks);assert.equal(await evaluate('document.querySelectorAll(".gp-link").length'),result.links);assert.ok(await evaluate('document.querySelectorAll(".gp-not").length > 0'));assert.ok(await evaluate('document.querySelector(".gp-svg").textContent.includes("D32")'));
    await evaluate('document.querySelector(".gp-not").dispatchEvent(new KeyboardEvent("keydown",{key:"Enter",bubbles:true}))');assert.ok(await evaluate('document.querySelector(".gp-inspector").textContent.includes("-32")'));
    await screenshot('fbd-not-inspection');
    await evaluate('const select=document.querySelector(".gp-tools select");select.value="1";select.dispatchEvent(new Event("change"))');assert.equal(await evaluate('document.querySelectorAll(".gp-not").length'),0);assert.ok(await evaluate('document.querySelector(".gp-svg").getAttribute("aria-label").includes("PLC715_DI_2")'));
  });
  await check('zoom, fit, keyboard pan and saved link-port inspection work',async()=>{
    assert.ok(Number((await evaluate('document.querySelector(".gp-zoom").textContent')).replace('%','')) >= 100);await evaluate('document.querySelector(".gp-tools button:last-child").click()');const before=await evaluate('document.querySelector(".gp-svg").getAttribute("viewBox")');await evaluate('document.querySelector("button[aria-label=Увеличить]").click()');assert.notEqual(await evaluate('document.querySelector(".gp-svg").getAttribute("viewBox")'),before);
    await evaluate('document.querySelector(".gp-canvas").dispatchEvent(new KeyboardEvent("keydown",{key:"ArrowRight",bubbles:true}))');await evaluate('document.querySelector(".gp-tools button:last-child").click()');assert.equal(await evaluate('document.querySelector(".gp-svg").getAttribute("viewBox")'),before);
    await evaluate('document.querySelector(".gp-link").dispatchEvent(new KeyboardEvent("keydown",{key:"Enter",bubbles:true}))');assert.ok(await evaluate('document.querySelector(".gp-inspector").textContent.includes("PointList") && document.querySelector(".gp-inspector").textContent.includes("FP")'));
  });
  await check('native ST shows the exact saved STCODE',async()=>{const result=await open('st.xml');assert.equal(result.kind,'ST');const expected=await evaluate(`new DOMParser().parseFromString(${JSON.stringify(files.get('st.xml').replace(/^\uFEFF/,''))},'application/xml').getElementsByTagName('STCODE')[0].textContent`);assert.equal(await evaluate('document.querySelector(".gp-code").textContent'),expected);assert.ok(expected.includes('REAL_TO_DINT'));await screenshot('st-source');});
  await check('native HMI uses saved frames and honest library-symbol geometry fallback',async()=>{const result=await open('hmi.xml');assert.equal(result.kind,'HMI');assert.ok(result.pages>=1);assert.equal(await evaluate('document.querySelectorAll(".gp-primitive").length'),5);assert.ok(await evaluate('document.querySelector(".gp-status").textContent.includes("границами")'));await evaluate('document.querySelectorAll(".gp-primitive")[1].dispatchEvent(new KeyboardEvent("keydown",{key:"Enter",bubbles:true}))');assert.ok(await evaluate('document.querySelector(".gp-inspector").textContent.includes("CardID")'));await evaluate('{const field=document.querySelector(".gp-navigation input");field.value=document.querySelectorAll(".gp-primitive")[1].dataset.id;field.dispatchEvent(new Event("input"));}');assert.ok(await evaluate('document.querySelectorAll(".gp-search-match").length > 0'));assert.ok(await evaluate('document.querySelector(".gp-status").textContent.includes("границами")'));await screenshot('hmi-frame');});
  await check('mobile dialog remains usable and Escape restores the caller focus',async()=>{await cdp.send('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:false});await evaluate('document.querySelector(".gp-tools button:last-child").click()');assert.ok(await evaluate('document.querySelector(".gp-dialog").getBoundingClientRect().width <= innerWidth'));assert.ok(await evaluate('document.querySelector(".gp-canvas").clientHeight >= 200'));await screenshot('hmi-mobile');await cdp.send('Input.dispatchKeyEvent',{type:'keyDown',key:'Escape',code:'Escape',windowsVirtualKeyCode:27});await cdp.send('Input.dispatchKeyEvent',{type:'keyUp',key:'Escape',code:'Escape',windowsVirtualKeyCode:27});assert.equal(await evaluate('document.querySelector(".gp-dialog").open'),false);assert.equal(await evaluate('document.activeElement.id'),'origin');});
  await check('invalid XML, DTD, unsafe markup and foreign output URLs do not execute or fetch externally',async()=>{await open('bad.xml');assert.ok(await evaluate('document.querySelector(".gp-status").textContent.includes("некорректный")'));await open('doctype.xml');assert.ok(await evaluate('document.querySelector(".gp-status").textContent.includes("DTD")'));await open('injection.xml');assert.equal(await evaluate('document.querySelectorAll(".gp-dialog img,.gp-dialog script").length'),0);assert.equal(await evaluate('!!window.previewInjected'),false);const count=evidence.requests.length;await evaluate('import("/preview/viewer.js").then(viewer=>viewer.open({url:"https://example.com/api/output/fbd.xml"}))');assert.equal(evidence.requests.length,count);assert.ok(await evaluate('document.querySelector(".gp-status").textContent.includes("этого генератора")'));});
  await check('opening again reads changed saved bytes instead of the generation form state',async()=>{files.set('st.xml',files.get('st.xml').replace('REAL_TO_DINT','SAVED_FILE_CHANGED'));await open('st.xml');assert.ok(await evaluate('document.querySelector(".gp-code").textContent.includes("SAVED_FILE_CHANGED")'));});
  await check('module DO opens at readable scale and search moves to an exact signal without altering geometry', async () => {
    await cdp.send('Emulation.setDeviceMetricsOverride',{width:1400,height:950,deviceScaleFactor:1,mobile:false});
    const result = await open('module-do.xml'); assert.equal(result.blocks,67);
    assert.ok(Number((await evaluate('document.querySelector(".gp-zoom").textContent')).replace('%','')) >= 100);
    assert.equal(await evaluate('document.querySelectorAll(".gp-module").length'),1);
    assert.equal(await evaluate('document.querySelectorAll(".gp-not").length'),32);
    assert.equal(await evaluate('document.querySelector(".gp-inspector").hidden'),true);
    const before = await evaluate('document.querySelector(".gp-svg").getAttribute("viewBox")');
    await screenshot('module-do-readable');
    await evaluate('const field=document.querySelector(".gp-navigation input");field.value="6032A";field.dispatchEvent(new Event("input"))');
    assert.notEqual(await evaluate('document.querySelector(".gp-svg").getAttribute("viewBox")'),before);
    assert.match(await evaluate('document.querySelector(".gp-status").textContent'),/6032A/);
    assert.equal(await evaluate('document.querySelectorAll(".gp-search-match").length'),1);
    await screenshot('module-do-search');
  });
  assert.deepEqual(evidence.errors,[]);
}
main().catch(async error=>{evidence.failure=error.stack;process.exitCode=1;console.error(error.stack);if(cdp)try{await screenshot('failure');}catch{}}).finally(async()=>{fs.mkdirSync(artifact,{recursive:true});fs.writeFileSync(path.join(artifact,'verification.json'),JSON.stringify(evidence,null,2));if(cdp){try{await cdp.send('Browser.close');}catch{}cdp.socket.close();}if(browser&&!browser.killed)browser.kill();if(server)server.close();console.log('Evidence: '+artifact);});
