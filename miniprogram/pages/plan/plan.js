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

    // 买菜清单
    scope: "personal",
    hasFamily: false,
    familyName: "",
    reminderEnabled: true,
    shoppingLoading: true,

    personalCats: [],
    familyCats: [],
    familyManual: [],
    curMeals: [],

    curTotal: 0,
    curNeed: 0,
    curChecked: 0,
    curStock: 0,
    needBuy: 0,

    quickDay: null,
    quickOpen: false,
    recording: false,
  },

  onLoad(query) {
    if (query.tab === "shopping" || query.tab === "family-shopping") {
      this.setData({ tab: "shopping", tabIndex: 1 })
      this._wantScope = query.tab === "family-shopping" ? "family" : "personal"
    }
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
      const data = await api.get("/shopping/overview")
      this.applyOverview(data || {})
    } catch (error) {
      ui.toast(error.message || "买菜清单加载失败")
    } finally {
      this.setData({ shoppingLoading: false })
    }
  },

  applyOverview(data) {
    const personal = data.personal || {}
    const family = data.family || null
    const personalCats = this.mapCats(personal.categories, "p", true)
    let familyCats = []
    let familyManual = []
    let familyMeals = []
    if (family) {
      familyCats = this.mapCats(family.categories, "f", false)
      familyManual = (family.manual || []).map((it) => ({ id: it.id, name: it.name, amount: it.amount || "", checked: Boolean(it.checked) }))
      familyMeals = this.mapMeals(family.meals)
    }
    let scope = this.data.scope
    if (this._wantScope) scope = this._wantScope === "family" && family ? "family" : "personal"
    if (scope === "family" && !family) scope = "personal"
    this._wantScope = null
    this._personalMeals = this.mapMeals(personal.meals)
    this._familyMeals = familyMeals
    this.setData({
      hasFamily: Boolean(family),
      familyName: family ? family.family_name || "家庭" : "",
      reminderEnabled: data.reminder_enabled !== false,
      personalCats,
      familyCats,
      familyManual,
      scope,
    }, () => this.refreshStats())
  },

  mapCats(list, prefix, withStock) {
    return (list || []).map((cat, ci) => {
      const items = (cat.items || []).map((item, ii) => ({
        key: `${prefix}${ci}-${ii}`,
        name: item.name,
        amount: item.amount || "",
        checked: Boolean(item.checked),
        stock: withStock ? Boolean(item.in_stock) : false,
      }))
      return { name: cat.category, items, done: items.every((it) => it.checked || it.stock) }
    })
  },
  mapMeals(meals) {
    return (meals || []).map((m) => ({
      key: `${m.meal_date}-${m.meal_type}-${m.dish_id}`,
      label: `${fmt.relativeDate(m.meal_date)}${m.meal_type === "lunch" ? "午餐" : "晚餐"}`,
      dish: m.dish_name,
      by: m.added_by_name || "",
      lunch: m.meal_type === "lunch",
    }))
  },

  statOf(cats, manual) {
    let total = 0
    let need = 0
    let checked = 0
    let stock = 0
    ;(cats || []).forEach((cat) => cat.items.forEach((it) => {
      total += 1
      if (it.checked) checked += 1
      if (it.stock) stock += 1
      if (!it.checked && !it.stock) need += 1
    }))
    ;(manual || []).forEach((it) => {
      total += 1
      if (it.checked) checked += 1
      else need += 1
    })
    return { total, need, checked, stock }
  },
  refreshStats() {
    const pStat = this.statOf(this.data.personalCats)
    const fStat = this.statOf(this.data.familyCats, this.data.familyManual)
    const family = this.data.scope === "family"
    const cur = family ? fStat : pStat
    this.setData({
      curTotal: cur.total, curNeed: cur.need, curChecked: cur.checked, curStock: cur.stock,
      curMeals: family ? this._familyMeals : this._personalMeals,
      needBuy: pStat.need + fStat.need,
    })
  },

  switchScope(event) {
    const scope = event.currentTarget.dataset.scope
    if (scope === this.data.scope) return
    ui.haptic()
    this.setData({ scope }, () => this.refreshStats())
  },

  // 个人清单：按食材名勾选已买 / 标记家中库存（今明两天同名一起变）
  patchPersonal(name, patch) {
    const personalCats = this.data.personalCats.map((cat) => {
      const items = cat.items.map((it) => (it.name === name ? { ...it, ...patch } : it))
      return { ...cat, items, done: items.every((it) => it.checked || it.stock) }
    })
    this.setData({ personalCats }, () => this.refreshStats())
  },

  // 家庭自动清单：按食材名勾选（今明两天同名一起变）
  patchFamilyAuto(name, patch) {
    const familyCats = this.data.familyCats.map((cat) => {
      const items = cat.items.map((it) => (it.name === name ? { ...it, ...patch } : it))
      return { ...cat, items, done: items.every((it) => it.checked) }
    })
    this.setData({ familyCats }, () => this.refreshStats())
  },

  async toggleFamilyAuto(event) {
    const ds = event.currentTarget.dataset
    const name = ds.name
    const next = !(ds.checked === true || ds.checked === "true")
    ui.haptic()
    this.patchFamilyAuto(name, { checked: next })
    try {
      await api.post("/family/shopping/toggle", { item_name: name, checked: next })
    } catch (error) {
      this.patchFamilyAuto(name, { checked: !next })
      ui.toast(error.message || "更新失败")
    }
  },
  patchFamilyManual(id, patch) {
    const familyManual = this.data.familyManual.map((it) => (it.id === id ? { ...it, ...patch } : it))
    this.setData({ familyManual }, () => this.refreshStats())
  },

  async toggleFamilyManual(event) {
    const id = Number(event.currentTarget.dataset.id)
    const next = !(event.currentTarget.dataset.checked === true || event.currentTarget.dataset.checked === "true")
    ui.haptic()
    this.patchFamilyManual(id, { checked: next })
    try {
      await api.patch(`/family/shopping/${id}`, { checked: next })
    } catch (error) {
      this.patchFamilyManual(id, { checked: !next })
      ui.toast(error.message || "更新失败")
    }
  },

  async deleteFamilyManual(event) {
    const id = Number(event.currentTarget.dataset.id)
    ui.haptic()
    const prev = this.data.familyManual
    this.setData({ familyManual: prev.filter((it) => it.id !== id) }, () => this.refreshStats())
    try {
      await api.delete(`/family/shopping/${id}`)
    } catch (error) {
      this.setData({ familyManual: prev }, () => this.refreshStats())
      ui.toast(error.message || "删除失败")
    }
  },
  addFamilyItem() {
    wx.showModal({
      title: "添加到家庭清单",
      editable: true,
      placeholderText: "如：鸡蛋 6个",
      success: async (res) => {
        if (!res.confirm) return
        const text = (res.content || "").trim()
        if (!text) return
        const parts = text.split(/\s+/)
        let name = text
        let amount = ""
        if (parts.length > 1) {
          amount = parts.pop()
          name = parts.join(" ")
        }
        try {
          await api.post("/family/shopping", { name, amount })
          ui.toast("已添加")
          this.loadShopping()
        } catch (error) {
          ui.toast(error.message || "添加失败")
        }
      },
    })
  },

  async toggleCheck(event) {
    const ds = event.currentTarget.dataset
    if (ds.stock === true || ds.stock === "true") return
    const name = ds.name
    const next = !(ds.checked === true || ds.checked === "true")
    ui.haptic()
    this.patchPersonal(name, { checked: next })
    try {
      await api.post("/shopping-list/toggle", { item_name: name, checked: next })
    } catch (error) {
      this.patchPersonal(name, { checked: !next })
      ui.toast(error.message || "更新失败")
    }
  },

  async toggleStock(event) {
    const ds = event.currentTarget.dataset
    const name = ds.name
    const next = !(ds.stock === true || ds.stock === "true")
    ui.haptic()
    this.patchPersonal(name, { stock: next })
    try {
      await api.post("/shopping-list/inventory", { item_name: name, in_stock: next })
    } catch (error) {
      this.patchPersonal(name, { stock: !next })
      ui.toast(error.message || "库存状态更新失败")
    }
  },

  copyList() {
    const lines = []
    if (this.data.scope === "family") {
      this.data.familyCats.forEach((cat) => {
        const pending = cat.items.filter((it) => !it.checked)
        if (!pending.length) return
        lines.push(`【${cat.name}】`)
        pending.forEach((it) => lines.push(`□ ${it.name}${it.amount ? `  ${it.amount}` : ""}`))
      })
      const manual = this.data.familyManual.filter((it) => !it.checked)
      if (manual.length) {
        lines.push("【手动添加】")
        manual.forEach((it) => lines.push(`□ ${it.name}${it.amount ? `  ${it.amount}` : ""}`))
      }
    } else {
      this.data.personalCats.forEach((cat) => {
        const pending = cat.items.filter((it) => !it.checked && !it.stock)
        if (!pending.length) return
        lines.push(`【${cat.name}】`)
        pending.forEach((it) => lines.push(`□ ${it.name}${it.amount ? `  ${it.amount}` : ""}`))
      })
    }
    if (!lines.length) {
      ui.toast("没有待买的食材啦")
      return
    }
    const title = this.data.scope === "family" ? `🛒 ${this.data.familyName}买菜清单` : "🛒 我的买菜清单"
    wx.setClipboardData({
      data: `${title}\n${lines.join("\n")}`,
      success: () => ui.toast("清单已复制，可以发给家人"),
    })
  },

  goTomorrow() { wx.navigateTo({ url: "/pages/tomorrow/tomorrow" }) },
  goDishes() { wx.switchTab({ url: "/pages/dishes/dishes" }) },
})
