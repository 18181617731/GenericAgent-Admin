import test from 'node:test'
import assert from 'node:assert/strict'
import { modelDiagnosisAdvice, modelDiagnosisTitle } from './modelDiagnosis.js'
import { frontendSource } from './frontendSources.mjs'

const zh = (chinese) => chinese
const en = (_chinese, english) => english

test('a missing module names the package and the interpreter in both languages', () => {
  const diagnosis = { code: 'missing_python_module', python: 'C:/ga/.venv/Scripts/python.exe', install_packages: ['requests'], fixable: true }
  assert.match(modelDiagnosisTitle(diagnosis, zh), /缺少 Python 依赖/)
  assert.match(modelDiagnosisAdvice(diagnosis, zh), /requests/)
  assert.match(modelDiagnosisAdvice(diagnosis, zh), /python\.exe/)
  assert.match(modelDiagnosisAdvice(diagnosis, en), /requests/)
  assert.doesNotMatch(modelDiagnosisAdvice(diagnosis, en), /[\u4e00-\u9fff]/)
})

test('an unconfigured GA root reads differently from a wrong one', () => {
  const empty = modelDiagnosisAdvice({ code: 'ga_root_unusable', ga_root: '' }, en)
  const wrong = modelDiagnosisAdvice({ code: 'ga_root_unusable', ga_root: 'D:/tmp' }, en)
  assert.match(empty, /setup wizard/)
  assert.match(wrong, /D:\/tmp/)
  assert.match(wrong, /agentmain\.py/)
})

test('a GA that starts but has no model is sent to the models page, not to pip', () => {
  const advice = modelDiagnosisAdvice({ code: 'no_models_configured' }, en)
  assert.match(advice, /Models page/)
  assert.doesNotMatch(advice, /pip/)
})

test('an unrecognized code falls back to the backend hint', () => {
  assert.equal(modelDiagnosisAdvice({ code: 'something_new', hint: 'raw backend hint' }, en), 'raw backend hint')
  assert.match(modelDiagnosisTitle({ code: 'something_new' }, en), /Reading the model list failed/)
  assert.match(modelDiagnosisAdvice({ code: 'unknown' }, en), /Python output/)
})

test('the composer renders the diagnosis instead of leaving the picker to say nothing', () => {
  const source = frontendSource()
  assert.match(source, /setChatBackend\(st\.backend/)
  assert.match(source, /const modelDiagnosis = !llms\.length \? \(chatBackend\?\.diagnosis \|\| null\) : null/)
  assert.match(source, /className=\{`oa-model-alert/)
  assert.match(source, /modelDiagnosisAdvice\(modelDiagnosis, ct\)/)
  // The repair button must stay tied to the backend's own fixable verdict, so a
  // GA root or interpreter problem never offers a pip install that cannot help.
  assert.match(source, /modelDiagnosis\.fixable && <button/)
})
