const API_BASE = "https://cook.arrebyte.top/api"

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

App({
  globalData: {
    apiBase: API_BASE,
    origin: API_BASE.replace(/\/api\/?$/, ""),
    sessionRestoreAttempted: false,
    nav: null,
    user: null,
  },

  onLaunch() {
    this.globalData.nav = readNavMetrics()
  },

  navMetrics() {
    if (!this.globalData.nav) this.globalData.nav = readNavMetrics()
    return this.globalData.nav
  },
})
