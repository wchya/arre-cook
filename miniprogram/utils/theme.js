// 页面、原生导航、图标、弹层和 TabBar 共享一份外观状态。
const PALETTES = require("./theme-palettes")
const STORAGE_KEY = "ninimenu_appearance"
const listeners = new Set()
let appearance
let systemTheme = "light"
let persisted = true
let snapshot

function parse(raw) {
  try {
    const value = typeof raw === "string" ? JSON.parse(raw) : raw
    return {
      palette: value && Object.prototype.hasOwnProperty.call(PALETTES, value.palette) ? value.palette : "lime",
      mode: value && ["light", "dark", "system"].includes(value.mode) ? value.mode : "system",
    }
  } catch (_) { return { palette: "lime", mode: "system" } }
}

function readSystemTheme() {
  try {
    const base = typeof wx.getAppBaseInfo === "function" ? wx.getAppBaseInfo() : wx.getSystemInfoSync()
    return base && base.theme === "dark" ? "dark" : "light"
  } catch (_) { return "light" }
}

function build() {
  const p = PALETTES[appearance.palette]
  const mode = appearance.mode === "system" ? systemTheme : appearance.mode
  const dark = mode === "dark"
  const value = (key) => p[(dark ? "night-" : "") + key]
  const tokens = {
    bg: value("page"), card: value("card"), text: value("ink"), text2: value("muted"), text3: value("muted"), text4: value("muted"),
    primary: value("primary"), "primary-action": p.action, "primary-dark": p["action-hover"], "on-primary": p["on-action"],
    "primary-light": value("soft"), inset: value("inset"), "hero-warm": value("soft"), border: value("border"), border2: value("border"),
    mint: value("primary"), "mint-ink": value("primary"), "mint-action": p.action, "mint-light": value("inset"),
    pink: value("primary"), "pink-ink": value("primary"), "pink-action": p.action, "pink-light": value("soft"),
    yellow: value("primary"), "yellow-dark": value("primary"), "yellow-light": value("soft"),
    purple: value("primary"), "purple-light": value("inset"),
    red: dark ? "#FF9696" : "#DC2626", "red-light": dark ? "#4C2426" : "#FEE2E2", white: "#FFFFFF", wechat: "#07C160",
    "glass-bg": value("card"), "glass-strong": value("card"), "glass-border": value("border"),
    "glass-ios-bg": value("card"), "glass-ios-strong": value("card"),
    highlight: dark ? "rgba(255,255,255,.06)" : "rgba(255,255,255,.8)",
    scrim: "rgba(0,0,0,.48)", thumb: value("card"),
    "shadow-card": "0 4rpx 16rpx rgba(0,0,0,.03)", "shadow-float": "0 20rpx 60rpx rgba(0,0,0,.10)",
    "shadow-primary": "none", "glass-shadow": "none",
  }
  const style = Object.keys(tokens).map((key) => `--${key}:${tokens[key]}`).join(";") + ";"
  return { ...appearance, theme: mode, persisted, tokens, style, bg: tokens.bg, warm: tokens["hero-warm"], refresher: dark ? "white" : "black" }
}

function emit() {
  snapshot = build()
  listeners.forEach((fn) => { try { fn(snapshot) } catch (_) {} })
}

function ensure() {
  if (appearance) return
  try { appearance = parse(wx.getStorageSync(STORAGE_KEY)) } catch (_) { appearance = parse(null); persisted = false }
  systemTheme = readSystemTheme()
  snapshot = build()
  if (typeof wx.onThemeChange === "function") wx.onThemeChange((event) => {
    const next = event && event.theme === "dark" ? "dark" : "light"
    if (next === systemTheme) return
    systemTheme = next
    if (appearance.mode === "system") emit()
  })
}

function palette() { ensure(); return snapshot }
function name() { return palette().theme }
function subscribe(fn) { ensure(); listeners.add(fn); return () => listeners.delete(fn) }

function set(patch) {
  ensure()
  appearance = parse({ ...appearance, ...patch })
  try { wx.setStorageSync(STORAGE_KEY, JSON.stringify(appearance)); persisted = true } catch (_) { persisted = false }
  emit()
  return snapshot
}

function refresh() {
  ensure()
  const next = readSystemTheme()
  if (next !== systemTheme) {
    systemTheme = next
    if (appearance.mode === "system") emit()
  }
}

function refresherData(p) { return { refresherBg: p.bg, refresherWarm: p.warm, refresherStyle: p.refresher } }
function bindRefresher(page) {
  page.setData(refresherData(palette()))
  return subscribe((p) => page.setData(refresherData(p)))
}

function nativeColors(p) {
  const ignore = () => {}
  if (typeof wx.setBackgroundColor === "function") wx.setBackgroundColor({ backgroundColor: p.bg, backgroundColorTop: p.bg, backgroundColorBottom: p.bg, fail: ignore })
  if (typeof wx.setBackgroundTextStyle === "function") wx.setBackgroundTextStyle({ textStyle: p.theme === "dark" ? "light" : "dark", fail: ignore })
  if (typeof wx.setNavigationBarColor === "function") wx.setNavigationBarColor({ frontColor: p.theme === "dark" ? "#ffffff" : "#000000", backgroundColor: p.bg, fail: ignore })
}

function pageData(p) { return { appearanceStyle: p.style + `background:${p.bg};color:${p.tokens.text};`, ...refresherData(p) } }

// 显式包装 Page 注册，保留所有原生命周期的参数、this 和返回值。
// 每个页面顶部必须有 page-meta；隐藏页面也更新令牌，但只有前台页修改原生导航。
function page(options) {
  const { onLoad, onShow, onUnload } = options
  return Page({
    ...options,
    data: { ...options.data, ...pageData(palette()) },
    onLoad(...args) {
      const paint = (p) => {
        this.setData(pageData(p))
        const pages = getCurrentPages()
        if (pages[pages.length - 1] === this) nativeColors(p)
      }
      this._offAppearance = subscribe(paint)
      paint(palette())
      return onLoad && onLoad.apply(this, args)
    },
    onShow(...args) {
      refresh()
      this.setData(pageData(palette()))
      nativeColors(palette())
      return onShow && onShow.apply(this, args)
    },
    onUnload(...args) {
      if (this._offAppearance) this._offAppearance()
      return onUnload && onUnload.apply(this, args)
    },
  })
}

module.exports = { STORAGE_KEY, parse, name, palette, subscribe, set, refresh, bindRefresher, page }
