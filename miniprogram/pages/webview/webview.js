const app = getApp()
const SESSION_KEY = "ninimenu_session"

function normalizedSharePath(raw) {
  if (typeof raw !== "string" || !raw.startsWith("/") || raw.startsWith("//") || raw.includes("#")) return "/"
  return raw
}

Page({
  data: { src: "" },

  onLoad(options) {
    const token = app.globalData.pendingToken || wx.getStorageSync(SESSION_KEY)
    app.globalData.pendingToken = ""
    if (!token) {
      wx.reLaunch({ url: "/pages/login/login?reason=expired" })
      return
    }
    const sharePath = normalizedSharePath(options.path || "/")
    const target = new URL(sharePath, app.globalData.webBase)
    if (target.origin !== new URL(app.globalData.webBase).origin) {
      target.pathname = "/"
      target.search = ""
    }
    target.searchParams.set("from", "mp")
    target.hash = `token=${encodeURIComponent(token)}`
    this.setData({ src: target.toString() })
  },

  onMessage(event) {
    const messages = event.detail && Array.isArray(event.detail.data) ? event.detail.data : []
    for (const message of messages) {
      if (!message || typeof message !== "object") continue
      if (message.type === "share") {
        app.globalData.shareTitle = typeof message.title === "string" ? message.title : "NiniMenu · 今天吃什么"
        app.globalData.sharePath = normalizedSharePath(message.path)
      }
      if (message.type === "logout") {
        try { wx.removeStorageSync(SESSION_KEY) } catch (_) { /* ignore */ }
      }
    }
  },

  onShareAppMessage() {
    const path = normalizedSharePath(app.globalData.sharePath)
    return {
      title: app.globalData.shareTitle || "NiniMenu · 今天吃什么",
      path: `/pages/webview/webview?path=${encodeURIComponent(path)}`,
    }
  },
})
