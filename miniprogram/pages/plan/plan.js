const api = require("../../utils/api")
const session = require("../../utils/session")
const ui = require("../../utils/ui")
const fmt = require("../../utils/format")

const DAY_ORDER = { 周一: 1, 周二: 2, 周三: 3, 周四: 4, 周五: 5, 周六: 6, 周日: 7 }

function dishChips(list) {
  return (list || []).map((dish) => ({ id: dish.id, name: dish.name }))
}

Page({
  data: {
    tab: "week",
    tabIndex: 0,
    days: [],
    showAll: false,
    visibleDays: [],
    weekLoading: true,
    regenerating: false,

    categories: [],
    shoppingLoading: true,
    totalItems: 0,
    needBuy: 0,
    checkedCount: 0,
    stockCount: 0,

    quickDay: null,
    quickOpen: false,
    recording: false,
  },

  onLoad(query) {
    if (query.tab === "shopping") this.setData({ tab: "shopping", tabIndex: 1 })
    if (!session.requireLogin("/pages/plan/plan")) return
    this.loadWeek()
    this.loadShopping()
  },

  async onPullDownRefresh() {
    await Promise.all([this.loadWeek(), this.loadShopping()])
    wx.stopPullDownRefresh()
  },

  switchTab(event) {
    const tab = event.currentTarget.dataset.tab
    if (tab === this.data.tab) return
    ui.haptic()
    this.setData({ tab, tabIndex: tab === "week" ? 0 : 1 })
  },

  // ---------- 一周菜单 ----------

  async loadWeek() {
    try {
      const plan = await api.get("/week-plan")
      const today = fmt.dateKey()
      const tomorrow = fmt.dateKey(fmt.addDays(new Date(), 1))
      const days = (plan.days || []).slice()
        .sort((a, b) => (DAY_ORDER[a.day_name] || 0) - (DAY_ORDER[b.day_name] || 0))
        .map((day) => ({
          date: day.date,
          dayName: day.day_name,
          short: day.date.slice(5),
          tag: day.date === today ? "今天" : day.date === tomorrow ? "明天" : "",
          quick: day.date === today || day.date === tomorrow,
          lunch: dishChips(day.lunch),
          dinner: dishChips(day.dinner),
        }))
      this._days = plan.days || []
      this.setData({ days })
      this.applyVisible()
    } catch (error) {
      ui.toast(error.message || "一周菜单加载失败")
    } finally {
      this.setData({ weekLoading: false })
    }
  },

  applyVisible() {
    const days = this.data.days
    const near = days.filter((day) => day.quick)
    const collapsed = near.length ? near : days.slice(0, 2)
    this.setData({ visibleDays: this.data.showAll ? days : collapsed })
  },

  toggleAll() {
    ui.haptic()
    this.setData({ showAll: !this.data.showAll }, () => this.applyVisible())
  },

  async regenerate() {
    if (this.data.regenerating) return
    const ok = await ui.confirm({ title: "重新生成一周菜单？", content: "会按你的口味和偏好重新安排 7 天的午晚餐。", confirmText: "重新生成" })
    if (!ok) return
    this.setData({ regenerating: true })
    try {
      await api.post("/week-plan/regenerate")
      await Promise.all([this.loadWeek(), this.loadShopping()])
      ui.toast("已重新生成", "success")
    } catch (error) {
      ui.toast(error.message || "重新生成失败")
    } finally {
      this.setData({ regenerating: false })
    }
  },

  openDish(event) {
    wx.navigateTo({ url: `/pages/dish/dish?id=${event.currentTarget.dataset.id}` })
  },

  openQuick(event) {
    const date = event.currentTarget.dataset.date
    const day = this.data.days.find((item) => item.date === date)
    if (!day || !day.quick) return
    this.setData({ quickDay: day, quickOpen: true })
  },

  closeQuick() { this.setData({ quickOpen: false }) },

  async confirmQuick() {
    const day = this.data.quickDay
    if (!day || this.data.recording) return
    const records = []
    day.lunch.forEach((dish) => records.push({ dish_id: dish.id, dish_name: dish.name, meal_type: "lunch", meal_date: day.date }))
    day.dinner.forEach((dish) => records.push({ dish_id: dish.id, dish_name: dish.name, meal_type: "dinner", meal_date: day.date }))
    if (!records.length) return
    this.setData({ recording: true })
    try {
      const result = await api.post("/records/batch", { records })
      this.setData({ quickOpen: false })
      ui.toast(result.skipped ? `已记录 ${result.created.length} 道，${result.skipped} 道已存在` : "❤ 已记录！", result.skipped ? "" : "success")
      this.loadShopping()
    } catch (error) {
      ui.toast(error.message || "记录失败")
    } finally {
      this.setData({ recording: false })
    }
  },

  // ---------- 买菜清单 ----------

  async loadShopping() {
    try {
      const list = await api.get("/shopping-list")
      this.applyShopping(list || [])
    } catch (error) {
      ui.toast(error.message || "买菜清单加载失败")
    } finally {
      this.setData({ shoppingLoading: false })
    }
  },

  applyShopping(list) {
    let total = 0
    let needBuy = 0
    let checked = 0
    let stock = 0
    const categories = list.map((cat, ci) => {
      const items = (cat.items || []).map((item, ii) => {
        total += 1
        if (item.checked) checked += 1
        if (item.in_stock) stock += 1
        if (!item.checked && !item.in_stock) needBuy += 1
        return { key: `${ci}-${ii}`, name: item.name, amount: item.amount || "", checked: Boolean(item.checked), stock: Boolean(item.in_stock) }
      })
      return { name: cat.category, items, done: items.every((item) => item.checked || item.stock) }
    })
    this._shopping = list
    this.setData({ categories, totalItems: total, needBuy, checkedCount: checked, stockCount: stock })
  },

  patchItem(name, patch) {
    const list = (this._shopping || []).map((cat) => ({
      ...cat,
      items: (cat.items || []).map((item) => (item.name === name ? { ...item, ...patch } : item)),
    }))
    this.applyShopping(list)
  },

  async toggleCheck(event) {
    const name = event.currentTarget.dataset.name
    const checked = event.currentTarget.dataset.checked === true || event.currentTarget.dataset.checked === "true"
    const stock = event.currentTarget.dataset.stock === true || event.currentTarget.dataset.stock === "true"
    if (stock) return
    ui.haptic()
    this.patchItem(name, { checked: !checked })
    try {
      await api.post("/shopping-list/toggle", { item_name: name, meal_date: fmt.dateKey(), checked: !checked })
    } catch (error) {
      this.patchItem(name, { checked })
      ui.toast(error.message || "更新失败")
    }
  },

  async toggleStock(event) {
    const name = event.currentTarget.dataset.name
    const stock = event.currentTarget.dataset.stock === true || event.currentTarget.dataset.stock === "true"
    ui.haptic()
    this.patchItem(name, { in_stock: !stock })
    try {
      await api.post("/shopping-list/inventory", { item_name: name, in_stock: !stock })
    } catch (error) {
      this.patchItem(name, { in_stock: stock })
      ui.toast(error.message || "库存状态更新失败")
    }
  },

  copyList() {
    const lines = []
    this.data.categories.forEach((cat) => {
      const pending = cat.items.filter((item) => !item.checked && !item.stock)
      if (!pending.length) return
      lines.push(`【${cat.name}】`)
      pending.forEach((item) => lines.push(`□ ${item.name}${item.amount ? `  ${item.amount}` : ""}`))
    })
    if (!lines.length) {
      ui.toast("没有待买的食材啦")
      return
    }
    wx.setClipboardData({
      data: `🛒 NiniMenu 买菜清单\n${lines.join("\n")}`,
      success: () => ui.toast("清单已复制，可以发给家人"),
    })
  },

  goTomorrow() { wx.navigateTo({ url: "/pages/tomorrow/tomorrow" }) },
  goDishes() { wx.switchTab({ url: "/pages/dishes/dishes" }) },
})
