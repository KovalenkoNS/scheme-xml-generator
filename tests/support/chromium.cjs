// Minimal Chromium DevTools test transport. It controls an isolated headless
// profile and has no dependency on Host or the user's interactive browser.
const fs = require('node:fs');
const path = require('node:path');
const { spawn } = require('node:child_process');
// Pause spaces readiness probes in the isolated browser test process.
const pause = ms => new Promise(resolve => setTimeout(resolve, ms));
// Until polls an observable test condition with a deadline and names a failing dependency.
async function until(description, callback, timeout = 30000) {
  const end = Date.now() + timeout;
  while (Date.now() < end) { if (await callback()) return; await pause(100); }
  throw new Error(`Timed out: ${description}`);
}
class CDP {
  // Constructor correlates browser replies with pending test requests and forwards events.
  constructor(socket) {
    this.socket = socket; this.sequence = 0; this.pending = new Map(); this.listeners = [];
    socket.addEventListener('message', event => {
      const message = JSON.parse(event.data);
      if (message.id) { const item = this.pending.get(message.id); if (!item) return; this.pending.delete(message.id); clearTimeout(item.timer); message.error ? item.reject(new Error(JSON.stringify(message.error))) : item.resolve(message.result); }
      else this.listeners.forEach(listener => listener(message));
    });
  }
  // Send transmits one DevTools request and rejects missing or failed browser replies.
  send(method, params = {}) {
    return new Promise((resolve, reject) => {
      const id = ++this.sequence;
      const timer = setTimeout(() => { this.pending.delete(id); reject(new Error(`CDP timeout: ${method}`)); }, 60000);
      this.pending.set(id, { resolve, reject, timer }); this.socket.send(JSON.stringify({ id, method, params }));
    });
  }
  // Close releases only this test's DevTools connection; browser termination belongs to its caller.
  close() { this.socket.close(); }
}
// Evaluate executes a test probe in the page and returns its serializable value or browser exception.
async function evaluate(cdp, expression) {
  const result = await cdp.send('Runtime.evaluate', { expression, returnByValue: true, awaitPromise: true, userGesture: true });
  if (result.exceptionDetails) throw new Error(JSON.stringify(result.exceptionDetails));
  return result.result.value;
}
// Browser launches headless Chromium with an isolated profile and returns the owned process/CDP transport.
async function browser(directory) {
  const profile = path.join(directory, 'browser-profile');
  const command = process.env.CHROME_PATH || 'C:/Program Files/Google/Chrome/Application/chrome.exe';
  const processHandle = spawn(command, ['--headless=new', '--no-first-run', '--disable-background-networking', '--no-default-browser-check', '--remote-debugging-port=0', `--user-data-dir=${profile}`, 'about:blank'], { windowsHide: true, stdio: 'ignore' });
  const portFile = path.join(profile, 'DevToolsActivePort'); await until('browser port', () => fs.existsSync(portFile));
  const port = fs.readFileSync(portFile, 'utf8').split(/\r?\n/)[0];
  const pages = await (await fetch(`http://127.0.0.1:${port}/json/list`)).json();
  const socket = new WebSocket(pages.find(page => page.type === 'page').webSocketDebuggerUrl);
  await new Promise((resolve, reject) => { socket.addEventListener('open', resolve, { once: true }); socket.addEventListener('error', reject, { once: true }); });
  const cdp = new CDP(socket);
  await cdp.send('Page.enable'); await cdp.send('Runtime.enable'); await cdp.send('DOM.enable'); await cdp.send('Log.enable');
  await cdp.send('Emulation.setDeviceMetricsOverride', { width: 1280, height: 960, deviceScaleFactor: 1, mobile: false });
  return { cdp, processHandle };
}
// Upload selects actual fixture files through the page's file input to exercise its upload lifecycle.
async function upload(cdp, selector, files) {
  const { root } = await cdp.send('DOM.getDocument');
  const { nodeId } = await cdp.send('DOM.querySelector', { nodeId: root.nodeId, selector });
  await cdp.send('DOM.setFileInputFiles', { nodeId, files });
}
// Screenshot saves the visible browser viewport as evidence without capturing other applications.
async function screenshot(cdp, filename) {
  const result = await cdp.send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: false });
  fs.writeFileSync(filename, Buffer.from(result.data, 'base64'));
}
module.exports = { pause, until, browser, evaluate, upload, screenshot };
