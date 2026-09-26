// 系统深浅色主题：CSS 由 app.wxss 的 prefers-color-scheme 媒体查询自动切换；
// 只能在 JS / WXML 属性里写颜色的地方（图标取色、下拉刷新底色与加载点、透明导航栏渐显底色）
// 从这里取值，并通过 subscribe 在系统切换深浅色时刷新。取值与 app.wxss / theme.json 保持一致。
const PALETTES = {
  light: { theme: "light", bg: "#FAFAF8", warm: "#FFE9DE", refresher: "black", navRgb: "250, 250, 248" },
  dark: { theme: "dark", bg: "#121016", warm: "#45291D", refresher: "white", navRgb: "18, 16, 22" },
}

const listeners = new Set()
let current = ""

function readSystemTheme() {
  try {
    const base = typeof wx.getAppBaseInfo === "function" ? wx.getAppBaseInfo() : wx.getSystemInfoSync()
    return base && base.theme === "dark" ? "dark" : "light"
  } catch (_) {
    return "light"
  }
}

// 单点监听系统主题变化，广播给所有订阅者。
function ensureWatch() {
  if (current) return
  current = readSystemTheme()
  if (typeof wx.onThemeChange !== "function") return
  wx.onThemeChange((res) => {
    const next = res && res.theme === "dark" ? "dark" : "light"
    if (next === current) return
    current = next
    listeners.forEach((fn) => {
      try { fn(PALETTES[next]) } catch (_) {}
    })
  })
}

function name() {
  ensureWatch()
  return current
}

function palette() {
  return PALETTES[name()]
}

// 订阅深浅色切换，回调参数为新色板；返回取消订阅函数（页面 onUnload / 组件 detached 时调用）。
function subscribe(fn) {
  ensureWatch()
  listeners.add(fn)
  return () => listeners.delete(fn)
}

module.exports = { name, palette, subscribe }
