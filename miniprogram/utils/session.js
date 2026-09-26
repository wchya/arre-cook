const api = require("./api")

const TAB_PAGES = ["pages/home/home", "pages/dishes/dishes", "pages/history/history", "pages/me/me"]
const CONSENT_KEY = "ninimenu_privacy_consent_v1"

function hasSession() {
  return Boolean(api.token())
}

// 页面 onShow / onLoad 调用：没有登录态时回到登录页（带上回跳地址），返回 false 让页面停止加载。
function requireLogin(redirect) {
  if (hasSession()) return true
  const query = redirect ? `?redirect=${encodeURIComponent(redirect)}` : ""
  wx.reLaunch({ url: `/pages/login/login${query}` })
  return false
}

// 登录成功后的去向：Tab 页用 switchTab，其余页面 reLaunch（导航栏会显示“回首页”）。
function enterAfterLogin(redirect) {
  const target = redirect && redirect.charAt(0) === "/" && redirect.indexOf("/pages/login/") !== 0 ? redirect : ""
  if (!target) {
    wx.switchTab({ url: "/pages/home/home" })
    return
  }
  const path = target.split("?")[0].replace(/^\//, "")
  if (isTabPage(path)) wx.switchTab({ url: `/${path}` })
  else wx.reLaunch({ url: target })
}

// 当前用户（带缓存）：Web 端对应 useAuthStore.user。
function currentUser(force) {
  const app = getApp()
  if (!force && app.globalData.user) return Promise.resolve(app.globalData.user)
  return api.get("/me").then((user) => {
    app.globalData.user = user
    return user
  })
}

function setUser(user) {
  getApp().globalData.user = user || null
}

function logout(reason) {
  try { wx.removeStorageSync(api.SESSION_KEY) } catch (_) { /* ignore */ }
  const app = getApp()
  app.globalData.user = null
  app.globalData.sessionRestoreAttempted = true
  wx.reLaunch({ url: `/pages/login/login?reason=${reason || "logout"}` })
}

// 自定义 TabBar 选中态：每个 Tab 页 onShow 时同步。
function syncTabBar(page, index) {
  if (typeof page.getTabBar !== "function") return
  const tabBar = page.getTabBar()
  if (tabBar && tabBar.data.selected !== index) tabBar.setData({ selected: index })
}

function isTabPage(route) {
  return TAB_PAGES.indexOf(String(route || "").replace(/^\//, "")) >= 0
}

module.exports = { TAB_PAGES, CONSENT_KEY, hasSession, requireLogin, enterAfterLogin, currentUser, setUser, logout, syncTabBar, isTabPage }
