export const isBTWCommand = (value) => /^\/btw(?:$|[ \t])/.test(String(value || '').trim())
