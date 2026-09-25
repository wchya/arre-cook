const SESSION_KEY = "ninimenu_session"

function decodeUTF8Chunk(input, tail) {
  const bytes = new Uint8Array(input || new ArrayBuffer(0))
  const combined = new Uint8Array(tail.length + bytes.length)
  combined.set(tail, 0)
  combined.set(bytes, tail.length)
  let index = 0
  let output = ""
  while (index < combined.length) {
    const first = combined[index]
    let size = 0
    let code = 0
    if (first < 0x80) { size = 1; code = first }
    else if ((first & 0xe0) === 0xc0) { size = 2; code = first & 0x1f }
    else if ((first & 0xf0) === 0xe0) { size = 3; code = first & 0x0f }
    else if ((first & 0xf8) === 0xf0) { size = 4; code = first & 0x07 }
    else { size = 1; code = 0xfffd }
    if (index + size > combined.length) break
    let valid = size === 1 || code !== 0xfffd
    for (let offset = 1; offset < size; offset++) {
      const next = combined[index + offset]
      if ((next & 0xc0) !== 0x80) { valid = false; break }
      code = (code << 6) | (next & 0x3f)
    }
    if (!valid) { output += "�"; index++; continue }
    if (code <= 0xffff) output += String.fromCharCode(code)
    else {
      code -= 0x10000
      output += String.fromCharCode(0xd800 + (code >> 10), 0xdc00 + (code & 0x3ff))
    }
    index += size
  }
  return { text: output, tail: combined.slice(index) }
}

Page({
  data: { messages: [], input: "", canSend: false, loading: false, error: "", sessionId: 0, suggestions: [] },

  onShow() {
    if (!wx.getStorageSync(SESSION_KEY)) {
      wx.reLaunch({ url: "/pages/login/login" })
      return
    }
    if (!this._loaded) this.loadHistory()
  },

  async loadHistory() {
    this._loaded = true
    try {
      const [sessions, status] = await Promise.all([
        this.apiGet("/assistant/sessions"),
        this.apiGet("/assistant/status"),
      ])
      const session = (sessions || [])[0]
      if (session) {
        const detail = await this.apiGet(`/assistant/sessions/${session.id}`)
        this.setData({
          sessionId: session.id,
          messages: (detail.messages || []).map((message) => ({ role: message.role, content: message.content, steps: [] })),
        })
      }
      this.setData({ suggestions: status.suggestions || [] })
    } catch (error) {
      this.setData({ error: error.message || "对话记录暂时无法加载" })
    }
  },

  apiGet(path) {
    const app = getApp()
    return new Promise((resolve, reject) => wx.request({
      url: `${app.globalData.apiBase}${path}`,
      header: { Authorization: `Bearer ${wx.getStorageSync(SESSION_KEY)}` },
      success: (response) => response.statusCode === 200 && response.data?.code === 0 ? resolve(response.data.data) : reject(new Error(response.data?.message || "读取失败")),
      fail: () => reject(new Error("网络连接失败")),
    }))
  },

  onInput(event) {
    const input = event.detail.value
    this.setData({ input, canSend: Boolean(input.trim()) })
  },
  useSuggestion(event) { this.setData({ input: event.currentTarget.dataset.text || "" }) },

  send() {
    const message = this.data.input.trim()
    if (!message || this.data.loading) return
    const messages = this.data.messages.concat([{ role: "user", content: message, steps: [] }, { role: "assistant", content: "", steps: [] }])
    this.setData({ messages, input: "", canSend: false, loading: true, error: "" })
    this._buffer = ""
    this._utf8Tail = new Uint8Array(0)
    this._sessionId = this.data.sessionId
    this._assistantIndex = messages.length - 1
    const app = getApp()
    this._task = wx.request({
      url: `${app.globalData.apiBase}/assistant/chat`,
      method: "POST",
      data: { session_id: this._sessionId || undefined, message },
      timeout: 120000,
      enableChunked: true,
      responseType: "arraybuffer",
      header: {
        "content-type": "application/json",
        Accept: "text/event-stream",
        Authorization: `Bearer ${wx.getStorageSync(SESSION_KEY)}`,
      },
      onChunkReceived: (event) => this.receiveChunk(event.data),
      success: (response) => {
        if (response.statusCode === 401) {
          this.setData({ error: "登录已过期，请重新登录" })
          wx.removeStorageSync(SESSION_KEY)
          wx.reLaunch({ url: "/pages/login/login?reason=expired" })
        } else if (response.statusCode < 200 || response.statusCode >= 300) {
          this.setData({ error: "助手暂时不可用，请稍后再试" })
        }
      },
      fail: () => this.setData({ error: "网络连接失败，请重试" }),
      complete: () => this.setData({ loading: false }),
    })
  },

  receiveChunk(buffer) {
    const decoded = decodeUTF8Chunk(buffer, this._utf8Tail || new Uint8Array(0))
    this._utf8Tail = decoded.tail
    this._buffer += decoded.text
    let boundary
    while ((boundary = this._buffer.search(/\r?\n\r?\n/)) >= 0) {
      const frame = this._buffer.slice(0, boundary)
      const separator = this._buffer.slice(boundary).match(/^\r?\n\r?\n/)[0]
      this._buffer = this._buffer.slice(boundary + separator.length)
      this.handleFrame(frame)
    }
  },

  handleFrame(frame) {
    let eventName = "message"
    const dataLines = []
    frame.split(/\r?\n/).forEach((line) => {
      if (line.startsWith("event:")) eventName = line.slice(6).trim()
      else if (line.startsWith("data:")) dataLines.push(line.slice(5).trim())
    })
    if (!dataLines.length) return
    let data
    try { data = JSON.parse(dataLines.join("\n")) } catch (_) { return }
    if (eventName === "session") {
      this._sessionId = data.session_id || this._sessionId
      this.setData({ sessionId: this._sessionId })
      return
    }
    if (eventName === "delta") {
      const messages = this.data.messages.slice()
      messages[this._assistantIndex].content += data.text || ""
      this.setData({ messages })
      return
    }
    if (eventName === "tool_start") {
      const messages = this.data.messages.slice()
      const item = messages[this._assistantIndex]
      item.steps = (item.steps || []).concat([data.label || data.name || "正在整理数据"])
      this.setData({ messages })
      return
    }
    if (eventName === "error") this.setData({ error: data.message || "助手遇到问题" })
  },

  cancel() {
    if (this._task) this._task.abort()
  },
})
