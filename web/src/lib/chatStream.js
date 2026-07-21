export const isBTWCommand = (value) => /^\/btw(?:$|[ \t])/.test(String(value || '').trim())

export const shouldFinishStreamFollow = ({ running, replay, completed, eventCount }) => (
  !running && replay && completed && eventCount === 0
)

export const mergeFinalStreamMessage = (streamed = {}, finalMessage = {}) => {
  const merged = { ...finalMessage }
  if ((!merged.model_id || !String(merged.model_id).trim()) && streamed.model_id) merged.model_id = streamed.model_id
  if (merged.usage == null && streamed.usage != null) merged.usage = streamed.usage
  if ((!Array.isArray(merged.usages) || merged.usages.length === 0) && Array.isArray(streamed.usages) && streamed.usages.length) {
    merged.usages = streamed.usages
  }
  return merged
}
