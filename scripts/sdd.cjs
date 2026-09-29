'use strict';

// Project-local adapter around GitHub Spec Kit 1.0.12. No npm dependencies.
const fs = require('node:fs');
const path = require('node:path');
const { spawnSync } = require('node:child_process');
const states = new Set(['draft', 'ready', 'implemented', 'verified', 'superseded']);
const documents = ['spec.md', 'plan.md', 'tasks.md', 'verification.md'];
const read = file => fs.readFileSync(file, 'utf8').replace(/^\uFEFF/u, '');
const prose = text => text.replace(/```[^\n]*\n[\s\S]*?```/gu, '');

// checkProject validates Spec Kit states, requirement coverage and local document links.
// It remains document-only so isolated workflow fixtures need no product source tree.
function checkProject(root) {
  const errors = [], warnings = [];
  const fail = (file, message) => errors.push(path.relative(root, file) + ': ' + message);
  const note = (file, message) => warnings.push(path.relative(root, file) + ': ' + message);
  const requireFile = relative => {
    const file = path.join(root, relative);
    if (!fs.existsSync(file) || !fs.statSync(file).isFile()) { fail(file, 'Missing file'); return null; }
    return read(file);
  };
  const linked = new Map();
  for (const file of ['AGENTS.md', 'docs/SDD.md', '.specify/memory/constitution.md']) {
    const text = requireFile(file);
    if (text !== null) linked.set(path.join(root, file), text);
  }
  for (const file of ['spec', 'plan', 'tasks', 'verification']) requireFile('.specify/templates/' + file + '-template.md');
  for (const command of ['specify', 'plan', 'tasks', 'analyze', 'implement', 'converge', 'constitution']) {
    requireFile('.agents/skills/speckit-' + command + '/SKILL.md');
  }
  for (const script of ['create-new-feature', 'check-prerequisites', 'setup-plan', 'setup-tasks', 'common']) {
    requireFile('.specify/scripts/powershell/' + script + '.ps1');
  }
  for (const file of ['.specify/init-options.json', '.vscode/tasks.json']) {
    const text = requireFile(file);
    if (text !== null) {
      try {
        const data = JSON.parse(text);
        if (file.endsWith('init-options.json') && (data.speckit_version !== '1.0.12' || data.integration !== 'codex')) fail(path.join(root, file), 'Expected Spec Kit 1.0.12 / codex');
        if (file.endsWith('tasks.json') && !data.tasks?.some(task => task.label === 'SDD: check')) fail(path.join(root, file), 'Missing SDD: check task');
      } catch (error) { fail(path.join(root, file), error.message); }
    }
  }
  const specs = path.join(root, 'specs');
  const features = fs.existsSync(specs) ? fs.readdirSync(specs, { withFileTypes: true }).filter(item => item.isDirectory() && !item.isSymbolicLink() && /^\d{3,}-[a-z0-9-]+$/u.test(item.name)) : [];
  for (const feature of features) {
    const directory = path.join(specs, feature.name);
    const specFile = path.join(directory, 'spec.md');
    if (!fs.existsSync(specFile)) { fail(specFile, 'Missing spec.md'); continue; }
    const spec = read(specFile);
    const metadata = [...prose(spec).matchAll(/^(?:\*\*)?Status(?:\*\*)?:\s*(?:\*\*)?([^\r\n*]+)(?:\*\*)?\s*$/gmu)];
    const state = metadata[0]?.[1].trim().toLowerCase() || 'draft';
    if (metadata.length > 1 || !states.has(state)) fail(specFile, 'Invalid or duplicate Status');
    const strict = ['ready', 'implemented', 'verified'].includes(state);
    const texts = {};
    for (const name of documents) {
      const file = path.join(directory, name);
      if (!fs.existsSync(file)) {
        (strict ? fail : note)(file, 'Not created yet');
        texts[name] = '';
      } else {
        texts[name] = read(file);
        linked.set(file, texts[name]);
        if (strict && (name !== 'verification.md' || state === 'verified') && /\bTODO\b|\bTBD\b|\[NEEDS CLARIFICATION[^\]]*\]|\{\{[^}]+\}\}/u.test(texts[name])) fail(file, 'Unresolved placeholder');
      }
    }
    const requirements = [...prose(spec).matchAll(/^\s*-\s+(?:\*\*)?(FR-\d{3})(?:\*\*)?:\s+(.+)$/gmu)];
    const ids = new Set(requirements.map(match => match[1]));
    if (ids.size !== requirements.length) fail(specFile, 'Duplicate requirement ID');
    if (strict && !ids.size) fail(specFile, 'Expected - **FR-001**: requirement records');
    const taskProse = prose(texts['tasks.md']);
    const taskPattern = /^\s*-\s+\[([ xX-])\]\s+(T\d{3,})\b([^\r\n]*)$/u;
    for (const line of taskProse.split(/\r?\n/u)) {
      if (/^\s*-\s+\[[^\]]*\]/u.test(line) && !taskPattern.test(line)) fail(path.join(directory, 'tasks.md'), 'Unrecognized checkbox task: ' + line.trim());
    }
    const tasks = taskProse.split(/\r?\n/u).map(line => line.match(taskPattern)).filter(Boolean);
    if (new Set(tasks.map(match => match[2])).size !== tasks.length) fail(path.join(directory, 'tasks.md'), 'Duplicate task ID');
    if (strict && !tasks.length) fail(path.join(directory, 'tasks.md'), 'No T001 checkbox tasks');
    const taskText = tasks.map(match => match[3]).join('\n');
    const rows = new Map();
    for (const line of prose(texts['verification.md']).split(/\r?\n/u)) {
      const cells = line.trim().replace(/^\||\|$/gu, '').split(/(?<!\\)\|/u).map(value => value.trim());
      if (!/^FR-\d{3}$/u.test(cells[0])) continue;
      if (rows.has(cells[0])) fail(path.join(directory, 'verification.md'), 'Duplicate result ' + cells[0]);
      rows.set(cells[0], cells);
      if (!ids.has(cells[0])) fail(path.join(directory, 'verification.md'), 'Unknown requirement ' + cells[0]);
      if (!cells[1] || !['PASS', 'FAIL', 'NOT-RUN'].includes(cells[2]) || !cells[3]) fail(path.join(directory, 'verification.md'), 'Expected Requirement | Check | Result | Evidence');
    }
    for (const id of ids) {
      if (strict && !new RegExp('\\b' + id + '\\b', 'u').test(taskText)) fail(path.join(directory, 'tasks.md'), 'No task references ' + id);
      if (strict && !rows.has(id)) fail(path.join(directory, 'verification.md'), 'Missing result for ' + id);
      if (state === 'verified' && rows.get(id)?.[2] !== 'PASS') fail(path.join(directory, 'verification.md'), id + ' must PASS before verified');
    }
    for (const id of taskText.match(/\bFR-\d{3}\b/gu) || []) if (!ids.has(id)) fail(path.join(directory, 'tasks.md'), 'Unknown requirement ' + id);
    if (state === 'verified' && tasks.some(match => match[1].toLowerCase() !== 'x')) fail(path.join(directory, 'tasks.md'), 'Incomplete tasks in verified feature');
  }
  for (const [file, text] of linked) {
    for (const match of prose(text).matchAll(/\[[^\]]*\]\(([^)]+)\)/gu)) {
      const target = match[1].trim().replace(/^<|>$/gu, '').split('#')[0];
      if (!target || /^[a-z][a-z0-9+.-]*:/iu.test(target)) continue;
      let local;
      try { local = decodeURIComponent(target); } catch { fail(file, 'Malformed link ' + target); continue; }
      if (!fs.existsSync(path.resolve(path.dirname(file), local))) fail(file, 'Broken local link ' + target);
    }
  }
  return { features: features.length, errors, warnings };
}

