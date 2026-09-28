const api = require("../../utils/api")
const session = require("../../utils/session")
const ui = require("../../utils/ui")
const media = require("../../utils/media")
const fmt = require("../../utils/format")

const FALLBACK_SUGGESTIONS = ["今晚吃什么？", "我最近的饮食报告", "推荐一道省事的晚餐"]

// 小程序没有 TextDecoder：分块收到的 UTF-8 字节可能在多字节字符中间被截断，需要保留尾部待下一块拼接。
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

function toCard(card) {
  if (!card) return null
  if (card.type === "action") return { type: "action", text: card.text || "" }
  return {
    type: "dishes",
    title: card.title || "",
    items: (card.items || []).filter((item) => item && item.dish).map((item) => ({
      id: item.dish.id,
      name: item.dish.name,
      cover: media.assetUrl(item.dish.image),
      desc: (item.reasons && item.reasons.length ? item.reasons.join(" · ") : [item.dish.category, item.dish.cook_time > 0 ? `${item.dish.cook_time} 分钟` : ""].filter(Boolean).join(" · ")),
    })),
  }
}

let seq = 0
function uid(prefix) {
  seq += 1
  return `${prefix}-${Date.now()}-${seq}`
}

require("../../utils/theme").page({
  data: {
    llmEnabled: true,
    statusText: "可以聊饮食、菜谱与记录",
    suggestions: FALLBACK_SUGGESTIONS,
    sessionId: 0,
    messages: [],
    draft: "",
    busy: false,
    progressText: "正在确认你的食谱需求…",
    loadingSession: false,
    historyOpen: false,
    sessions: [],
    keyboard: 0,
    navHeight: 64,
    anchor: "",
    inputFocus: false,
    quota: null,
    quotaText: "正在读取今日次数…",
    canSend: false,
    statusLoading: true,
    statusError: "",
    sessionsError: "",
    loadingSessions: false,
  },

  onLoad(query) {
    this._request = 0
    this.setData({ navHeight: getApp().navMetrics().navHeight })
    if (!session.requireLogin("/pages/chat/chat")) return
    this.loadSessions()
    if (query.q) {
      try { this.setData({ draft: decodeURIComponent(query.q).slice(0, 1000) }) } catch (_) { /* malformed optional link */ }
    }
  },

  onShow() {
    this._visible = true
    if (session.requireLogin("/pages/chat/chat")) this.loadStatus()
  },

  onHide() {
    this._visible = false
    clearTimeout(this._quotaTimer)
  },

  onUnload() {
    this._unloaded = true
    this._visible = false
    clearTimeout(this._quotaTimer)
    this.abort()
  },

  // ---------- 数据 ----------

  applyQuota(quota) {
    if (!quota) return
    const quotaText = quota.blocked_reason === "site_limit" ? "今日助手服务次数已用完或已暂停" : quota.limit === 0 ? "管理员已暂停 AI 助手请求" : quota.remaining === 0 ? `今日 ${quota.limit} 次已用完` : `今日还可提问 ${quota.remaining} 次 / 每日 ${quota.limit} 次`
    this.setData({ quota, quotaText, canSend: quota.remaining > 0 && !quota.blocked_reason, statusError: "" })
  },

  scheduleQuotaRefresh() {
    clearTimeout(this._quotaTimer)
    if (!this._visible || this._unloaded) return
    const resetAt = this.data.quota && Date.parse(this.data.quota.reset_at)
    const delay = resetAt ? Math.min(60000, Math.max(1000, resetAt - Date.now() + 100)) : 60000
    this._quotaTimer = setTimeout(() => this.loadStatus(), delay)
  },

  async loadStatus() {
    if (this._unloaded) return
    const request = this._statusRequest = (this._statusRequest || 0) + 1
    const token = api.token()
    this.setData({ statusLoading: true })
    try {
      const status = await api.get("/assistant/status")
      if (this._unloaded || request !== this._statusRequest || token !== api.token()) return
      this.setData({
        llmEnabled: Boolean(status.llm_enabled),
        statusText: status.llm_enabled ? "可以聊饮食、菜谱与记录" : "基础推荐与饮食报告",
        suggestions: status.suggestions && status.suggestions.length ? status.suggestions : FALLBACK_SUGGESTIONS,
      })
      this.applyQuota(status.quota)
    } catch (error) {
      if (!this._unloaded && request === this._statusRequest && token === api.token()) this.setData({ statusError: error.message || "助手次数读取失败", canSend: false })
    } finally {
      if (!this._unloaded && request === this._statusRequest) {
        this.setData({ statusLoading: false })
        this.scheduleQuotaRefresh()
      }
    }
  },

  async loadSessions() {
    this.setData({ loadingSessions: true, sessionsError: "" })
    try {
      const sessions = await api.get("/assistant/sessions")
      if (this._unloaded) return
      this.setData({ sessions: (sessions || []).map((item) => ({ id: item.id, title: item.title || "新对话", date: fmt.relativeDate(item.updated_at) })) })
    } catch (error) {
      if (!this._unloaded) this.setData({ sessionsError: error.message || "对话记录读取失败" })
    } finally {
      if (!this._unloaded) this.setData({ loadingSessions: false })
    }
  },

  async openSession(event) {
    const id = Number(event.currentTarget.dataset.id)
    this.abort()
    const request = ++this._request
    this.setData({ sessionId: id, messages: [], loadingSession: true, historyOpen: false })
    try {
      const result = await api.get(`/assistant/sessions/${id}`)
      if (request !== this._request) return
      const messages = (result.messages || []).map((message) => ({
        id: `m${message.id}`,
        role: message.role,
        content: message.content || "",
        cards: (message.cards || []).map(toCard).filter(Boolean),
        tools: [],
      }))
      this.setData({ messages }, () => this.scrollToBottom())
    } catch (error) {
      if (request === this._request) ui.toast(error.message || "对话读取失败")
    } finally {
      if (request === this._request) this.setData({ loadingSession: false })
    }
  },

  async deleteSession(event) {
    const id = Number(event.currentTarget.dataset.id)
    const ok = await ui.confirm({ title: "删除这段对话？", confirmText: "删除", danger: true })
    if (!ok) return
    try {
      await api.delete(`/assistant/sessions/${id}`)
      if (id === this.data.sessionId) this.newChat()
      this.loadSessions()
    } catch (error) {
      ui.toast(error.message || "删除失败")
    }
  },

  newChat() {
    this.abort()
    this.setData({ sessionId: 0, messages: [], draft: "", loadingSession: false, historyOpen: false })
  },

  openHistory() {
    this.loadSessions()
    this.setData({ historyOpen: true })
  },

  closeHistory() { this.setData({ historyOpen: false }) },

  // ---------- 输入 ----------

  onInput(event) { this.setData({ draft: event.detail.value }) },
  onFocus() { this.setData({ inputFocus: true }) },
  onBlur() { this.setData({ inputFocus: false }) },

  onKeyboard(event) {
    const height = event.detail.height || 0
    if (height !== this.data.keyboard) this.setData({ keyboard: height }, () => this.scrollToBottom())
  },

  useSuggestion(event) {
    this.send(event.currentTarget.dataset.text)
  },

  sendDraft() {
    this.send(this.data.draft)
  },

  scrollToBottom() {
    this.setData({ anchor: "" }, () => this.setData({ anchor: "chat-bottom" }))
  },

  // ---------- 发送（SSE 流式） ----------

  send(value) {
    const text = String(value || "").trim()
    if (!text || this.data.busy || this.data.loadingSession) return
    if (!this.data.canSend) {
      this.setData({ draft: text })
      ui.toast(this.data.statusError ? "请先重试加载助手次数" : this.data.quotaText)
      return
    }
    const assistantId = uid("a")
    const previousMessages = this.data.messages
    const messages = previousMessages.concat([
      { id: uid("u"), role: "user", content: text, cards: [], tools: [] },
      { id: assistantId, role: "assistant", content: "", cards: [], tools: [], streaming: true, error: "" },
    ])
    this._request = (this._request || 0) + 1
    const request = this._request
    this._assistantIndex = messages.length - 1
    this._buffer = ""
    this._tail = new Uint8Array(0)
    this._completed = false
    this._gotChunk = false
    this._pending = ""
    this.setData({ messages, draft: "", busy: true, progressText: "正在确认你的食谱需求…" }, () => this.scrollToBottom())
    ui.haptic()

    const app = getApp()
    const requestToken = api.token()
    this._task = wx.request({
      url: `${app.globalData.apiBase}/assistant/chat`,
      method: "POST",
      data: { session_id: this.data.sessionId || undefined, message: text },
      timeout: 120000,
      enableChunked: true,
      responseType: "arraybuffer",
      header: {
        "content-type": "application/json",
        Accept: "text/event-stream",
        Authorization: `Bearer ${requestToken}`,
      },
      success: (response) => {
        if (request !== this._request || requestToken !== api.token()) return
        if (response.statusCode === 401) {
          session.logout("expired")
          return
        }
        if (response.statusCode < 200 || response.statusCode >= 300) {
          let message = "助手暂时不可用，请稍后再试"
          try {
            const raw = typeof response.data === "string" ? response.data : decodeUTF8Chunk(response.data, new Uint8Array(0)).text
            const body = JSON.parse(raw || this._buffer)
            if (body && body.message) message = body.message
            if (body && body.data && body.data.quota) this.applyQuota(body.data.quota)
          } catch (_) { /* ignore */ }
          if (response.statusCode === 429) {
            this.setData({ messages: previousMessages, draft: this.data.draft || text })
            ui.toast(message)
          } else {
            this.patchAssistant({ error: message })
            this.setData({ draft: this.data.draft || text })
          }
        } else if (response.data && response.data.byteLength && !this._gotChunk) {
          // 不支持分块的基础库会在最后一次性返回全部内容
          this.receiveChunk(response.data)
        }
      },
      fail: (error) => {
        if (request !== this._request) return
        if (!/abort/i.test((error && error.errMsg) || "")) {
          this.patchAssistant({ error: "网络连接失败，请查看对话记录后重试" })
          this.setData({ draft: this.data.draft || text })
        }
      },
      complete: () => {
        if (request !== this._request) return
        this.flushDelta()
        const current = this.data.messages[this._assistantIndex]
        if (current && !this._completed && !current.error && !current.content) this.patchAssistant({ error: "连接中断，请查看对话记录后重试" })
        this.patchAssistant({ streaming: false })
        this.setData({ busy: false })
        this._task = null
        this.loadSessions()
        this.loadStatus()
      },
    })
    if (this._task && typeof this._task.onChunkReceived === "function") {
      this._task.onChunkReceived((event) => {
        if (request !== this._request) return
        this._gotChunk = true
        this.receiveChunk(event.data)
      })
    }
  },

  receiveChunk(buffer) {
    const decoded = decodeUTF8Chunk(buffer, this._tail || new Uint8Array(0))
    this._tail = decoded.tail
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
    const lines = []
    frame.split(/\r?\n/).forEach((line) => {
      if (line.indexOf("event:") === 0) eventName = line.slice(6).trim()
      else if (line.indexOf("data:") === 0) lines.push(line.slice(5).replace(/^\s/, ""))
    })
    if (!lines.length) return
    let data
    try { data = JSON.parse(lines.join("\n")) } catch (_) { return }
    if (eventName === "status") {
      this.setData({ progressText: data.message || "正在整理菜谱…" })
      return
    }
    if (eventName === "cards") {
      const message = this.data.messages[this._assistantIndex]
      if (message) this.patchAssistant({ cards: message.cards.concat((data.cards || []).map(toCard).filter(Boolean)) })
      return
    }
    if (eventName === "quota") {
      this._statusRequest = (this._statusRequest || 0) + 1
      this.setData({ statusLoading: false })
      this.applyQuota(data)
      this.scheduleQuotaRefresh()
      return
    }
    const index = this._assistantIndex
    const message = this.data.messages[index]
    if (!message) return

    if (eventName === "session") {
      this.setData({ sessionId: data.session_id || this.data.sessionId })
    } else if (eventName === "delta") {
      // 合并高频的增量文本，减少 setData 次数
      this._pending += data.text || ""
      if (!this._flushTimer) this._flushTimer = setTimeout(() => this.flushDelta(), 60)
    } else if (eventName === "tool_start") {
      this.flushDelta()
      const tools = message.tools.concat([{ id: data.id || uid("t"), label: data.label || data.name || "正在整理数据", status: "running", error: "" }])
      this.setData({ [`messages[${index}].tools`]: tools }, () => this.scrollToBottom())
    } else if (eventName === "tool_end") {
      this.flushDelta()
      const tools = message.tools.map((tool) => (tool.id === data.id ? { ...tool, status: data.ok ? "ok" : "error", error: data.error || "" } : tool))
      const card = toCard(data.card)
      const updates = { [`messages[${index}].tools`]: tools }
      if (card) updates[`messages[${index}].cards`] = message.cards.concat([card])
      this.setData(updates, () => this.scrollToBottom())
    } else if (eventName === "error") {
      this.flushDelta()
      this.patchAssistant({ error: data.message || "助手遇到问题" })
    } else if (eventName === "done") {
      this.flushDelta()
      this._completed = true
      this.patchAssistant({ streaming: false })
    }
  },

  flushDelta() {
    clearTimeout(this._flushTimer)
    this._flushTimer = null
    if (!this._pending) return
    const index = this._assistantIndex
    const message = this.data.messages[index]
    if (!message) return
    const content = message.content + this._pending
    this._pending = ""
    this.setData({ [`messages[${index}].content`]: content }, () => this.scrollToBottom())
  },

  patchAssistant(patch) {
    const index = this._assistantIndex
    if (index === undefined || !this.data.messages[index]) return
    const updates = {}
    Object.keys(patch).forEach((key) => { updates[`messages[${index}].${key}`] = patch[key] })
    this.setData(updates)
  },

  abort() {
    this._request = (this._request || 0) + 1
    clearTimeout(this._flushTimer)
    this._flushTimer = null
    if (this._task) {
      try { this._task.abort() } catch (_) { /* ignore */ }
      this._task = null
    }
    if (this.data.busy) {
      this.patchAssistant({ streaming: false })
      this.setData({ busy: false })
    }
    if (!this._unloaded) this.loadStatus()
  },

  stop() {
    this.flushDelta()
    this.abort()
    ui.toast("已停止生成")
  },

  // ---------- 卡片 ----------

  openDish(event) {
    wx.navigateTo({ url: `/pages/dish/dish?id=${event.currentTarget.dataset.id}` })
  },

  copyMessage(event) {
    const text = event.currentTarget.dataset.text
    if (!text) return
    wx.setClipboardData({ data: text, success: () => ui.toast("已复制") })
  },

  goDiary() { wx.navigateTo({ url: "/pages/diary/diary" }) },
  goAssistant() { wx.navigateTo({ url: "/pages/assistant/assistant" }) },
})
