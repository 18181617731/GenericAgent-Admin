import React from 'react'

/**
 * Parse Anthropic Messages API content blocks into the same structure that
 * parseAssistantContent() produces from plain text.
 * 
 * Input: content blocks from message.structured_content
 * [
 *   {type: 'text', text: '...'},
 *   {type: 'tool_use', id: 'toolu_xxx', name: 'file_read', input: {...}},
 *   {type: 'tool_result', tool_use_id: 'toolu_xxx', content: [...] | "..."},
 *   {type: 'thinking', thinking: '...'}  // extended thinking
 * ]
 * 
 * Output: {runs: [], summary: '', body: '', tools: [], thinking: ''}
 * Compatible with existing AssistantContent renderer.
 */
export function parseStructuredContent(contentBlocks) {
  if (!Array.isArray(contentBlocks) || contentBlocks.length === 0) {
    return null
  }

  const tools = []
  let thinking = ''
  const textParts = []

  for (const block of contentBlocks) {
    if (!block || typeof block !== 'object') continue

    switch (block.type) {
      case 'text':
        if (block.text) textParts.push(block.text)
        break

      case 'thinking':
        if (block.thinking) thinking = block.thinking
        break

      case 'tool_use':
        tools.push({
          id: block.id || '',
          name: block.name || '',
          input: block.input || {},
        })
        break

      case 'tool_result':
        // Tool results are typically paired with tool_use blocks
        // We'll render them inline when processing the full message
        break

      default:
        // Unknown block types are ignored
        break
    }
  }

  // Join all text blocks
  const fullText = textParts.join('\n\n').trim()

  // Check for turn markers in the text (for backward compatibility with GA's output)
  // If text contains turn markers, fall back to text parsing
  if (fullText.includes('**LLM Running (Turn ') || fullText.includes('[Info] Final response')) {
    return null // Signal to fall back to parseAssistantContent
  }

  return {
    runs: [],
    summary: '',
    body: fullText,
    tools,
    thinking,
  }
}
