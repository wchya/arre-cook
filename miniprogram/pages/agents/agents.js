const api = require("../../utils/api")
const ui = require("../../utils/ui")
const fmt = require("../../utils/format")
const media = require("../../utils/media")
const session = require("../../utils/session")

const PRESETS = [
  { key: "readonly", title: "只读", desc: "读取口味画像、菜谱、记录，适合做分析报告" },
  { key: "advisor", title: "顾问 · 推荐", desc: "只读 + 推送建议 + 记录反馈，由你决定是否采纳" },
  { key: "full", title: "完全代理", desc: "还可替你记一餐、收藏、改偏好、重排菜单" },
]
const EXPIRES = [
  { days: 0, label: "永久" },
  { days: 30, label: "30 天" },
  { days: 90, label: "90 天" },
  { days: 365, label: "1 年" },
]
const NAME_SUGGESTIONS = ["DeepSeek", "Hermes", "Claude", "Cursor", "Dify", "Coze"]

function fmtTime(value) {
  const d = fmt.parseDate(value)
  if (!d) return "从未"
  const diff = (Date.now() - d.getTime()) / 1000
  if (diff < 60) return "刚刚"
  if (diff < 3600) return `${Math.floor(diff / 60)} 分钟前`
  if (diff < 86400) return `${Math.floor(diff / 3600)} 小时前`
  return `${d.getMonth() + 1}月${d.getDate()}日`
}

function expiryText(value) {
  const d = fmt.parseDate(value)
  if (!d) return "永久有效"
  return `${d.getFullYear()}/${d.getMonth() + 1}/${d.getDate()} 过期`
}

