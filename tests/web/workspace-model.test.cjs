// Request-boundary tests: protect PLC isolation, library provenance and physical
// IDs independently from DOM layout or visual styling.
const { test } = require('node:test');
const assert = require('node:assert/strict');
const M = {
  ...require('../../web/equipment/controllers.js'), ...require('../../web/shared/validation.js'),
  ...require('../../web/sources/model.js'), ...require('../../web/generation/fbd/document.js'),
  ...require('../../web/generation/fbd/placement.js'), ...require('../../web/generation/fbd/modules.js'),
  ...require('../../web/generation/fbd/jobs.js'), ...require('../../web/generation/st/model.js'),
  ...require('../../web/generation/hmi/model.js'),
};
const ctx = { version: '29', project: '', controllerId: '10', resourceId: '20', groupId: '30', pouNumber: '40', resourceNumber: '1' };
const catalog = { templates: [{ key: 'lib|2097|12772', ownerName: 'D32V', name: 'DIO-1', supported: true }, { key: 'custom|7|100', ownerName: 'CustomType', name: 'Owner-supplied', supported: true }] };
// Source isolates two PLCs with identical physical module names to detect data leakage.
function source() {
  return { id: 'book', file: { name: 'io.xlsx' }, assignments: { groups: [
    { key: 'p1_DO', controllerName: 'PLC_1', kind: 'DO', pouName: 'DO_A1', modules: [{ name: 'A1-01', channels: [{ channel: 0, tag: 'OUT_A' }, { channel: 3, tag: 'OUT_B' }] }] },
    { key: 'p2_DO', controllerName: 'PLC_2', kind: 'DO', pouName: 'DO_A1', modules: [{ name: 'A1-01', channels: [{ channel: 0, tag: 'NEVER_PLC2' }] }] },
  ] } };
}
// Options supply explicit module IDs and library selection; no hardware value is inferred.
const options = () => ({ cpu: M.CPU850, templates: { DO: catalog.templates[0].key }, context: { ...ctx }, fileName: 'test', modules: { 'book|assignments|p1_DO': { ids: ['7'] } }, inversion: {} });
// Existing browser preferences keep physical ID zero when obsolete adapter keys are migrated.
test('adapter key migration preserves actual ModuleID zero and never overwrites newer preferences', () => {
  const old = { 'book|skz|PLC:DO:A1': { count: 1, ids: ['0'] }, 'other|skz|PLC:AI:A2': { ids: ['99'] }, 'other|assignments|PLC:AI:A2': { ids: ['7'] } };
  const migrated = M.migrateModuleSettings(old);
  assert.deepEqual(migrated['book|assignments|PLC:DO:A1'], { count: 1, ids: ['0'] });
  assert.deepEqual(migrated['other|assignments|PLC:AI:A2'], { ids: ['7'] });
  assert.ok(old['book|skz|PLC:DO:A1']);
  assert.equal(migrated['book|skz|PLC:DO:A1'], undefined);
});
// Two controllers share module names; the FBD request must contain only the selected controller and its library key.
test('only the selected PLC contributes FBD instances and the exact library key is retained', () => {
  const result = M.libraryDORequest([source()], 'PLC_1', options(), catalog);
  assert.deepEqual(result.pous.flatMap(pou => pou.modules.flatMap(module => module.channels.map(channel => channel.tag))), ['OUT_A', 'OUT_B']);
  assert.equal(result.pous[0].modules.length, 1);
  assert.equal(result.templateKey, catalog.templates[0].key);
  assert.equal(result.context.controllerId, '10');
  assert.equal(result.context.controllerTypeName, M.CPU850);
});
// Missing catalog entries must stop request planning before a fixed template can be substituted.
test('unknown templates fail instead of falling back to fixed native types', () => {
  for (const key of ['', 'not-loaded']) assert.throws(() => M.libraryDORequest([source()], 'PLC_1', { ...options(), templates: { DO: key } }, catalog), /шаблон/);
  const custom = M.libraryDORequest([source()], 'PLC_1', { ...options(), templates: { DO: 'custom|7|100' } }, catalog);
  assert.equal(custom.templateKey, 'custom|7|100');
});
// The NOT preference changes channel behavior while retaining the exact chosen library template.
test('inversion changes a request option while keeping the chosen library template', () => {
  const result = M.libraryDORequest([source()], 'PLC_1', { ...options(), inversion: { DO: true } }, catalog);
  assert.ok(result.pous[0].modules[0].channels.every(channel => channel.invert === true));
  assert.deepEqual(result.pous[0].modules[0].channels.map(channel => channel.channel), [0, 3]);
  assert.equal(result.templateKey, catalog.templates[0].key);
});
// Repeated source data is deduplicated, but conflicting library provenance is rejected before generation.
test('duplicate copies of a tag do not create duplicate instances; conflicting templates fail', () => {
  const one = source(), two = source(); two.id = 'second';
  one.assignments.groups[0].kind = 'AI'; two.assignments.groups[0].kind = 'AI';
  const opts = { ...options(), templates: { AI: catalog.templates[0].key } };
  assert.equal(M.fbdRequest([one, two], 'PLC_1', opts, catalog).pous.length, 1);
  two.assignments.groups[0].kind = 'DI';
  assert.throws(() => M.fbdRequest([one, two], 'PLC_1', { ...opts, templates: { AI: catalog.templates[0].key, DI: catalog.templates[1].key } }, catalog), /разные шаблоны/);
});
// ST request planning rejects missing physical IDs and retains explicit zero without leaking another PLC.
test('ST requires actual module IDs and preserves explicit zero and the selected PLC', () => {
  const book = source(), opts = { ...options(), modules: {} };
  assert.throws(() => M.stJobs([book], 'PLC_1', opts), /ModuleID/);
  opts.modules['book|assignments|p1_DO'] = { count: 1, ids: ['0'] };
  const jobs = M.stJobs([book], 'PLC_1', opts);
  assert.deepEqual(jobs[0].fields.config.pous, [{ groupKey: 'p1_DO', moduleCount: 1, moduleIds: [0] }]);
  assert.equal(jobs.length, 1);
});
// Overlapping physical module IDs from separate inputs must not reach the ST generation API.
test('duplicate physical IDs across sources are rejected before a generation request', () => {
  const one = source(), two = source(); two.id = 'second'; two.assignments.groups[0].key = 'another_group';
  const opts = options(); opts.modules = { 'book|assignments|p1_DO': { ids: ['7'] }, 'second|assignments|another_group': { ids: ['7'] } };
  assert.throws(() => M.stJobs([one, two], 'PLC_1', opts), /повторяется/);
});
// The HMI planner exposes its current export limitation and isolates the supported controller selection.
test('HMI 850 is an explicit capability gap; HMI715 only requests the selected controller', () => {
  const io = { id: 'io', file: {}, io: { controllers: [{ key: 'a', name: 'PLC_1', modules: [] }, { key: 'b', name: 'PLC_2', modules: [] }] } };
  assert.throws(() => M.hmiJobs([io], 'PLC_1', options()), /850/);
  const jobs = M.hmiJobs([io], 'PLC_1', { ...options(), cpu: M.CPU715 });
  assert.deepEqual(jobs[0].fields.diagnostic.controllers, [{ key: 'a', name: 'PLC_1' }]);
});
// Partial assignment parsing must leave other source directions available to library FBD planning.
test('a partial assignment parser does not hide remaining IO directions needed for library FBD', () => {
  const book = source(); book.io = { controllers: [{ key: 'a', name: 'PLC_1', modules: [{ name: 'A1-02', type: 'AI16H', channels: [{ tag: 'AI_FROM_IO' }] }, { name: 'A1-01', type: 'DO32P', channels: [{ tag: 'OUT_A' }] }] }] };
  assert.deepEqual(M.kinds([book], 'PLC_1'), ['AI', 'DO']);
});

