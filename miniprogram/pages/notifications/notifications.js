const api = require("../../utils/api")
const session = require("../../utils/session")
const ui = require("../../utils/ui")
const route = require("../../utils/route")
const fmt = require("../../utils/format")

const META = {
  system_update: { icon: "bell", tone: "primary", color: "primary", label: "系统" },
  feature: { icon: "sparkles", tone: "purple", color: "purple", label: "新功能" },
  maintenance: { icon: "wrench", tone: "yellow", color: "yellow-dark", label: "维护" },
  health_tip: { icon: "heart-pulse", tone: "mint", color: "mint-ink", label: "健康" },
  agent_security: { icon: "shield-check", tone: "red", color: "red", label: "安全" },
  family: { icon: "users-round", tone: "yellow", color: "yellow-dark", label: "家庭" },
  shopping: { icon: "shopping-cart", tone: "mint", color: "mint-ink", label: "买菜" },
}

function timeLabel(raw) {
  const date = fmt.parseDate(raw)
  if (!date) return ""
  const diff = Date.now() - date.getTime()
  if (diff < 60000) return "刚刚"
  if (diff < 3600000) return `${Math.floor(diff / 60000)} 分钟前`
  if (diff < 86400000) return `${Math.floor(diff / 3600000)} 小时前`
  return fmt.relativeDate(raw)
}

Page({
  data: { loading: true, items: [], unread: 0, marking: false },

  onLoad() {
    if (!session.requireLogin("/pages/notifications/notifications")) return
    this.load()
  },

  async onPullDownRefresh() {
    await this.load()
    wx.stopPullDownRefresh()
  },

  async load() {
    try {
      const result = await api.get("/notifications", { pageSize: 50 })
      const items = (result.items || []).map((item) => {
        const meta = META[item.type] || META.system_update
        return { ...meta, id: item.id, title: item.title, content: item.content, link: item.link || "", linkable: Boolean(route.resolve(item.link)), read: Boolean(item.read_at), time: timeLabel(item.created_at) }
      })
      this.setData({ items, unread: result.unread || 0 })
    } catch (error) {
      ui.toast(error.message || "消息暂时无法加载")
    } finally {
      this.setData({ loading: false })
    }
  },

  async open(event) {
    const id = Number(event.currentTarget.dataset.id)
    const item = this.data.items.find((entry) => entry.id === id)
    if (!item) return
    if (!item.read) {
      const index = this.data.items.indexOf(item)
      this.setData({ [`items[${index}].read`]: true, unread: Math.max(0, this.data.unread - 1) })
      api.post(`/notifications/${id}/read`).catch(() => null)
    }
    if (item.link && !route.open(item.link)) {
      wx.setClipboardData({ data: item.link, success: () => ui.toast("链接已复制") })
    }
  },

  async markAll() {
    if (!this.data.unread || this.data.marking) return
    this.setData({ marking: true })
    try {
      await api.post("/notifications/read-all")
      this.setData({ items: this.data.items.map((item) => ({ ...item, read: true })), unread: 0 })
      ui.toast("已全部标记为已读", "success")
    } catch (error) {
      ui.toast(error.message || "操作失败")
    } finally {
      this.setData({ marking: false })
    }
  },
})
