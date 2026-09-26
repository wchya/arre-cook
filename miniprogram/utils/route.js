// 把 Web 端路由（站内信 link、分享路径等）映射到小程序页面。
const session = require("./session")

const STATIC = {
  "/": "/pages/home/home",
  "/dishes": "/pages/dishes/dishes",
  "/history": "/pages/history/history",
  "/me": "/pages/me/me",
  "/favorites": "/pages/favorites/favorites",
  "/plan": "/pages/plan/plan",
  "/more": "/pages/plan/plan",
  "/tomorrow": "/pages/tomorrow/tomorrow",
  "/taste-profile": "/pages/taste-profile/taste-profile",
  "/suggestions": "/pages/suggestions/suggestions",
  "/achievements": "/pages/achievements/achievements",
  "/me/preferences": "/pages/preferences/preferences",
  "/me/ai": "/pages/agents/agents",
  "/me/account": "/pages/account/account",
  "/family": "/pages/family/family",
  "/health": "/pages/diary/diary",
  "/notifications": "/pages/notifications/notifications",
  "/photo-wall": "/pages/photo-wall/photo-wall",
  "/assistant": "/pages/assistant/assistant",
  "/assistant/chat": "/pages/chat/chat",
  "/dishes/new": "/pages/dish-edit/dish-edit",
}

function resolve(link) {
  if (!link || typeof link !== "string") return ""
  let path = link.trim()
  if (/^https?:\/\//i.test(path)) path = path.replace(/^https?:\/\/[^/]+/i, "")
  const [pathname, query = ""] = path.split("?")
  const clean = pathname.replace(/\/+$/, "") || "/"
  const dish = clean.match(/^\/dishes\/(\d+)(\/(cook|edit))?$/)
  if (dish) {
    if (dish[3] === "cook") return `/pages/cook/cook?id=${dish[1]}`
    if (dish[3] === "edit") return `/pages/dish-edit/dish-edit?id=${dish[1]}`
    return `/pages/dish/dish?id=${dish[1]}`
  }
  const target = STATIC[clean]
  if (!target) return ""
  return query ? `${target}?${query}` : target
}

// 打开 Web 路由对应的小程序页面；找不到对应页面时返回 false。
function open(link) {
  const url = resolve(link)
  if (!url) return false
  const path = url.split("?")[0].replace(/^\//, "")
  if (session.isTabPage(path)) wx.switchTab({ url: `/${path}` })
  else wx.navigateTo({ url })
  return true
}

module.exports = { resolve, open }
