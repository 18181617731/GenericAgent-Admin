const COMPLETION_TONE_SOURCE = '/audio/chat-complete.wav?v=2'
const COMPLETION_TONE_VOLUME = 0.85
const FALLBACK_TONE_DURATION = 0.18

const audioConstructor = audioWindow => audioWindow?.Audio
const audioContextConstructor = audioWindow => audioWindow?.AudioContext || audioWindow?.webkitAudioContext

const resetAudio = audio => {
  try {
    audio.pause?.()
    audio.currentTime = 0
  } catch {
    // Audio cleanup is best effort and must not affect the chat flow.
  }
}

const mountAudio = (audioWindow, audio) => {
  const body = audioWindow?.document?.body
  if (!body || audio?.parentNode || typeof body.appendChild !== 'function') return
  try {
    audio.setAttribute?.('aria-hidden', 'true')
    audio.setAttribute?.('preload', 'auto')
    if (audio.style) Object.assign(audio.style, {
      position: 'fixed', left: '0', bottom: '0', width: '1px', height: '1px', opacity: '0.01', pointerEvents: 'none',
    })
    body.appendChild(audio)
  } catch {}
}

const getAudioContext = (audioWindow, contextRef) => {
  const Constructor = audioContextConstructor(audioWindow)
  if (typeof Constructor !== 'function') return null
  try {
    return contextRef.current || (contextRef.current = new Constructor())
  } catch {
    return null
  }
}

const decodeAudioBuffer = (context, arrayBuffer) => new Promise((resolve, reject) => {
  try {
    const result = context.decodeAudioData?.(arrayBuffer, resolve, reject)
    if (result?.then) result.then(resolve, reject)
    else if (!context.decodeAudioData) resolve(null)
  } catch (error) {
    reject(error)
  }
})

const loadCompletionBuffer = async (audioWindow, contextRef, stateRef, source) => {
  if (stateRef.buffer || stateRef.loading) return stateRef.loading || stateRef.buffer
  const context = getAudioContext(audioWindow, contextRef)
  if (!context || typeof audioWindow?.fetch !== 'function' || typeof context.decodeAudioData !== 'function') return null
  stateRef.loading = (async () => {
    try {
      const response = await audioWindow.fetch(source, { cache: 'force-cache' })
      if (!response) return null
      const arrayBuffer = await response.arrayBuffer()
      if (!arrayBuffer) return null
      const buffer = await decodeAudioBuffer(context, arrayBuffer)
      if (buffer) stateRef.buffer = buffer
      return buffer
    } catch {
      return null
    } finally {
      stateRef.loading = null
    }
  })()
  return stateRef.loading
}

const unlockAudioContext = (audioWindow, contextRef) => {
  const context = getAudioContext(audioWindow, contextRef)
  if (!context) return false
  try {
    // Start the silent buffer synchronously. On iOS, waiting for resume()'s
    // promise can lose the user-activation window needed to unlock audio.
    if (typeof context.createBuffer === 'function' && typeof context.createBufferSource === 'function') {
      const source = context.createBufferSource()
      source.buffer = context.createBuffer(1, 1, 22050)
      source.connect(context.destination)
      source.start(context.currentTime)
      source.stop(context.currentTime + 0.001)
    }
    const resumed = context.state === 'suspended' ? context.resume?.() : null
    resumed?.catch?.(() => {})
    return true
  } catch {
    return false
  }
}

const playFallbackTone = (audioWindow, contextRef) => {
  const context = getAudioContext(audioWindow, contextRef)
  if (!context) return false
  try {
    const start = () => {
      const oscillator = context.createOscillator()
      const gain = context.createGain()
      const startAt = context.currentTime
      gain.gain.setValueAtTime(COMPLETION_TONE_VOLUME * 0.18, startAt)
      gain.gain.exponentialRampToValueAtTime(0.001, startAt + FALLBACK_TONE_DURATION)
      oscillator.frequency.setValueAtTime(880, startAt)
      oscillator.frequency.exponentialRampToValueAtTime(660, startAt + FALLBACK_TONE_DURATION)
      oscillator.connect(gain)
      gain.connect(context.destination)
      oscillator.start(startAt)
      oscillator.stop(startAt + FALLBACK_TONE_DURATION)
    }
    start()
    const resumed = context.state === 'suspended' ? context.resume?.() : null
    resumed?.catch?.(() => {})
    return true
  } catch {
    return false
  }
}

