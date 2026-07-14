import test from 'node:test'
import assert from 'node:assert/strict'
import {
  dedupeUltraPlanTasks,
  reconcileUltraPlanTasks,
  preferredUltraPlanOutputFile,
  ultraPlanTaskDescriptionKey,
} from './ultraPlanTasks.js'

test('normalizes cosmetic task numbering for duplicate detection', () => {
  assert.equal(
    ultraPlanTaskDescriptionKey({ desc: 'Task 2:  Review   evidence' }),
    'review evidence',
  )
})

test('deduplicates a phase and keeps rich output plus terminal status', () => {
  const tasks = dedupeUltraPlanTasks([
    { desc: 'Collect evidence', status: 'done', output: 'short' },
    { desc: 'Collect   evidence', status: 'running', id: 'S1', output_file: 'S1.out.txt', output: 'longer output' },
  ])
  assert.equal(tasks.length, 1)
  assert.equal(tasks[0].id, 'S1')
  assert.equal(tasks[0].output_file, 'S1.out.txt')
  assert.equal(tasks[0].output, 'longer output')
  assert.equal(tasks[0].status, 'done')
})

test('enriches a phase task from recent tasks and hides the duplicate recent row', () => {
  const result = reconcileUltraPlanTasks([
    { name: 'explore', tasks: [{ desc: 'Failure modes lens', status: 'running' }] },
  ], [
    { id: 'S2', desc: 'failure modes lens', output_file: 'S2.out.txt', output: 'live log', status: 'done' },
  ])
  assert.equal(result.recentTasks.length, 0)
  assert.equal(result.phases[0].tasks.length, 1)
  assert.equal(result.phases[0].tasks[0].id, 'S2')
  assert.equal(result.phases[0].tasks[0].output, 'live log')
  assert.equal(result.phases[0].tasks[0].status, 'done')
})

test('removes the same strong task leaked into a later phase', () => {
  const result = reconcileUltraPlanTasks([
    { name: 'explore', tasks: [{ id: 'S3', desc: 'Architecture lens', status: 'running' }] },
    { name: 'synthesize', tasks: [{ id: 'S3', desc: 'Architecture lens', status: 'done', output: 'result' }] },
  ], [])
  assert.equal(result.phases[0].tasks.length, 1)
  assert.equal(result.phases[0].tasks[0].status, 'done')
  assert.equal(result.phases[0].tasks[0].output, 'result')
  assert.equal(result.phases[1].tasks.length, 0)
})

test('keeps genuinely unmatched recent work visible', () => {
  const result = reconcileUltraPlanTasks([
    { name: 'explore', tasks: [{ desc: 'Architecture lens' }] },
  ], [
    { id: 'S4', desc: 'Independent verification', status: 'running' },
  ])
  assert.equal(result.recentTasks.length, 1)
  assert.equal(result.recentTasks[0].id, 'S4')
})


test('prefers generated .out.txt and upgrades a persisted input prompt path', () => {
  assert.equal(
    preferredUltraPlanOutputFile({
      output_file: String.raw`E:\run\001_architecture_lens.txt`,
      file: String.raw`E:\run\001_architecture_lens.out.txt`,
    }),
    String.raw`E:\run\001_architecture_lens.out.txt`,
  )
  assert.equal(
    preferredUltraPlanOutputFile({ output_file: String.raw`E:\run\002_failure_modes.txt` }),
    String.raw`E:\run\002_failure_modes.out.txt`,
  )
})
