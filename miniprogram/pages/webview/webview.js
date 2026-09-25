const app = getApp()
const SESSION_KEY = "ninimenu_session"

function normalizedSharePath(raw) {
  if (typeof raw !== "string" || !raw.startsWith("/") || raw.startsWith("//") || raw.includes("#")) return "/"
  return raw
}

Page({
  data: { src: "", error: "" },

  onLoad(options) {
    const token = app.globalData.pendingToken || wx.getStorageSync(SESSION_KEY)
    app.globalData.pendingToken = ""
    if (!token) {
      wx.reLaunch({ url: "/pages/login/login?reason=expired" })
      return
    }
    try {
      const sharePath = normalizedSharePath(options.path || "/")
      const base = String(app.globalData.webBase || "").replace(/\/+$/, "")
      if (!/^https:\/\/[^/]+(?:\/[^/]*)?$/i.test(base)) throw new Error("网页地址配置无效")
      // Do not use browser-only URL/URLSearchParams here. Android WeChat
      // runtimes can lack those globals, which previously left src empty.
      const separator = sharePath.includes("?") ? "&" : "?"
      const src = `${base}${sharePath}${separator}from=mp#token=${encodeURIComponent(token)}`
      this.setData({ src, error: "" })
    } catch (_) {
      this.setData({ error: "页面地址暂时无法打开，请点击重试" })
    }
  },

  onWebViewLoad() {
    this.setData({ error: "" })
  },

  onWebViewError() {
    this.setData({ error: "页面地址暂时无法打开，请检查小程序业务域名配置" })
  },

  retry() {
    this.setData({ error: "" })
    this.onLoad({})
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
