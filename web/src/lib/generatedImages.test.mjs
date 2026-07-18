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

test('builds safe encoded preview and download endpoints', () => {
  const path = String.raw`G:\MygenericAgent\temp\成品 1.png`
  assert.equal(generatedImageURL(path), `/api/files/image?path=${encodeURIComponent(path)}`)
  assert.equal(generatedImageDownloadURL(path), `/api/files/download?path=${encodeURIComponent(path)}`)
})
