const api = require("../../utils/api")
const session = require("../../utils/session")
const ui = require("../../utils/ui")
const fmt = require("../../utils/format")
const media = require("../../utils/media")
const dishUtil = require("../../utils/dish")

const MEALS = [
  { value: "breakfast", label: "早餐", emoji: "🌅" },
  { value: "lunch", label: "午餐", emoji: "🍳" },
  { value: "dinner", label: "晚餐", emoji: "🍲" },
  { value: "snack", label: "加餐", emoji: "🍎" },
]
const MEAL_MAP = MEALS.reduce((map, item) => { map[item.value] = item; return map }, {})

const GROUP_DEFS = [
  { value: "vegetable", label: "蔬菜", icon: "leaf", tone: "mint", color: "mint-ink" },
  { value: "fruit", label: "水果", icon: "carrot", tone: "pink", color: "pink-ink" },
  { value: "protein", label: "蛋白质", icon: "fish", tone: "primary", color: "primary" },
  { value: "whole_grain", label: "全谷物", icon: "wheat", tone: "yellow", color: "yellow-dark" },
  { value: "dairy", label: "奶类", icon: "egg", tone: "purple", color: "purple" },
]
const GROUP_MAP = GROUP_DEFS.reduce((map, item) => { map[item.value] = item; return map }, {})

const CUISINES = ["家常", "川菜", "湘菜", "粤菜", "鲁菜", "苏菜", "浙菜", "闽菜", "徽菜", "东北菜", "西餐", "日料", "韩餐"]

function groupChips(values) {
  return (values || []).map((value) => (GROUP_MAP[value] ? GROUP_MAP[value].label : value))
}

