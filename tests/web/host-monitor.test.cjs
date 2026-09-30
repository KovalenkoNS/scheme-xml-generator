/* Регрессии обновления соединения: время, отмена и поздние ответы проверяются на настоящем web/host monitor. */
const test = require('node:test');
const assert = require('node:assert/strict');

// Создаёт управляемые запросы и часы, сохраняя настоящий цикл monitor без DOM/фиктивной реализации.
async function fixture() {
  const { createSessionMonitor } = await import('../../web/host/monitor.js');
  let nextTimer = 0;
  const timers = new Map(), calls = [], published = [];
  const monitor = createSessionMonitor({
    read: signal => new Promise((resolve, reject) => calls.push({ signal, resolve, reject })),
    publish: state => published.push(state),
    schedule: (callback, delay) => { const id = ++nextTimer; timers.set(id, { callback, delay }); return id; },
    cancel: id => timers.delete(id),
  });
  return { monitor, timers, calls, published };
}

// Пропускает завершение промиса чтения и finally, не используя реальные секунды в тестах цикла.
async function settle() { await new Promise(resolve => setImmediate(resolve)); }

test('initial request and periodic updates replace login, logout and restored connection without restart', async () => {
  const f = await fixture(); f.monitor.start(); f.monitor.start();
  assert.equal(f.calls.length, 1);
  for (const snapshot of [{ connected: false, status: 'authentication-required' }, { connected: true, status: 'connected', username: 'operator' }, { connected: false, status: 'authentication-required' }, { connected: true, status: 'connected', server: 'server-b' }]) {
    f.calls.at(-1).resolve(snapshot); await settle();
    assert.deepEqual(f.published.at(-1), snapshot);
    const poll = [...f.timers.values()].find(item => item.delay === 3000);
    assert.ok(poll, 'automatic three-second update was not scheduled'); poll.callback();
  }
  f.monitor.stop(); assert.equal(f.timers.size, 0);
});

test('manual or focus refresh aborts previous request and ignores its late successful response', async () => {
  const f = await fixture(); f.monitor.start();
  void f.monitor.refresh();
  assert.equal(f.calls[0].signal.aborted, true);
  const current = { connected: false, status: 'authentication-required' };
  f.calls[1].resolve(current); await settle();
  f.calls[0].resolve({ connected: true, status: 'connected', username: 'former-user' }); await settle();
  assert.deepEqual(f.published, [current]);
  f.monitor.stop();
});

test('failed refresh removes previous success and continues automatic recovery', async () => {
  const f = await fixture(); f.monitor.start();
  f.calls[0].resolve({ connected: true, server: 'old-server', username: 'old-user' }); await settle();
  void f.monitor.refresh(); f.calls[1].reject(new Error('sensitive transport trace')); await settle();
  assert.equal(f.published.at(-1).connected, false);
  assert.equal(f.published.at(-1).username, undefined);
  assert.equal(f.published.at(-1).server, undefined);
  assert.equal(f.published.at(-1).message, 'Не удалось обновить соединение');
  [...f.timers.values()].find(item => item.delay === 3000).callback();
  f.calls[2].resolve({ connected: true, server: 'new-server' }); await settle();
  assert.equal(f.published.at(-1).server, 'new-server'); f.monitor.stop();
});

test('deadline aborts stalled fetch and closed page cannot publish or reschedule', async () => {
  const f = await fixture(); f.monitor.start();
  [...f.timers.values()].find(item => item.delay === 4000).callback();
  assert.equal(f.calls[0].signal.aborted, true);
  f.calls[0].reject(new Error('aborted')); await settle();
  assert.equal(f.published.at(-1).connected, false);
  void f.monitor.refresh(); f.monitor.stop();
  f.calls[1].resolve({ connected: true }); await settle();
  assert.equal(f.published.length, 1); assert.equal(f.timers.size, 0);
});
