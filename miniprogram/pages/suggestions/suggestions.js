const api = require("../../utils/api")
const session = require("../../utils/session")
const ui = require("../../utils/ui")
const dishUtil = require("../../utils/dish")
const fmt = require("../../utils/format")

const STATUS = { pending: "待处理", accepted: "已采纳", dismissed: "已忽略", expired: "已过期" }

Page({
  data: { loading: true, history: false, items: [], busyId: 0 },

  onLoad() {
    if (!session.requireLogin("/pages/suggestions/suggestions")) return
    this.load()
  },

  async onPullDownRefresh() {
    await this.load()
    wx.stopPullDownRefresh()
  },

  async load() {
    this.setData({ loading: !this.data.items.length })
    try {
      const list = await api.get("/suggestions", { status: this.data.history ? "all" : "pending" })
      const items = (list || []).map((item) => ({
        id: item.id,
        title: item.title,
        reason: item.reason || "",
        source: item.source || "AI 助手",
        date: item.meal_date ? fmt.monthDay(item.meal_date.slice(0, 10)) : "未指定日期",
        status: item.status,
        statusLabel: STATUS[item.status] || item.status,
        pending: item.status === "pending",
        dishes: dishUtil.toCards(item.dishes || []),
      }))
      this.setData({ items })
    } catch (error) {
      ui.toast(error.message || "建议暂时无法加载")
    } finally {
      this.setData({ loading: false })
    }
  },

  toggleHistory() {
    ui.haptic()
    this.setData({ history: !this.data.history, items: [] }, () => this.load())
  },

  async resolve(event) {
    const id = Number(event.currentTarget.dataset.id)
    const accept = event.currentTarget.dataset.accept === true || event.currentTarget.dataset.accept === "true"
    const meal = event.currentTarget.dataset.meal
    const item = this.data.items.find((entry) => entry.id === id)
    if (!item || this.data.busyId) return
    if (!accept) {
      const ok = await ui.confirm({ title: "忽略这条建议？", confirmText: "忽略" })
      if (!ok) return
    }
    this.setData({ busyId: id })
    try {
      await api.post(`/suggestions/${id}/resolve`, accept ? { accept: true, meal_type: meal, dish_ids: item.dishes.map((dish) => dish.id) } : { accept: false })
      ui.toast(accept ? "已加入用餐记录" : "已忽略这条建议", accept ? "success" : "")
      await this.load()
    } catch (error) {
      ui.toast(error.message || "操作失败")
    } finally {
      this.setData({ busyId: 0 })
    }
  },

  openDish(event) {
    wx.navigateTo({ url: `/pages/dish/dish?id=${event.currentTarget.dataset.id}` })
  },

  goAgents() { wx.navigateTo({ url: "/pages/agents/agents" }) },
})
