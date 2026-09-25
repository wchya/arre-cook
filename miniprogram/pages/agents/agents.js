const api = require("../../utils/api")
const SESSION_KEY = "ninimenu_session"
const PRESETS = [
  { value: "readonly", label: "只读", desc: "分析菜谱与饮食记录" },
  { value: "advisor", label: "顾问", desc: "读取数据并向你推送建议" },
  { value: "full", label: "完全代理", desc: "可按授权记录和调整菜单" },
]
const EXPIRY = [0, 30, 90, 365]

Page({
  data: {
    tokens: [], audit: [], summary: null, scopes: {},
    presets: PRESETS, presetIndex: 1, expires: EXPIRY, expiryIndex: 2,
    name: "", showCreate: false, createdToken: "", loading: true,
    saving: false, error: "",
  },

  onShow() {
    if (!wx.getStorageSync(SESSION_KEY)) {
      wx.reLaunch({ url: "/pages/login/login" })
      return
    }
    this.load()
  },

  async load() {
    if (this._loading) return
    this._loading = true
    this.setData({ loading: true, error: "" })
    try {
      const [data, audit] = await Promise.all([
        api.get("/me/agent-tokens"),
        api.get("/me/agent-audit", { pageSize: 12 }),
      ])
      const labels = data.scopes || {}
      const tokens = (data.tokens || []).map((token) => ({
        ...token,
        scopeText: (token.scopes || []).map((scope) => labels[scope] || scope).join("、") || "未授权范围",
      }))
      const summary = data.summary || {}
      summary.error_total = (summary.error_calls || 0) + (summary.denied_calls || 0)
      this.setData({ tokens, audit: audit.items || [], summary, scopes: labels })
    } catch (error) {
      this.setData({ error: error.message || "Agent 数据暂时无法加载" })
    } finally {
      this._loading = false
      this.setData({ loading: false })
    }
  },

  openCreate() { this.setData({ showCreate: true, name: "", error: "" }) },
  closeCreate() { if (!this.data.saving) this.setData({ showCreate: false }) },
  onNameInput(event) { this.setData({ name: event.detail.value }) },
  onPresetChange(event) { this.setData({ presetIndex: Number(event.detail.value) }) },
  onExpiryChange(event) { this.setData({ expiryIndex: Number(event.detail.value) }) },

  async createToken() {
    const name = this.data.name.trim()
    if (!name) { this.setData({ error: "请填写 Agent 名称" }); return }
    this.setData({ saving: true, error: "" })
    try {
      const preset = PRESETS[this.data.presetIndex]
      const result = await api.post("/me/agent-tokens", {
        name,
        scopes: [preset.value],
        expires_in_days: EXPIRY[this.data.expiryIndex],
      })
      this.setData({ showCreate: false, createdToken: result.token || "" })
      wx.showToast({ title: "令牌已创建", icon: "success" })
      await this.load()
    } catch (error) {
      this.setData({ error: error.message || "创建失败" })
    } finally {
      this.setData({ saving: false })
    }
  },

  copyToken() {
    if (!this.data.createdToken) return
    wx.setClipboardData({ data: this.data.createdToken, success: () => wx.showToast({ title: "已复制令牌", icon: "success" }) })
  },

  dismissToken() { this.setData({ createdToken: "" }) },

  revokeToken(event) {
    const token = this.data.tokens.find((item) => item.id === Number(event.currentTarget.dataset.id))
    if (!token) return
    wx.showModal({
      title: "撤销这个令牌？",
      content: `${token.name} 将立即失去食谱和饮食数据访问权限。`,
      confirmColor: "#a74431",
      success: async (result) => {
        if (!result.confirm) return
        try {
          await api.delete(`/me/agent-tokens/${token.id}`)
          await this.load()
          wx.showToast({ title: "令牌已撤销", icon: "success" })
        } catch (error) {
          wx.showToast({ title: error.message || "撤销失败", icon: "none" })
        }
      },
    })
  },

  openChat() { wx.navigateTo({ url: "/pages/chat/chat" }) },
})
