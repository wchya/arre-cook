function platform(raw) {
  if (typeof raw !== "string" || raw.length > 6144) return null
  const links = raw.match(/https?:\/\/[^\s<>"'，。；！）】]+/g)
  if (!links || links.length !== 1) return null
  try {
    const match = links[0].replace(/[.,;!)\]}]+$/, "").match(/^(https?):\/\/([a-zA-Z0-9.-]+)(?::([0-9]+))?(\/[^?#]*)?(?:\?([^#]*))?(?:#.*)?$/)
    if (!match || match[3] && match[3] !== (match[1] === "https" ? "443" : "80")) return null
    const host = match[2].toLowerCase(), pathname = match[4] || "/"
    const params = {}
    for (const pair of (match[5] || "").split("&")) {
      const parts = pair.split("=")
      const key = decodeURIComponent(parts.shift().replace(/\+/g, " "))
      const value = decodeURIComponent(parts.join("=").replace(/\+/g, " "))
      if (key === "p" || key === "modal_id") {
        if (Object.prototype.hasOwnProperty.call(params, key)) return null
        params[key] = value
      }
    }
    if (host === "b23.tv" && /^\/[a-zA-Z0-9_-]{1,80}\/?$/.test(pathname)) return { url: "https://b23.tv" + pathname, key: "bilibili", name: "哔哩哔哩" }
    if (["bilibili.com", "www.bilibili.com", "m.bilibili.com"].indexOf(host) >= 0 && /^\/video\/(BV[0-9A-Za-z]{10}|av[0-9]{1,20})\/?$/.test(pathname)) {
      const page = params.p === undefined ? 1 : Number(params.p)
      if (!Number.isInteger(page) || page < 1 || page > 100) return null
      return { url: "https://www.bilibili.com" + pathname.replace(/\/$/, "") + "/" + (page > 1 ? "?p=" + page : ""), key: "bilibili", name: "哔哩哔哩" }
    }
    if (host === "v.douyin.com" && /^\/[a-zA-Z0-9_-]{1,80}\/?$/.test(pathname)) return { url: "https://v.douyin.com" + pathname, key: "douyin", name: "抖音" }
    if (["douyin.com", "www.douyin.com", "iesdouyin.com", "www.iesdouyin.com"].indexOf(host) >= 0) {
      const direct = pathname.match(/^\/(?:share\/)?video\/([0-9]{1,20})\/?$/)
      const id = direct && direct[1] || (pathname === "/" ? params.modal_id : "")
      if (id && /^[0-9]{1,20}$/.test(id)) return { url: "https://www.douyin.com/video/" + id, key: "douyin", name: "抖音" }
    }
  } catch (_) { /* ignore malformed historical links */ }
  return null
}

function canOpenDirectly() { return typeof wx !== "undefined" && typeof wx.openUrl === "function" }

function actionLabel(url) {
  const info = platform(url)
  const name = info ? info.name : "平台"
  return canOpenDirectly() ? "在" + name + "打开视频" : "复制链接到" + name + "播放"
}

function open(url, onFallback) {
  const info = platform(url)
  if (!info) return false
  const value = info.url
  if (canOpenDirectly()) {
    try {
      wx.openUrl({ url: value, fail: () => fallback(info, onFallback) })
      return true
    } catch (_) { /* older base libraries may expose but reject this API */ }
  }
  fallback(info, onFallback)
  return true
}

function fallback(info, onFallback) {
  wx.setClipboardData({ data: info.url, success: () => {
    if (typeof onFallback === "function") onFallback(info.name)
  }, fail: () => {
    if (typeof wx.showToast === "function") wx.showToast({ title: "复制失败，请稍后重试", icon: "none" })
  } })
}

module.exports = { platform, open, actionLabel, canOpenDirectly }