// Placement uses measured library dimensions and fails before creating coordinates outside the XML limit.
test('large IO groups use the actual library bounds without overlapping templates or exceeding the coordinate limit', () => {
  const template = { layout: { minX: -100, minY: -30, maxX: 700, maxY: 1000 } };
  const positions = Array.from({ length: 300 }, (_, index) => M.signalOffsets(template, index));
  assert.ok(positions.every(position => position.offsetX <= 100000 && position.offsetY <= 100000));
  assert.equal(new Set(positions.map(position => `${position.offsetX},${position.offsetY}`)).size, 300);
  assert.ok(positions[24].offsetX > positions[0].offsetX + 800);
});

// Mixed FBD plans separate DO module requests from other templates while allocating disjoint POU numbers.
test('mixed FBD uses a separate module endpoint and non-overlapping POU numbers', () => {
  const book = source(); book.assignments.groups.push({ ...book.assignments.groups[0], key: 'AI', kind: 'AI', pouName: 'AI_A1', modules: [{ name: 'A2-01', channels: [{ channel: 0, tag: 'INPUT' }] }] });
  const opts = options(); opts.templates.AI = catalog.templates[1].key;
  const jobs = M.fbdJobs([book], 'PLC_1', opts, catalog);
  assert.deepEqual(jobs.map(job => job.url), ['/api/generate', '/api/generate/library-do']);
  assert.deepEqual(jobs.map(job => job.json.pous[0].pouNumber), [40, 41]);
  assert.equal(jobs[1].json.pous[0].modules.length, 1);
  assert.throws(() => M.fbdJobs([book], 'PLC_1', { ...opts, context: { ...ctx, groupId: '0' } }, catalog), /положительное/);
});
