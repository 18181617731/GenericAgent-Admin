import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

function functionBlock(source, start, end) {
  const from = source.indexOf(start)
  assert.notEqual(from, -1, `missing start marker: ${start}`)
  const to = source.indexOf(end, from + start.length)
  assert.notEqual(to, -1, `missing end marker: ${end}`)
  return source.slice(from, to)
}

test('stream events update the visible model context without a page reload', () => {
  const source = readFileSync(new URL('../ChatApp.jsx', import.meta.url), 'utf8')
  const applyStreamEvent = functionBlock(
    source,
    '  const applyStreamEvent = (ev, pendingId, clientUserID = \'\', sessionId = \'\') => {',
    '  const createStreamBatcher =',
  )

  assert.match(applyStreamEvent, /hasOwnProperty\.call\(ev, 'raw_history'\)[\s\S]*setRawHistory\(Array\.isArray\(ev\.raw_history\) \? ev\.raw_history : \[\]\)/)
  assert.match(applyStreamEvent, /hasOwnProperty\.call\(ev, 'history_info'\)[\s\S]*setHistoryInfo\(Array\.isArray\(ev\.history_info\) \? ev\.history_info : \[\]\)/)
  assert.match(applyStreamEvent, /hasOwnProperty\.call\(ev, 'working'\)[\s\S]*setWorkingState\(ev\.working && typeof ev\.working === 'object' \? ev\.working : null\)/)
})
