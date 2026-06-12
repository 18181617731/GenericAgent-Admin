import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const common = readFileSync(new URL('../components/common.jsx', import.meta.url), 'utf8')
const processGuard = readFileSync(new URL('../components/ProcessGuard.jsx', import.meta.url), 'utf8')
const app = readFileSync(new URL('../App.jsx', import.meta.url), 'utf8')

test('service cards expose lifecycle metadata without mutating services', () => {
  assert.match(common, /serviceReturnCode/)
  assert.match(common, /svc\?\.returncode/)
  assert.match(common, /svc\?\.started_at/)
  assert.match(common, /svc\?\.workdir/)
  assert.match(common, /serviceLogPath/)
  assert.match(common, /ServiceMeta svc=\{svc\}/)
  assert.match(common, /ServiceMeta svc=\{svc\} compact/)
})

test('process guard table includes identity and command context', () => {
  assert.match(processGuard, /processCommand/)
  assert.match(processGuard, /processPath/)
  assert.match(processGuard, /<th>可执行文件<\/th><th>命令<\/th>/)
  assert.match(processGuard, /colSpan="6"/)
  assert.match(processGuard, /\u6700\u8fd1\u626b\u63cf\uff1a\{snapshot\.scanned_at\} \u00b7 \u603b\u8ba1 \{counts\.total\}/)
})

test('service actions remain dangerous-confirm guarded', () => {
  assert.match(app, /confirmDanger\(`service-\$\{action\}`/)
  assert.match(app, /api\(`\/api\/services\/\$\{action\}`.*, \{ dangerous:true, method:'POST'/s)
  assert.match(app, /confirmDanger\('service-autostart'/)
  assert.match(app, /api\('\/api\/services\/autostart', \{ dangerous:true, method:'POST'/)
  assert.match(processGuard, /confirmDanger\('ga-process-adopt'/)
  assert.match(processGuard, /api\('\/api\/ga\/processes\/adopt', \{ dangerous:true, method:'POST'/)
  assert.match(processGuard, /confirmDanger\('ga-process-kill'/)
  assert.match(processGuard, /api\('\/api\/ga\/processes\/kill', \{ dangerous:true, method:'POST'/)
})
