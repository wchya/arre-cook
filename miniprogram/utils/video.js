function platform(url) {
  try {
    const match = String(url).match(/^https?:\/\/([^/?#]+)/i)
    const host = String((match && match[1]) || "").toLowerCase().replace(/:\d+$/, "").replace(/\.$/, "")
    if (host === "bilibili.com" || host.endsWith(".bilibili.com") || host === "b23.tv" || host.endsWith(".b23.tv")) {
      return { key: "bilibili", name: "哔哩哔哩" }
    }
    if (host === "douyin.com" || host.endsWith(".douyin.com") || host === "iesdouyin.com" || host.endsWith(".iesdouyin.com")) {
      return { key: "douyin", name: "抖音" }
    }
  } catch (_) { /* ignore malformed historical links */ }
  return null
}

function open(url, onFallback) {
  const value = String(url || "").trim()
  if (!value) return false
  if (!platform(value)) return false
  if (typeof wx.openUrl === "function") {
    try {
      wx.openUrl({ url: value, fail: () => fallback(value, onFallback) })
      return true
    } catch (_) { /* older base libraries may expose but reject this API */ }
  }
  fallback(value, onFallback)
  return true
}

function fallback(url, onFallback) {
  wx.setClipboardData({ data: url, success: () => {
    if (typeof onFallback === "function") onFallback()
  } })
}

module.exports = { platform, open }