Page({
  data: {
    loading: true,
    view: "journal",
    viewIndex: 0,
    days: 7,
    today: fmt.dateKey(),
    meals: MEALS,
    cuisines: CUISINES,
    // journal
    dayGroups: [],
    // report
    report: null,
    cuisineBars: [],
    groupBars: [],
    hasGroups: false,
    recs: [],
    // form
    showForm: false,
    saving: false,
    formDate: fmt.dateKey(),
    formMeal: "lunch",
    formDish: "",
    formCuisine: "",
    formNotes: "",
    groupDefs: GROUP_DEFS.map((item) => ({ ...item, on: false })),
    focusDish: false,
    focusCuisine: false,
    focusNotes: false,
  },

  onShow() {
    if (!session.requireLogin("/pages/diary/diary")) return
    this.load()
  },

  async onPullDownRefresh() {
    await this.load(true)
    wx.stopPullDownRefresh()
  },

  async load(force) {
    if (this.data.view === "journal") await this.loadJournal(force)
    else await this.loadReport(force)
  },

  async loadJournal(force) {
    if (this._journalLoaded && !force) { this.setData({ loading: false }); return }
    this.setData({ loading: !this.data.dayGroups.length })
    try {
      const from = fmt.dateKey(fmt.addDays(new Date(), -120))
      const list = await api.get("/food-journal", { from, to: fmt.dateKey() })
      this.applyJournal(list || [])
      this._journalLoaded = true
    } catch (error) {
      ui.toast(error.message || "饮食记录暂时无法加载")
    } finally {
      this.setData({ loading: false })
    }
  },

  applyJournal(list) {
    const byDate = {}
    const order = []
    list.forEach((entry) => {
      const key = String(entry.meal_date || "").slice(0, 10)
      if (!key) return
      if (!byDate[key]) {
        byDate[key] = { date: key, label: fmt.relativeDate(key), weekday: fmt.weekday(key), items: [] }
        order.push(key)
      }
      const meal = MEAL_MAP[entry.meal_type] || { label: entry.meal_type || "用餐", emoji: "🍽" }
      byDate[key].items.push({
        id: entry.id,
        mealLabel: meal.label,
        mealEmoji: meal.emoji,
        dishName: entry.dish_name,
        cuisine: entry.cuisine || "",
        groups: groupChips(entry.food_groups),
        notes: entry.notes || "",
      })
    })
    this.setData({ dayGroups: order.map((key) => byDate[key]) })
  },

  async loadReport(force) {
    const cacheKey = `_report${this.data.days}`
    if (this[cacheKey] && !force) { this.applyReport(this[cacheKey]); this.setData({ loading: false }); return }
    this.setData({ loading: true })
    try {
      const report = await api.get("/health-report", { days: this.data.days })
      this[cacheKey] = report
      this.applyReport(report)
    } catch (error) {
      ui.toast(error.message || "报告暂时无法生成")
    } finally {
      this.setData({ loading: false })
    }
  },

  applyReport(report) {
    if (!report) { this.setData({ report: null }); return }
    const period = report.period_days || this.data.days
    const counts = report.cuisine_counts || []
    const cuisineMax = Math.max(1, ...counts.map((item) => item.count || 0))
    const cuisineBars = counts.slice(0, 8).map((item) => ({
      name: item.name, count: item.count, pct: Math.max(6, Math.round((item.count / cuisineMax) * 100)),
    }))
    const dayMap = report.food_group_days || {}
    let hasGroups = false
    const groupBars = GROUP_DEFS.map((def) => {
      const days = dayMap[def.value] || 0
      if (days > 0) hasGroups = true
      return { value: def.value, label: def.label, tone: def.tone, days, pct: Math.round((days / Math.max(1, period)) * 100) }
    })
    const recs = (report.recommendations || []).map((item) => ({
      id: item.dish.id,
      name: item.dish.name,
      reason: item.reason,
      cover: media.dishCover(item.dish),
      emoji: dishUtil.dishEmoji(item.dish),
    }))
    this.setData({
      report: { logged_days: report.logged_days, meal_count: report.meal_count, period, insights: report.insights || [], plan_actions: report.plan_actions || [] },
      cuisineBars, groupBars, hasGroups, recs,
    })
  },

  switchView(event) {
    const view = event.currentTarget.dataset.view
    if (view === this.data.view) return
    ui.haptic()
    this.setData({ view, viewIndex: view === "report" ? 1 : 0, loading: true }, () => this.load())
  },

  choosePeriod(event) {
    const days = Number(event.currentTarget.dataset.days)
    if (days === this.data.days) return
    ui.haptic()
    this.setData({ days }, () => this.loadReport())
  },

  // ---------- 表单 ----------
  openForm() {
    ui.haptic()
    this.setData({
      showForm: true, formDate: fmt.dateKey(), formMeal: "lunch", formDish: "", formCuisine: "", formNotes: "",
      groupDefs: GROUP_DEFS.map((item) => ({ ...item, on: false })),
    })
  },
  closeForm() { if (!this.data.saving) this.setData({ showForm: false }) },
  onDateChange(event) { this.setData({ formDate: event.detail.value }) },
  chooseMeal(event) { ui.haptic(); this.setData({ formMeal: event.currentTarget.dataset.value }) },
  onDishInput(event) { this.setData({ formDish: event.detail.value }) },
  onCuisineInput(event) { this.setData({ formCuisine: event.detail.value }) },
  chooseCuisine(event) {
    const value = event.currentTarget.dataset.value
    this.setData({ formCuisine: this.data.formCuisine === value ? "" : value })
  },
  onNotesInput(event) { this.setData({ formNotes: event.detail.value }) },
  toggleGroup(event) {
    const value = event.currentTarget.dataset.value
    this.setData({ groupDefs: this.data.groupDefs.map((item) => (item.value === value ? { ...item, on: !item.on } : item)) })
  },
  focusOn(event) { this.setData({ [`focus${event.currentTarget.dataset.k}`]: true }) },
  focusOff(event) { this.setData({ [`focus${event.currentTarget.dataset.k}`]: false }) },

  async saveEntry() {
    const dish = this.data.formDish.trim()
    if (!dish || this.data.saving) return
    this.setData({ saving: true })
    try {
      await api.post("/food-journal", {
        meal_date: this.data.formDate,
        meal_type: this.data.formMeal,
        dish_name: dish,
        cuisine: this.data.formCuisine.trim(),
        food_groups: this.data.groupDefs.filter((item) => item.on).map((item) => item.value),
        notes: this.data.formNotes.trim(),
      })
      this.setData({ showForm: false })
      ui.toast("已记下这餐", "success")
      this._journalLoaded = false
      this._report7 = null
      this._report30 = null
      await this.load(true)
    } catch (error) {
      ui.toast(error.message || "保存失败")
    } finally {
      this.setData({ saving: false })
    }
  },

  async removeEntry(event) {
    const id = Number(event.currentTarget.dataset.id)
    const ok = await ui.confirm({ title: "删除这条记录？", content: "删除后健康报告会同步更新。", confirmText: "删除", danger: true })
    if (!ok) return
    try {
      await api.delete(`/food-journal/${id}`)
      this._report7 = null
      this._report30 = null
      await this.loadJournal(true)
      ui.toast("已删除")
    } catch (error) {
      ui.toast(error.message || "删除失败")
    }
  },

  goHistory() { wx.switchTab({ url: "/pages/history/history" }) },
  goPlan() { wx.navigateTo({ url: "/pages/plan/plan" }) },
  goDish(event) { wx.navigateTo({ url: `/pages/dish/dish?id=${event.currentTarget.dataset.id}` }) },
  goChat() { wx.navigateTo({ url: "/pages/chat/chat" }) },
})
