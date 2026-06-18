import { spawn } from 'node:child_process';
import { readdirSync, statSync } from 'node:fs';
import { join, sep } from 'node:path';

function collectTests(dir) {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) return collectTests(path);
    if (entry.isFile() && entry.name.endsWith('.test.mjs')) return [path];
    return [];
  });
}

const root = join('src', 'lib');
if (!statSync(root, { throwIfNoEntry: false })?.isDirectory()) {
  console.error(`Missing test directory: ${root}`);
  process.exit(1);
}

const files = collectTests(root).sort((a, b) => a.localeCompare(b));
if (files.length === 0) {
  console.error(`No .test.mjs files found under ${root}`);
  process.exit(1);
}

const child = spawn(process.execPath, ['--test', ...files], {
  stdio: 'inherit',
  shell: false,
});

child.on('exit', (code, signal) => {
  if (signal) {
    console.error(`node --test terminated by signal ${signal}`);
    process.exit(1);
  }
  process.exit(code ?? 1);
});
