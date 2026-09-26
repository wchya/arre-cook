// 图标组件：渲染 lucide 图标（与 Web 端同一套），颜色可用色板名或十六进制。
// <icon name="heart" size="36" color="primary" stroke="2.4" fill />
const ICONS = require("../../utils/icons")

const TOKENS = {
  text: "#1A1A2E",
  text2: "#6B7280",
  text3: "#9CA3AF",
  text4: "#D1D5DB",
  primary: "#E8734A",
  "primary-dark": "#C85A35",
  mint: "#6EC6B8",
  "mint-ink": "#3F9D90",
  pink: "#F4A8A0",
  "pink-ink": "#E07A6E",
  yellow: "#F5D76E",
  "yellow-dark": "#A67912",
  purple: "#8B5CF6",
  red: "#DC2626",
  wechat: "#07C160",
  bg: "#FAFAF8",
  white: "#FFFFFF",
}

// 深色模式取色（与 app.wxss 深色令牌一致）；图标颜色在 JS 侧解析，无法走 CSS 变量。
const TOKENS_DARK = {
  text: "#F4F2F7",
  text2: "#A7A2B0",
  text3: "#6E6979",
  text4: "#45414D",
  primary: "#F0805A",
  "primary-dark": "#F5936F",
  mint: "#5FBFB0",
  "mint-ink": "#6FD8C6",
  pink: "#E89A91",
  "pink-ink": "#F3A99E",
  yellow: "#E9C65E",
  "yellow-dark": "#E7C25C",
  purple: "#A98BFF",
  red: "#F26D6D",
  wechat: "#07C160",
  bg: "#121016",
  white: "#FFFFFF",
}

// 单点订阅系统主题变化，广播给所有已挂载的图标实例重新取色。
const subscribers = new Set()
let THEME = null
function currentTheme() {
  try {
    const app = getApp()
    if (app && app.globalData && app.globalData.theme) return app.globalData.theme
  } catch (_) {}
  try {
    if (typeof wx.getAppBaseInfo === "function") {
      const base = wx.getAppBaseInfo()
      if (base && base.theme) return base.theme
    }
  } catch (_) {}
  return "light"
}
function ensureThemeWatch() {
  if (THEME !== null) return
  THEME = currentTheme()
  if (typeof wx.onThemeChange === "function") {
    wx.onThemeChange((res) => {
      THEME = (res && res.theme) || "light"
      subscribers.forEach((fn) => fn())
    })
  }
}

const B64 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
const cache = {}

function base64(input) {
  let output = ""
  for (let i = 0; i < input.length; i += 3) {
    const a = input.charCodeAt(i) & 0xff
    const hasB = i + 1 < input.length
    const hasC = i + 2 < input.length
    const b = hasB ? input.charCodeAt(i + 1) & 0xff : 0
    const c = hasC ? input.charCodeAt(i + 2) & 0xff : 0
    const n = (a << 16) | (b << 8) | c
    output += B64[(n >> 18) & 63] + B64[(n >> 12) & 63] + (hasB ? B64[(n >> 6) & 63] : "=") + (hasC ? B64[n & 63] : "=")
  }
  return output
}

function build(name, color, stroke, fill, theme) {
  const map = theme === "dark" ? TOKENS_DARK : TOKENS
  const hex = map[color] || color || map.text2
  const width = stroke || 2
  const key = `${name}|${hex}|${width}|${fill ? 1 : 0}`
  if (cache[key]) return cache[key]
  let svg = ""
  if (ICONS.filled[name]) {
    svg = `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="${hex}">${ICONS.filled[name]}</svg>`
  } else if (ICONS.stroke[name]) {
    svg = `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="${fill ? hex : "none"}" stroke="${hex}" stroke-width="${width}" stroke-linecap="round" stroke-linejoin="round">${ICONS.stroke[name]}</svg>`
  } else {
    return ""
  }
  cache[key] = `data:image/svg+xml;base64,${base64(svg)}`
  return cache[key]
}

Component({
  properties: {
    name: { type: String, value: "" },
    size: { type: Number, value: 36 },
    color: { type: String, value: "text2" },
    stroke: { type: Number, value: 2 },
    fill: { type: Boolean, value: false },
  },

  data: { src: "" },

  methods: {
    _paint() {
      const { name, color, stroke, fill } = this.data
      const src = build(name, color, stroke, fill, THEME || currentTheme())
      if (src !== this.data.src) this.setData({ src })
    },
  },

  observers: {
    "name, color, stroke, fill": function () {
      this._paint()
    },
  },

  lifetimes: {
    attached() {
      ensureThemeWatch()
      this._onTheme = () => this._paint()
      subscribers.add(this._onTheme)
      this._paint()
    },
    detached() {
      if (this._onTheme) subscribers.delete(this._onTheme)
    },
  },
})
