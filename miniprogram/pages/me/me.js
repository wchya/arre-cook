const api = require("../../utils/api")
const SESSION_KEY = "ninimenu_session"

Page({
  data: { user: null, stats: null, initial: "我", loading: true, error: "" },

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
      const [user, stats] = await Promise.all([api.get("/me"), api.get("/stats")])
      this.setData({ user, stats, initial: (user.nickname || user.username || "我").slice(0, 1) })
    } catch (error) {
      this.setData({ error: error.message || "账号信息暂时无法加载" })
    } finally {
      this._loading = false
      this.setData({ loading: false })
    }
  },

  goAgents() { wx.switchTab({ url: "/pages/agents/agents" }) },
  goDiary() { wx.switchTab({ url: "/pages/diary/diary" }) },
  goChat() { wx.navigateTo({ url: "/pages/chat/chat" }) },

  logout() {
    wx.showModal({
      title: "退出当前账号？",
      content: "退出后需要重新登录才能查看饮食数据。",
      confirmColor: "#a74431",
      success: (result) => {
        if (!result.confirm) return
        try { wx.removeStorageSync(SESSION_KEY) } catch (_) { /* ignore */ }
        const app = getApp()
        app.globalData.sessionRestoreAttempted = true
        wx.reLaunch({ url: "/pages/login/login?reason=logout" })
      },
    })
  },
})
