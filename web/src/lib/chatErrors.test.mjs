import assert from 'node:assert/strict'
import { chatErrorPresentation } from './chatErrors.js'

const provider = chatErrorPresentation({
  error:true,
  content:'!!!Error: HTTP 403: <!DOCTYPE html><html><head><title>Attention Required! | Cloudflare</title></head></html>',
})
assert.equal(provider.sourceLabel, '模型服务')
assert.equal(provider.code, 'HTTP_403')
assert.match(provider.summary, /403/)
assert.doesNotMatch(provider.detail, /<html/i)
assert.match(provider.detail, /Cloudflare/)

const project = chatErrorPresentation({ error:true, content:"Traceback: ModuleNotFoundError: No module named 'requests'" })
assert.equal(project.sourceLabel, '项目运行环境')
assert.equal(project.code, 'PROJECT_RUNTIME_ERROR')

const structured = chatErrorPresentation({
  error:true,
  content:'safe summary',
  error_info:{ source:'network', source_label:'网络连接', code:'NETWORK_ERROR', summary:'连接失败', hint:'检查网络', detail:'timeout', retryable:true },
})
assert.equal(structured.summary, '连接失败')
assert.equal(structured.retryable, true)

console.log('chatErrors tests passed')
