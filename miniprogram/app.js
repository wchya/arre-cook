const API_BASE = "https://cook.arrebyte.top/api"
const theme = require("./utils/theme")

function readWindowInfo() {
  try {
    return typeof wx.getWindowInfo === "function" ? wx.getWindowInfo() : wx.getSystemInfoSync()
  } catch (_) {
    return {}
  }
}

// 自定义导航栏尺寸：按胶囊按钮位置推算，保证标题与胶囊垂直居中对齐。
function readNavMetrics() {
  const info = readWindowInfo()
  const statusBarHeight = info.statusBarHeight || 20
  const windowWidth = info.windowWidth || 375
  let menu = null
  try { menu = wx.getMenuButtonBoundingClientRect() } catch (_) { menu = null }
  if (!menu || !menu.height || !menu.top) {
    menu = { top: statusBarHeight + 6, height: 32, left: windowWidth - 97 }
  }
  const navBarHeight = Math.max(40, (menu.top - statusBarHeight) * 2 + menu.height)
  return {
    statusBarHeight,
    navBarHeight,
    navHeight: statusBarHeight + navBarHeight,
    capsuleSpace: Math.max(0, windowWidth - menu.left) + 8,
    windowWidth,
    windowHeight: info.windowHeight || 667,
  }
}

// 读取系统主题与平台：主题用于图标等 JS 侧取色，平台用于毛玻璃能力判断。
function detectEnv() {
  let platform = ""
  try {
    const device = typeof wx.getDeviceInfo === "function" ? wx.getDeviceInfo() : readWindowInfo()
    if (device && device.platform) platform = device.platform
  } catch (_) {}
  return { theme: theme.name(), platform, isIOS: platform === "ios" }
}

App({
  globalData: {
    apiBase: API_BASE,
    origin: API_BASE.replace(/\/api\/?$/, ""),
    sessionRestoreAttempted: false,
    nav: null,
    user: null,
    theme: "light",
    platform: "",
    isIOS: false,
  },

  onLaunch() {
    this.globalData.nav = readNavMetrics()
    Object.assign(this.globalData, detectEnv())
    // 系统主题切换时更新（CSS 走媒体查询自动响应，JS 侧取色统一经 utils/theme 订阅）。
    theme.subscribe((palette) => { this.globalData.theme = palette.theme })
  },

  navMetrics() {
    if (!this.globalData.nav) this.globalData.nav = readNavMetrics()
    return this.globalData.nav
  },
})
