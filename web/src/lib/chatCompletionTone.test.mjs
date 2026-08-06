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
    this.readyState = 0
    this.playsInline = false
    this.attributes = {}
    this.loadCalls = 0
    this.playCalls = 0
    this.pauseCalls = 0
    FakeAudio.instances.push(this)
  }

  load() { this.loadCalls += 1 }
  setAttribute(name, value) { this.attributes[name] = value }
  play() { this.playCalls += 1; return Promise.resolve() }
  pause() { this.pauseCalls += 1 }
}

class RejectingAudio extends FakeAudio {
  play() {
    this.playCalls += 1
    return Promise.reject(new Error('NotAllowedError'))
  }
}

class FakeAudioContext {
  static instances = []

  constructor() {
    this.state = 'running'
    this.currentTime = 0
    this.destination = {}
    this.resumeCalls = 0
    this.oscillatorStarts = 0
    this.bufferSourceStarts = 0
    this.bufferSourceStops = 0
    this.bufferSources = []
    this.decodedBuffers = []
    this.oscillator = { frequency: { setValueAtTime() {}, exponentialRampToValueAtTime() {} }, connect() {}, start: () => { this.oscillatorStarts += 1 }, stop() {} }
    this.gain = { gain: { setValueAtTime() {}, exponentialRampToValueAtTime() {} }, connect() {} }
    FakeAudioContext.instances.push(this)
  }

  createOscillator() { return this.oscillator }
  createGain() { return this.gain }
  createBuffer() { return {} }
  createBufferSource() {
    const source = { buffer: null, connect: () => {}, start: () => { this.bufferSourceStarts += 1 }, stop: () => { this.bufferSourceStops += 1 } }
    this.bufferSources.push(source)
    return source
  }
  decodeAudioData(arrayBuffer, resolve, reject) {
    this.decodedBuffers.push(arrayBuffer)
    const buffer = { duration: 0.18 }
    if (typeof resolve === 'function') {
      resolve(buffer)
      return undefined
    }
    return Promise.resolve(buffer)
  }
  resume() { this.resumeCalls += 1; this.state = 'running'; return Promise.resolve() }
}

const createFetchWindow = (contextFactory = FakeAudioContext) => {
  const fetched = []
  const response = {
    ok: true,
    arrayBuffer: async () => new Uint8Array([1, 2, 3]).buffer,
  }
  return {
    Audio: FakeAudio,
    AudioContext: contextFactory,
    fetch: async source => {
      fetched.push(source)
      return response
    },
    fetched,
  }
}

test('primes and plays a downloaded completion notification sound', async () => {
  FakeAudio.instances = []
  const tone = createChatCompletionTone({ Audio: FakeAudio }, '/audio/test-notification.wav')

  assert.equal(tone.prime(), true)
  const audio = FakeAudio.instances[0]
  assert.equal(audio.src, '/audio/test-notification.wav')
  assert.equal(audio.preload, 'auto')
  assert.equal(audio.playsInline, true)
  assert.equal(audio.attributes.playsinline, '')
  assert.equal(audio.attributes['webkit-playsinline'], '')
  assert.equal(audio.loadCalls, 1)
  assert.equal(audio.playCalls, 1)
  assert.equal(audio.volume, 0.001)

  await Promise.resolve()
  await Promise.resolve()
  assert.equal(audio.pauseCalls, 1)
  assert.equal(audio.muted, false)
  assert.equal(audio.volume, 0.85)
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

test('falls back to Web Audio when HTML audio playback is rejected', async () => {
  FakeAudio.instances = []
  FakeAudioContext.instances = []
  const errors = []
  const tone = createChatCompletionTone({ Audio: RejectingAudio, AudioContext: FakeAudioContext })

  assert.equal(tone.play({ onError: error => errors.push(error) }), true)
  await Promise.resolve()
  await Promise.resolve()
  assert.equal(FakeAudio.instances[0].playCalls, 1)
  assert.equal(FakeAudioContext.instances.length, 1)
  assert.equal(errors.length, 0)
})

test('unlocks Web Audio during the user action before asynchronous completion', async () => {
  class SuspendedAudioContext extends FakeAudioContext {
    constructor() {
      super()
      this.state = 'suspended'
    }
  }

  FakeAudio.instances = []
  FakeAudioContext.instances = []
  const tone = createChatCompletionTone({ Audio: FakeAudio, AudioContext: SuspendedAudioContext })

  assert.equal(tone.prime(), true)
  await Promise.resolve()
  const context = FakeAudioContext.instances[0]
  assert.equal(context.resumeCalls, 1)
  assert.equal(context.state, 'running')
  assert.equal(tone.play(), true)
  assert.equal(context.oscillatorStarts, 0)
})

test('uses the downloaded sound for asynchronous completion after priming', async () => {
  FakeAudio.instances = []
  FakeAudioContext.instances = []
  const tone = createChatCompletionTone({ Audio: FakeAudio, AudioContext: FakeAudioContext })

  assert.equal(tone.prime(), true)
  await Promise.resolve()
  const audio = FakeAudio.instances[0]
  assert.equal(tone.play(), true)
  assert.equal(audio.playCalls, 2)
  assert.equal(FakeAudioContext.instances[0].oscillatorStarts, 0)
})

test('prefers the decoded completion buffer after priming', async () => {
  FakeAudio.instances = []
  FakeAudioContext.instances = []
  const window = createFetchWindow()
  const tone = createChatCompletionTone(window, '/audio/custom-tone.wav')

  assert.equal(tone.prime(), true)
  await new Promise(resolve => setTimeout(resolve, 0))
  const context = FakeAudioContext.instances[0]
  const audio = FakeAudio.instances[0]

  assert.equal(window.fetched[0], '/audio/custom-tone.wav')
  assert.equal(context.decodedBuffers.length, 1)
  assert.equal(tone.play(), true)
  assert.equal(context.bufferSources.some(source => source.buffer?.duration === 0.18), true)
  assert.equal(context.bufferSourceStarts, 2)
  assert.equal(context.bufferSourceStops, 2)
  assert.equal(audio.playCalls, 1)
})
