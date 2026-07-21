export const isBTWCommand = (value) => /^\/btw(?:$|[ \t])/.test(String(value || '').trim())

export const shouldFinishStreamFollow = ({ running, replay, completed, eventCount }) => (
  !running && replay && completed && eventCount === 0
)
