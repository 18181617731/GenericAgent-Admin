import test from 'node:test'
import assert from 'node:assert/strict'
import { extractGeneratedImagePaths, generatedImageDownloadURL, generatedImageURL } from './generatedImages.js'

test('extracts and deduplicates generated local image paths from assistant and tool output', () => {
  const content = String.raw`Generated: G:\MygenericAgent\temp\comfy output\final image.png
Artifact:
temp/preview.webp
{"path":"G:\MygenericAgent\temp\comfy output\final image.png"}`
  assert.deepEqual(extractGeneratedImagePaths(content), [
    String.raw`G:\MygenericAgent\temp\comfy output\final image.png`,
    'temp/preview.webp',
  ])
})

test('does not auto-load remote trackers, data URLs, or non-image files', () => {
  const content = 'https://tracker.example/pixel.png data:image/png;base64,abc G:\\MygenericAgent\\temp\\note.txt'
  assert.deepEqual(extractGeneratedImagePaths(content), [])
})

test('ignores image paths inside tool code, device output, and uploaded-file history', () => {
  const content = [
    'Tool: code_run',
    '```python',
    String.raw`src = "C:\\Users\\zk\\AppData\\Roaming\\GenericAgent-Admin\\chat_uploads\\dog.jpg"`,
    String.raw`remote = "/sdcard/ga_shot.png"`,
    String.raw`path = r"G:\\MygenericAgent\\temp\\run\\dog_new1.jpg"`,
    '```',
    String.raw`[Stdout] G:\\MygenericAgent\\temp\\run\\dog_new1.jpg: 1 file pushed`,
    String.raw`[image:C:\\Users\\zk\\AppData\\Roaming\\GenericAgent-Admin\\chat_uploads\\dog.jpg]`,
  ].join('\n')
  assert.deepEqual(extractGeneratedImagePaths(content), [])
})

test('keeps explicit image markup and labeled generated-image output', () => {
  const content = [
    String.raw`![preview](temp/preview.webp)`,
    String.raw`[FILE:G:\\MygenericAgent\\temp\\final.png]`,
    String.raw`Generated: G:\\MygenericAgent\\temp\\final-2.jpg`,
  ].join('\n')
  assert.deepEqual(extractGeneratedImagePaths(content), [
    'temp/preview.webp',
    String.raw`G:\\MygenericAgent\\temp\\final.png`,
    String.raw`G:\\MygenericAgent\\temp\\final-2.jpg`,
  ])
})

test('extracts screenshots from evidence labels across a blank line', () => {
  const path = String.raw`G:\MygenericAgent\temp\projects\evidence\turn75_final_read_screen.png`
  const content = [
    '\u8bc1\u636e\u5df2\u4fdd\u5b58\uff1a',
    '',
    `- \u622a\u56fe\uff1a ${path}`,
  ].join('\n')
  assert.deepEqual(extractGeneratedImagePaths(content), [path])
})

test('extracts screenshot result labels in English', () => {
  const path = String.raw`C:\tmp\evidence\final-screen.svg`
  assert.deepEqual(extractGeneratedImagePaths(`Screenshot: ${path}`), [path])
})

test('builds safe encoded preview and download endpoints', () => {
  const path = String.raw`G:\MygenericAgent\temp\成品 1.png`
  assert.equal(generatedImageURL(path), `/api/files/image?path=${encodeURIComponent(path)}`)
  assert.equal(generatedImageDownloadURL(path), `/api/files/download?path=${encodeURIComponent(path)}`)
})
