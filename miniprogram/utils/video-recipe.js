const api = require("./api")
const session = require("./session")

// Preserve split UTF-8 characters on base libraries without TextDecoder.
function decodeUTF8Chunk(input, tail) {
  const bytes = new Uint8Array(input || new ArrayBuffer(0))
  const combined = new Uint8Array(tail.length + bytes.length)
  combined.set(tail); combined.set(bytes, tail.length)
  let index = 0, text = ""
  while (index < combined.length) {
    const first = combined[index]
    const size = first < 0x80 ? 1 : first >= 0xc2 && first <= 0xdf ? 2 : first >= 0xe0 && first <= 0xef ? 3 : first >= 0xf0 && first <= 0xf4 ? 4 : 0
    if (!size) throw new Error("提炼结果编码无效，请重试")
    if (index + size > combined.length) break
    let code = first & (size === 1 ? 0x7f : size === 2 ? 0x1f : size === 3 ? 0x0f : 0x07)
    for (let offset = 1; offset < size; offset++) {
      const next = combined[index + offset]
      if ((next & 0xc0) !== 0x80) throw new Error("提炼结果编码无效，请重试")
      code = (code << 6) | (next & 0x3f)
    }
    if (size === 2 && code < 0x80 || size === 3 && code < 0x800 || size === 4 && code < 0x10000 || code > 0x10ffff || code >= 0xd800 && code <= 0xdfff) throw new Error("提炼结果编码无效，请重试")
    if (code <= 0xffff) text += String.fromCharCode(code)
    else { code -= 0x10000; text += String.fromCharCode(0xd800 + (code >> 10), 0xdc00 + (code & 0x3ff)) }
    index += size
  }
  return { text, tail: combined.slice(index) }
}

function validResult(value) {
  const r = value && value.recipe, source = value && value.source
  return Boolean(r && typeof r.name === "string" && typeof r.remark === "string" && Number.isInteger(r.cook_time) && r.cook_time >= 0 && r.cook_time <= 600 &&
    [r.ingredients, r.seasonings].every((items) => Array.isArray(items) && items.length <= 25 && items.every((item) => item && typeof item.name === "string" && typeof item.amount === "string" && typeof item.evidence === "string")) &&
    Array.isArray(r.steps) && r.steps.length > 0 && r.steps.length <= 30 && r.steps.every((step) => step && typeof step.text === "string" && Number.isInteger(step.time) && typeof step.evidence === "string") &&
    source && ["subtitle", "audio", "manual"].indexOf(source.method) >= 0 && typeof source.url === "string" && typeof source.text === "string")
}

function extract(data, onProgress) {
  const token = api.token()
  let task, settled = false, gotChunk = false, result, done = false, buffer = "", raw = "", total = 0
  let tail = new Uint8Array(0), rejectPromise
  function receive(input) {
    if (settled) return
    if (token !== api.token()) throw new Error("登录状态已变化，请重新提炼")
    const chunk = typeof input === "string" ? { text: input, tail: new Uint8Array(0) } : decodeUTF8Chunk(input, tail)
    tail = chunk.tail; total += typeof input === "string" ? input.length * 3 : input.byteLength
    if (total > 128 * 1024) throw new Error("提炼结果过长，请缩短字幕后重试")
    raw += chunk.text; buffer += chunk.text
    let match
    while ((match = buffer.match(/\r?\n\r?\n/))) {
      const frame = buffer.slice(0, match.index)
      buffer = buffer.slice(match.index + match[0].length)
      let event = ""
      const lines = []
      frame.split(/\r?\n/).forEach((line) => {
        if (line.indexOf("event:") === 0) event = line.slice(6).trim()
        if (line.indexOf("data:") === 0) lines.push(line.slice(5).trimStart())
      })
      if (!lines.length) continue
      const value = JSON.parse(lines.join("\n"))
      if (event === "error") throw new Error(value.message || "提炼未完成，请稍后重试")
      if (event === "status" || event === "quota") onProgress({ event, data: value })
      if (event === "recipe") { if (result || done || !validResult(value)) throw new Error("提炼结果格式不完整，请重试"); result = value }
      if (event === "done") done = true
    }
  }
  const promise = new Promise((resolve, reject) => {
    rejectPromise = reject
    const fail = (error) => { if (settled) return; settled = true; reject(error) }
    task = wx.request({
      url: `${getApp().globalData.apiBase}/assistant/video-recipe`, method: "POST", data,
      timeout: 115000, enableChunked: true, responseType: "arraybuffer",
      header: { "content-type": "application/json", Accept: "text/event-stream", Authorization: `Bearer ${token}` },
      success(response) {
        if (settled) return
        if (token !== api.token()) { fail(new Error("登录状态已变化，请重新提炼")); return }
        if (response.statusCode === 401) { session.logout("expired"); fail(new Error("登录已过期")); return }
        if (response.statusCode < 200 || response.statusCode >= 300) {
          let error = new Error("提炼服务暂时不可用，请稍后重试")
          try {
            const value = JSON.parse(raw || (typeof response.data === "string" ? response.data : decodeUTF8Chunk(response.data, new Uint8Array(0)).text))
            error = new Error(value.message || error.message)
            if (value.data && value.data.quota) onProgress({ event: "quota", data: value.data.quota })
          } catch (_) { /* Safe generic message for non-JSON errors. */ }
          fail(error); return
        }
        try {
          if (!gotChunk && response.data) receive(response.data)
          if (!done || !result || buffer.trim() || tail.length) throw new Error("连接中断，草稿未被改动，请稍后重试")
          settled = true; resolve(result)
        } catch (error) { fail(error) }
      },
      fail(error) { const aborted = /abort/i.test((error && error.errMsg) || ""); const value = new Error(aborted ? "已取消" : "网络连接失败，请稍后重试"); if (aborted) value.name = "AbortError"; fail(value) },
    })
    if (task && typeof task.onChunkReceived === "function") task.onChunkReceived((event) => {
      if (settled) return
      try { gotChunk = true; receive(event.data) } catch (error) { fail(error); task.abort() }
    })
  })
  return {
    promise,
    abort() {
      if (settled) return
      settled = true
      const error = new Error("已取消"); error.name = "AbortError"; rejectPromise(error)
      if (task) task.abort()
    },
  }
}
module.exports = { extract, decodeUTF8Chunk }