// newChange invokes the vendored Spec Kit creator with explicit project scope.
// It validates user arguments and returns the creator's structured feature paths.
function newChange(root, slug, title) {
  if (!/^[a-z0-9]+(?:-[a-z0-9]+)*$/u.test(slug) || slug.length > 80) throw new Error('Use a slug of lowercase Latin letters, digits and hyphens (max 80 characters)');
  if (!title?.trim() || /[\r\n\0]/u.test(title) || title.startsWith('-')) throw new Error('Provide a single-line title');
  const shell = process.platform === 'win32' ? 'powershell.exe' : 'pwsh';
  const script = path.join(root, '.specify/scripts/powershell/create-new-feature.ps1');
  const result = spawnSync(shell, ['-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', script, '-Json', '-ShortName', slug, title], {
    cwd: root, encoding: 'utf8', windowsHide: true,
    env: { ...process.env, SPECIFY_INIT_DIR: root, SPECIFY_FEATURE_DIRECTORY: '', SPECIFY_FEATURE: '' }
  });
  if (result.error) throw result.error;
  if (result.status !== 0) throw new Error(result.stderr || result.stdout || 'Spec Kit creation failed');
  const output = JSON.parse(result.stdout.trim().replace(/^\uFEFF/u, ''));
  return output;
}

// checkSourceRules audits the real Go/UI inventory after document validation.
// A nonzero gate status blocks SDD acceptance and the build even when Markdown is valid.
function checkSourceRules(root) {
  const result = spawnSync('go', ['run', './tools/rulescheck'], { cwd: root, encoding: 'utf8', windowsHide: true });
  if (result.stdout) process.stdout.write(result.stdout);
  if (result.stderr) process.stderr.write(result.stderr);
  if (result.error) throw result.error;
  return result.status === 0;
}

// main provides the VS Code/CLI entry for checking or creating a specification.
// Checking combines recorded SDD evidence with the mandatory physical source gate.
function main(args = process.argv.slice(2)) {
  const root = path.resolve(__dirname, '..');
  try {
    if (args[0] === 'check' && args.length === 1) {
      const result = checkProject(root);
      for (const warning of result.warnings) console.warn('SDD note: ' + warning);
      for (const error of result.errors) console.error(error);
      if (result.errors.length) { console.error('SDD check failed: ' + result.errors.length + ' issue(s)'); process.exitCode = 1; }
      else if (!checkSourceRules(root)) process.exitCode = 1;
      else console.log('SDD check passed: ' + result.features + ' Spec Kit feature(s), mandatory source inventory checked. Product behavior and semantic review are recorded separately.');
    } else if (args[0] === 'new' && args.length === 3) {
      console.log(JSON.stringify(newChange(root, args[1], args[2]), null, 2));
      console.log('Continue in Codex with $speckit-plan and $speckit-tasks after filling spec.md.');
    } else throw new Error('Usage: node scripts/sdd.cjs check | new <slug> "Title"');
  } catch (error) { console.error('SDD: ' + error.message); process.exitCode = 1; }
}

module.exports = { checkProject, newChange };
if (require.main === module) main();
