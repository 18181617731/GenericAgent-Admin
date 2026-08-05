import test from 'node:test'
import assert from 'node:assert/strict'
import { createChatCompletionTone } from './chatCompletionTone.js'

class FakeAudio {
  static instances = []

  constructor(source) {
    this.src = source
    this.preload = ''
    this.volume = 1
    this.muted = false
    this.currentTime = 0
    this.loadCalls = 0
    this.playCalls = 0
    this.pauseCalls = 0
    FakeAudio.instances.push(this)
  }

  load() { this.loadCalls += 1 }
  play() { this.playCalls += 1; return Promise.resolve() }
  pause() { this.pauseCalls += 1 }
}

test('primes and plays a downloaded completion notification sound', async () => {
  FakeAudio.instances = []
  const tone = createChatCompletionTone({ Audio: FakeAudio }, '/audio/test-notification.wav')

  assert.equal(tone.prime(), true)
  const audio = FakeAudio.instances[0]
  assert.equal(audio.src, '/audio/test-notification.wav')
  assert.equal(audio.preload, 'auto')
  assert.equal(audio.loadCalls, 1)
  assert.equal(audio.playCalls, 1)

  await Promise.resolve()
  await Promise.resolve()
  assert.equal(audio.pauseCalls, 1)
  assert.equal(audio.muted, false)
  assert.equal(audio.currentTime, 0)

  assert.equal(tone.play(), true)
  assert.equal(audio.playCalls, 2)
  assert.equal(audio.muted, false)
  assert.equal(audio.currentTime, 0)
})

test('reuses the same downloaded audio element', () => {
  FakeAudio.instances = []
  const tone = createChatCompletionTone({ Audio: FakeAudio })

  assert.equal(tone.prime(), true)
  assert.equal(tone.play(), true)
  assert.equal(FakeAudio.instances.length, 1)
})

test('does nothing when HTML audio is unavailable', () => {
  const tone = createChatCompletionTone({})
  assert.equal(tone.prime(), false)
  assert.equal(tone.play(), false)
})