Page({
  data: {
    loading: true,
    active: [], inactiveCount: 0,
    summary: null,
    audit: [],
    mcpUrl: "", apiUrl: "",
    presets: PRESETS, expires: EXPIRES, nameSuggestions: NAME_SUGGESTIONS,
    createOpen: false, name: "", presetKey: "advisor", expiresDays: 0, saving: false,
    createdOpen: false, createdToken: "",
    renameOpen: false, renameId: 0, renameName: "", renaming: false,
    snippets: [], snippetIndex: 0, snippet: null, lastToken: "",
  },

  onLoad() {
    const origin = media.origin()
    this.setData({ mcpUrl: `${origin}/mcp`, apiUrl: `${origin}/api/agent` }, () => this.setSnippets(""))
  },

  onShow() {
    if (!session.requireLogin("/pages/agents/agents")) return
    this.load()
  },

  async onPullDownRefresh() {
    await this.load()
    wx.stopPullDownRefresh()
  },

  async load() {
    if (this._loading) return
    this._loading = true
    this.setData({ loading: !this.data.active.length })
    try {
      const [data, audit] = await Promise.all([
        api.get("/me/agent-tokens"),
        api.get("/me/agent-audit", { pageSize: 20 }),
      ])
      const labels = data.scopes || {}
      const tokens = (data.tokens || []).map((t) => ({
        id: t.id,
        name: t.name,
        prefix: t.prefix || "",
        initial: String(t.name || "?").slice(0, 1).toUpperCase(),
        active: t.active !== false,
        scopeChips: (t.scopes || []).map((s) => labels[s] || s),
        lastUsed: fmtTime(t.last_used_at),
        expiry: expiryText(t.expires_at),
      }))
      const patch = {
        active: tokens.filter((t) => t.active),
        inactiveCount: tokens.filter((t) => !t.active).length,
        summary: this.mapSummary(data.summary),
        audit: (audit.items || []).map((a) => ({
          id: a.id, actor: a.actor, tool: a.tool, error: a.error || "",
          channel: a.channel || "", time: fmtTime(a.created_at),
          dot: a.status === "ok" ? "is-ok" : a.status === "denied" ? "is-warn" : "is-bad",
        })),
      }
      if (data.mcp_url) patch.mcpUrl = data.mcp_url
      if (data.api_url) patch.apiUrl = data.api_url
      this.setData(patch, () => { if (data.mcp_url || data.api_url) this.setSnippets(this.data.lastToken) })
    } catch (error) {
      ui.toast(error.message || "AI 连接数据加载失败")
    } finally {
      this._loading = false
      this.setData({ loading: false })
    }
  },

  mapSummary(s) {
    if (!s) return null
    const calls = s.calls || 0
    return {
      activeTokens: s.active_tokens || 0,
      calls24: s.calls_last_24_hours || 0,
      calls,
      successRate: calls ? `${Math.round((s.success_calls || 0) * 100 / calls)}%` : "暂无",
    }
  },

  buildSnippets(token) {
    const mcp = this.data.mcpUrl
    const apiUrl = this.data.apiUrl
    const tk = token || this.data.lastToken || "nm_你的令牌"
    const mcpJson = JSON.stringify({ mcpServers: { ninimenu: { type: "http", url: mcp, headers: { Authorization: `Bearer ${tk}` } } } }, null, 2)
    return [
      { key: "mcp", label: "MCP 通用", note: "Claude Desktop / Cursor / Cherry Studio 等支持远程 MCP 的客户端", code: mcpJson },
      { key: "hermes", label: "Hermes", note: "写入 ~/.hermes/config.yaml 的 mcp_servers（以所用版本文档为准）", code: `mcp_servers:\n  ninimenu:\n    url: "${mcp}"\n    headers:\n      Authorization: "Bearer ${tk}"` },
      { key: "openapi", label: "OpenAPI", note: "Dify / Coze / GPTs Actions 导入此地址，鉴权选 Bearer 填入令牌", code: `${apiUrl}/openapi.json` },
      { key: "curl", label: "curl", note: "直接调用工具，请求体即工具参数", code: `curl -s -X POST "${apiUrl}/tools/recommend_dishes" \\\n  -H "Authorization: Bearer ${tk}" \\\n  -H "Content-Type: application/json" \\\n  -d '{"meal_type":"dinner","mood":"spicy","count":3}'` },
    ]
  },

  setSnippets(token) {
    const snippets = this.buildSnippets(token)
    const idx = Math.min(this.data.snippetIndex, snippets.length - 1)
    this.setData({ snippets, snippetIndex: idx, snippet: snippets[idx], lastToken: token || this.data.lastToken })
  },

  pickSnippet(event) {
    const index = Number(event.currentTarget.dataset.index)
    this.setData({ snippetIndex: index, snippet: this.data.snippets[index] })
  },

  copy(event) {
    const text = event.currentTarget.dataset.text
    if (!text) return
    wx.setClipboardData({ data: String(text), success: () => ui.toast("已复制", "success") })
  },

  // ---------- 新建令牌 ----------
  openCreate() { ui.haptic(); this.setData({ createOpen: true, name: "", presetKey: "advisor", expiresDays: 0 }) },
  closeCreate() { if (!this.data.saving) this.setData({ createOpen: false }) },
  onNameInput(event) { this.setData({ name: event.detail.value }) },
  pickName(event) { this.setData({ name: event.currentTarget.dataset.name }) },
  pickPreset(event) { ui.haptic(); this.setData({ presetKey: event.currentTarget.dataset.key }) },
  pickExpiry(event) { this.setData({ expiresDays: Number(event.currentTarget.dataset.days) }) },

  async createToken() {
    const name = this.data.name.trim()
    if (!name) { ui.toast("请给它起个名字"); return }
    if (this.data.saving) return
    this.setData({ saving: true })
    try {
      const res = await api.post("/me/agent-tokens", { name, scopes: [this.data.presetKey], expires_in_days: this.data.expiresDays })
      this.setData({ createOpen: false, saving: false, createdOpen: true, createdToken: res.token || "" })
      this.setSnippets(res.token || "")
      this.load()
    } catch (error) {
      this.setData({ saving: false })
      ui.toast(error.message || "创建失败")
    }
  },

  dismissCreated() { this.setData({ createdOpen: false }) },

  async rotate(event) {
    const id = Number(event.currentTarget.dataset.id)
    const name = event.currentTarget.dataset.name
    const ok = await ui.confirm({ title: `轮换「${name}」？`, content: "旧令牌会立即失效，需要在客户端替换成新令牌。", confirmText: "轮换" })
    if (!ok) return
    try {
      const res = await api.post(`/me/agent-tokens/${id}/rotate`)
      this.setData({ createdOpen: true, createdToken: res.token || "" })
      this.setSnippets(res.token || "")
      ui.toast("已轮换", "success")
      this.load()
    } catch (error) {
      ui.toast(error.message || "轮换失败")
    }
  },

  async revoke(event) {
    const id = Number(event.currentTarget.dataset.id)
    const name = event.currentTarget.dataset.name
    const ok = await ui.confirm({ title: `撤销「${name}」？`, content: "撤销后它将立即无法访问你的数据。", confirmText: "撤销", danger: true })
    if (!ok) return
    try {
      await api.delete(`/me/agent-tokens/${id}`)
      ui.toast("已撤销", "success")
      this.load()
    } catch (error) {
      ui.toast(error.message || "撤销失败")
    }
  },

  openRename(event) {
    this.setData({ renameOpen: true, renameId: Number(event.currentTarget.dataset.id), renameName: event.currentTarget.dataset.name || "" })
  },
  closeRename() { if (!this.data.renaming) this.setData({ renameOpen: false }) },
  onRenameInput(event) { this.setData({ renameName: event.detail.value }) },

  async saveRename() {
    const name = this.data.renameName.trim()
    if (!name || this.data.renaming) return
    this.setData({ renaming: true })
    try {
      await api.patch(`/me/agent-tokens/${this.data.renameId}`, { name })
      this.setData({ renameOpen: false, renaming: false })
      ui.toast("已更新", "success")
      this.load()
    } catch (error) {
      this.setData({ renaming: false })
      ui.toast(error.message || "保存失败")
    }
  },
})