const playDecodedBuffer = (audioWindow, contextRef, stateRef) => {
  const context = getAudioContext(audioWindow, contextRef)
  const buffer = stateRef.buffer
  if (!context || !buffer || typeof context.createBufferSource !== 'function' || typeof context.createGain !== 'function') return false
  try {
    const source = context.createBufferSource()
    const gain = context.createGain()
    const startAt = context.currentTime
    gain.gain.setValueAtTime(COMPLETION_TONE_VOLUME, startAt)
    source.buffer = buffer
    source.connect(gain)
    gain.connect(context.destination)
    source.start(startAt)
    if (Number.isFinite(buffer.duration) && buffer.duration > 0) {
      source.stop(startAt + buffer.duration)
    }
    const resumed = context.state === 'suspended' ? context.resume?.() : null
    resumed?.catch?.(() => {})
    return true
  } catch {
    return false
  }
}

export const createChatCompletionTone = (
  audioWindow = typeof window !== 'undefined' ? window : null,
  source = COMPLETION_TONE_SOURCE,
) => {
  let audio = null
  const contextRef = { current: null }
  const bufferRef = { buffer: null, loading: null }

  const getAudio = () => {
    if (audio) return audio
    const Constructor = audioConstructor(audioWindow)
    if (typeof Constructor !== 'function') return null
    try {
      audio = new Constructor(source)
      audio.preload = 'auto'
      audio.volume = COMPLETION_TONE_VOLUME
      audio.playsInline = true
      audio.setAttribute?.('playsinline', '')
      audio.setAttribute?.('webkit-playsinline', '')
      mountAudio(audioWindow, audio)
      return audio
    } catch {
      return null
    }
  }

  const prime = () => {
    const player = getAudio()
    const contextReady = unlockAudioContext(audioWindow, contextRef)
    void loadCompletionBuffer(audioWindow, contextRef, bufferRef, source)
    if (!player) return contextReady
    mountAudio(audioWindow, player)
    try {
      player.load?.()
      const wasMuted = Boolean(player.muted)
      const previousVolume = Number.isFinite(player.volume) ? player.volume : 1
      // Keep the element technically audible while making the warm-up
      // inaudible. iOS may not unlock a permanently muted media element for a
      // later asynchronous, audible play() call.
      player.muted = false
      player.volume = 0.001
      const result = player.play?.()
      const cleanup = () => {
        resetAudio(player)
        player.muted = wasMuted
        player.volume = previousVolume
      }
      if (result?.then) result.then(cleanup, cleanup)
      else cleanup()
      return true
    } catch {
      return contextReady
    }
  }

  const play = ({ onError } = {}) => {
    if (playDecodedBuffer(audioWindow, contextRef, bufferRef)) return true
    const player = getAudio()
    if (player) {
      try {
        mountAudio(audioWindow, player)
        if (player.readyState === 0) player.load?.()
        player.muted = false
        player.volume = COMPLETION_TONE_VOLUME
        player.currentTime = 0
        const result = player.play?.()
        result?.catch?.(error => {
          if (!playFallbackTone(audioWindow, contextRef)) onError?.(error)
        })
        return true
      } catch {
        // Fall through to Web Audio when HTML media is unavailable.
      }
    }
    const played = playFallbackTone(audioWindow, contextRef)
    if (!played) onError?.(new Error('Audio playback is unavailable'))
    return played
  }

  return { prime, play }
}

const defaultCompletionTone = createChatCompletionTone()

export const primeChatCompletionTone = () => defaultCompletionTone.prime()
export const playChatCompletionTone = options => defaultCompletionTone.play(options)
