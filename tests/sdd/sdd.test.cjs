'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { spawnSync } = require('node:child_process');
const { checkProject, newChange } = require('../../scripts/sdd.cjs');
const root = path.resolve(__dirname, '../..');

function fixture(t) {
  const artifacts = path.join(root, '.artifacts');
  fs.mkdirSync(artifacts, { recursive: true });
  const directory = fs.mkdtempSync(path.join(artifacts, 'sdd-smoke-'));
  t.after(() => {
    const resolved = fs.realpathSync(directory);
    assert.ok(resolved.startsWith(fs.realpathSync(artifacts) + path.sep));
    fs.rmSync(resolved, { recursive: true });
  });
  fs.cpSync(path.join(root, '.specify'), path.join(directory, '.specify'), { recursive: true });
  fs.cpSync(path.join(root, '.agents'), path.join(directory, '.agents'), { recursive: true });
  fs.mkdirSync(path.join(directory, '.vscode'));
  fs.copyFileSync(path.join(root, '.vscode/tasks.json'), path.join(directory, '.vscode/tasks.json'));
  fs.mkdirSync(path.join(directory, 'docs'));
  fs.writeFileSync(path.join(directory, 'AGENTS.md'), '# Fixture\n');
  fs.writeFileSync(path.join(directory, 'docs/SDD.md'), '# Fixture\n');
  fs.writeFileSync(path.join(directory, '.specify/memory/constitution.md'), '# Fixture\n');
  fs.rmSync(path.join(directory, '.specify/feature.json'), { force: true });
  return directory;
}

function feature(directory) {
  const target = path.join(directory, 'specs/001-test');
  fs.mkdirSync(target, { recursive: true });
  const write = (name, text) => fs.writeFileSync(path.join(target, name), text);
  write('spec.md', '# Test\nStatus: verified\n\n- **FR-001**: Retain source files.\n');
  write('plan.md', '# Plan\nInspect the files.\n');
  write('tasks.md', '# Tasks\n- [x] T001 [US1] Check source files (FR-001).\n');
  write('verification.md', '# Verification\n| Requirement | Check | Result | Evidence |\n|---|---|---|---|\n| FR-001 | Compare hashes | PASS | Identical hashes. |\n');
  return { target, write };
}

test('verified requires complete tasks and recorded passing evidence', t => {
  const directory = fixture(t);
  const { write } = feature(directory);
  assert.deepEqual(checkProject(directory).errors, []);
  write('verification.md', '| FR-001 | Compare hashes | NOT-RUN | Pending. |\n');
  assert.ok(checkProject(directory).errors.some(error => error.includes('must PASS')));
  write('verification.md', '| FR-001 | Compare hashes | PASS | Identical. |\n');
  write('tasks.md', '- [ ] T001 [US1] Check source files (FR-001).\n');
  assert.ok(checkProject(directory).errors.some(error => error.includes('Incomplete tasks')));
  write('tasks.md', '- [x] T001 Check (FR-001).\n- [-] T002 Work in progress.\n');
  assert.ok(checkProject(directory).errors.some(error => error.includes('Incomplete tasks')));
  write('tasks.md', '- [x] T001 Check (FR-001).\n- [ ] T1000 Remaining work.\n');
  assert.ok(checkProject(directory).errors.some(error => error.includes('Incomplete tasks')));
  write('tasks.md', '- [x] T001 Check (FR-001).\n- [ ] Untyped task.\n');
  assert.ok(checkProject(directory).errors.some(error => error.includes('Unrecognized checkbox task')));
});

test('unknown IDs and broken local links fail without treating draft as completed', t => {
  const directory = fixture(t);
  const { write } = feature(directory);
  write('tasks.md', '- [x] T001 [US1] Check source files (FR-999).\n');
  assert.ok(checkProject(directory).errors.some(error => error.includes('Unknown requirement FR-999')));
  write('spec.md', '# Test\n**Status**: Draft\n[missing](missing.md)\n');
  assert.ok(checkProject(directory).errors.some(error => error.includes('Broken local link')));
});

test('official creator works in paths with spaces, selects feature and preserves files/branch', t => {
  const directory = fixture(t);
  const git = args => spawnSync('git', args, { cwd: directory, encoding: 'utf8', windowsHide: true });
  assert.equal(git(['init', '--quiet']).status, 0);
  const beforeBranch = git(['symbolic-ref', 'HEAD']).stdout;
  fs.writeFileSync(path.join(directory, 'sentinel.txt'), 'existing user file');
  const result = newChange(directory, 'first-change', 'Check spaces and $(literal) title');
  const state = JSON.parse(fs.readFileSync(path.join(directory, '.specify/feature.json'), 'utf8').replace(/^\uFEFF/u, ''));
  assert.equal(state.feature_directory.replaceAll('\\', '/'), 'specs/001-first-change');
  const firstSpec = path.join(directory, 'specs/001-first-change/spec.md');
  assert.ok(fs.existsSync(firstSpec));
  const firstBytes = fs.readFileSync(firstSpec);
  const shell = process.platform === 'win32' ? 'powershell.exe' : 'pwsh';
  const runScript = name => spawnSync(shell, ['-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', path.join(directory, '.specify/scripts/powershell/' + name + '.ps1'), '-Json'], {
    cwd: directory, encoding: 'utf8', windowsHide: true,
    env: { ...process.env, SPECIFY_INIT_DIR: directory, SPECIFY_FEATURE_DIRECTORY: '', SPECIFY_FEATURE: '' }
  });
  const plan = runScript('setup-plan');
  assert.equal(plan.status, 0, plan.stderr);
  const planPath = JSON.parse(plan.stdout.trim()).IMPL_PLAN;
  fs.writeFileSync(planPath, '# Existing plan\nKeep this decision.\n');
  assert.equal(runScript('setup-plan').status, 0);
  assert.equal(fs.readFileSync(planPath, 'utf8'), '# Existing plan\nKeep this decision.\n');
  const tasks = runScript('setup-tasks');
  assert.equal(tasks.status, 0, tasks.stderr);
  let taskData;
  try { taskData = JSON.parse(tasks.stdout.trim()); }
  catch (error) { throw new Error(error.message + ': ' + JSON.stringify(tasks.stdout.slice(6700, 6820)) + '; stderr=' + tasks.stderr); }
  assert.ok(taskData.TASKS_TEMPLATE_CONTENT);
  assert.equal(runScript('check-prerequisites').status, 0);
  newChange(directory, 'second-change', 'Second change');
  assert.ok(fs.existsSync(path.join(directory, 'specs/002-second-change/spec.md')));
  assert.deepEqual(fs.readFileSync(firstSpec), firstBytes);
  assert.equal(fs.readFileSync(path.join(directory, 'sentinel.txt'), 'utf8'), 'existing user file');
  assert.equal(git(['symbolic-ref', 'HEAD']).stdout, beforeBranch);
  assert.equal(git(['rev-parse', '--verify', 'HEAD']).status, 128);
  assert.equal(checkProject(directory).errors.length, 0);
  assert.ok(checkProject(directory).warnings.length > 0);
  assert.ok(result);
  assert.throws(() => newChange(directory, '../escape', 'Escape'), /slug/);
});
