const COMPLETION_TONE_SOURCE = '/audio/chat-complete.wav'
const COMPLETION_TONE_VOLUME = 0.45

const audioConstructor = audioWindow => audioWindow?.Audio

const resetAudio = audio => {
  try {
    audio.pause?.()
    audio.currentTime = 0
  } catch {
    // Audio cleanup is best effort and must not affect the chat flow.
  }
}

export const createChatCompletionTone = (
  audioWindow = typeof window !== 'undefined' ? window : null,
  source = COMPLETION_TONE_SOURCE,
) => {
  let audio = null

  const getAudio = () => {
    if (audio) return audio
    const Constructor = audioConstructor(audioWindow)
    if (typeof Constructor !== 'function') return null
    try {
      audio = new Constructor(source)
      audio.preload = 'auto'
      audio.volume = COMPLETION_TONE_VOLUME
      return audio
    } catch {
      return null
    }
  }

  const prime = () => {
    const player = getAudio()
    if (!player) return false
    try {
      player.load?.()
      const wasMuted = Boolean(player.muted)
      player.muted = true
      const result = player.play?.()
      const cleanup = () => {
        resetAudio(player)
        player.muted = wasMuted
      }
      if (result?.then) result.then(cleanup, cleanup)
      else cleanup()
      return true
    } catch {
      return false
    }
  }

  const play = () => {
    const player = getAudio()
    if (!player) return false
    try {
      player.muted = false
      player.currentTime = 0
      const result = player.play?.()
      result?.catch?.(() => {})
      return true
    } catch {
      return false
    }
  }

  return { prime, play }
}

const defaultCompletionTone = createChatCompletionTone()

export const primeChatCompletionTone = () => defaultCompletionTone.prime()
export const playChatCompletionTone = () => defaultCompletionTone.play()
