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

const NUTRIENTS = [
  { key: "energy_kcal", label: "能量", unit: "kcal" }, { key: "protein_g", label: "蛋白质", unit: "g" },
  { key: "carbohydrate_g", label: "碳水化合物", unit: "g" }, { key: "fat_g", label: "脂肪", unit: "g" },
  { key: "fiber_g", label: "膳食纤维", unit: "g" }, { key: "sodium_mg", label: "钠", unit: "mg" },
]
const FOOD_STATES = [ { value: "as_sold", label: "包装出售状态" }, { value: "ready_to_eat", label: "即食" }, { value: "raw", label: "生" }, { value: "cooked", label: "熟" } ]
const CUISINES = ["家常", "川菜", "湘菜", "粤菜", "鲁菜", "苏菜", "浙菜", "闽菜", "徽菜", "东北菜", "西餐", "日料", "韩餐"]

function healthDate(offset = 0) { return new Date(Date.now() + (8 * 3600 + offset * 86400) * 1000).toISOString().slice(0, 10) }
function requestKey() { return `journal_${Date.now()}_${Math.random().toString(36).slice(2)}` }

function groupChips(values) {
  return (values || []).map((value) => (GROUP_MAP[value] ? GROUP_MAP[value].label : value))
}

require("../../utils/theme").page({
  data: {
    loading: true,
    view: "journal",
    viewIndex: 0,
    days: 7,
    today: healthDate(),
    planDate: healthDate(), planEnd: healthDate(6), planMeal: "dinner", actionBusy: false, reportDays: [], selectedDay: null, plans: [],
    formPortion: "", editId: null, formLinked: null, formEventKey: "",
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
    nutritionFoods: [], nutritionChoices: ["暂不记录营养"], nutritionIndex: 0, nutritionMetrics: [], comparisonMetrics: [], comparisonExpanded: "",
    formNutritionMode: "", formNutritionSnapshot: null, formNutritionFood: null, formNutritionAmount: "", formPortionKey: "", formPortionCount: "", formStandardPortion: null, formPortionSource: "measured",
    catalogOpen: false, catalogQuery: "", catalogState: "", catalogStateIndex: 0, catalogResults: [], catalogLoading: false, catalogError: false, catalogStates: ["", "as_sold", "ready_to_eat", "raw", "cooked"], catalogStateLabels: ["所有状态", "出售状态", "即食", "生", "熟"],
    recipeOpen: false, recipeId: null, recipeVersion: 1, recipeName: "", recipeSource: "", recipeYield: "", recipeConfirmed: false, recipeRows: [], recipeFoods: [], recipeChoices: [],
    labelOpen: false, labelId: null, labelVersion: 1, labelName: "", labelSource: "", labelUnit: "g", labelState: "as_sold", labelStateIndex: 0,
    labelEnergyUnit: "kcal", labelSaving: false, foodStates: FOOD_STATES, foodStateLabels: FOOD_STATES.map(x => x.label),
    labelFields: NUTRIENTS.map(x => ({ ...x, value: "" })),
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
    const identity = api.token()
    if (this._identity !== identity) {
      this._loadRequest = (this._loadRequest || 0) + 1
      this._identity = identity; this._entries = []; this._report7 = null; this._report30 = null; this._journalLoaded = false
      this.setData({ dayGroups: [], report: null, reportDays: [], selectedDay: null, plans: [], showForm: false, formDish: "", formNotes: "", formCuisine: "", formPortion: "", actionBusy: false, saving: false, nutritionFoods: [], nutritionChoices: ["暂不记录营养"], labelOpen: false, catalogOpen: false, catalogResults: [], recipeOpen: false, recipeFoods: [], recipeChoices: [], recipeRows: [], recipeName: "", recipeSource: "", labelSaving: false, formNutritionSnapshot: null, formNutritionMode: "clear" })
    }
    this.setData({ today: healthDate(), planEnd: healthDate(6) })
    this.load(true)
  },

  profileChanged() {
    this._report7 = null; this._report30 = null
    if (this.data.view === "report") this.loadReport(true)
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
    const identity = api.token()
    const requestId = this._loadRequest = (this._loadRequest || 0) + 1
    this.setData({ loadError: "" })
    if (this._journalLoaded && !force) { this.setData({ loading: false }); return }
    this.setData({ loading: !this.data.dayGroups.length })
    try {
      const from = fmt.dateKey(fmt.addDays(new Date(), -120))
      const list = await api.get("/food-journal", { from, to: healthDate() })
      if (requestId !== this._loadRequest || identity !== api.token()) return
      this.applyJournal(list || [])
      this._journalLoaded = true
    } catch (error) {
      if (requestId === this._loadRequest && identity === api.token()) this.setData({ loadError: error.message || "饮食记录暂时无法加载" })
    } finally {
      if (requestId === this._loadRequest && identity === api.token()) this.setData({ loading: false })
    }
  },

  applyJournal(list) {
    this._entries = list
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
        notes: entry.notes || "", portion: entry.portion || "份量未记录",
      })
    })
    this.setData({ dayGroups: order.map((key) => byDate[key]) })
  },

  async loadReport(force) {
    const identity = api.token()
    const requestId = this._loadRequest = (this._loadRequest || 0) + 1
    this.setData({ loadError: "" })
    const cacheKey = `_report${this.data.days}`
    if (this[cacheKey] && !force) { this.applyReport(this[cacheKey]); this.setData({ loading: false }); return }
    this.setData({ loading: true })
    try {
      const report = await api.get("/health-report", { days: this.data.days })
      if (requestId !== this._loadRequest || identity !== api.token()) return
      this[cacheKey] = report
      this.applyReport(report)
    } catch (error) {
      if (requestId === this._loadRequest && identity === api.token()) this.setData({ loadError: error.message || "报告暂时无法生成" })
    } finally {
      if (requestId === this._loadRequest && identity === api.token()) this.setData({ loading: false })
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
      report: { ...report, period, insights: report.insights || [], plan_actions: report.plan_actions || [] },
      reportDays: (report.days || []).map(d => ({ ...d, shortDate: d.date.slice(5), statusLabel: d.status === "complete" ? "已确认完整" : d.status === "partial" ? "待确认完整" : "未记录", evidence: (d.evidence || []).map(e => ({ ...e, key: e.source + e.id, mealLabel: (MEAL_MAP[e.meal_type] || {}).label, sourceLabel: e.source === "record" ? "菜谱用餐记录" : "饮食日记", duplicateId: (e.possible_duplicate_ids || [])[0] || 0 })) })),
      nutritionMetrics: report.nutrients || [],
      comparisonExpanded: "",
      comparisonMetrics: ((report.comparison || {}).nutrients || []).map(n => ({ ...n, deltaText: n.delta == null ? "" : (n.delta > 0 ? "+" : "") + n.delta, datePairs: (n.current_dates || []).map((date, i) => ({ current: date, previous: n.previous_dates[i] })) })),
      plans: (report.plans || []).map(p => ({ ...p, mealLabel: (MEAL_MAP[p.meal_type] || {}).label, statusLabel: p.status === "recorded" ? "已有菜谱用餐记录支持完成" : p.status === "unconfirmed" ? "日期已过，尚无实际记录" : "已计划，尚未记为吃过" })),
      cuisineBars, groupBars, hasGroups, recs,
    })
    this.selectReportDay(this._selectedDate || report.to)
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

  selectReportDay(date) {
    const day = this.data.reportDays.find(d => d.date === date) || this.data.reportDays[this.data.reportDays.length - 1]
    this._selectedDate = day && day.date
    this.setData({ selectedDay: day || null })
  },
  chooseDay(e) { this.selectReportDay(e.currentTarget.dataset.date) },
  recordDay() { const date = this.data.selectedDay.date; this.openForm(); this.setData({ view: "journal", viewIndex: 0, formDate: date }); this.loadJournal(true) },
  async reportAction(work, message) {
    if (this.data.actionBusy) return
    const identity = api.token()
    this.setData({ actionBusy: true })
    try { await work(); if (identity !== api.token()) return; this._report7 = null; this._report30 = null; await this.loadReport(true); if (identity === api.token()) ui.toast(message, "success") }
    catch (error) { if (identity === api.token()) ui.toast(error.message || "操作失败，请重试") }
    finally { if (identity === api.token()) this.setData({ actionBusy: false }) }
  },
  async confirmDay() {
    const day = this.data.selectedDay
    if (!day || this.data.actionBusy) return
    if (day.status !== "complete" && !await ui.confirm({ title: "确认当天已记完整？", content: "请确认吃过的食物、饮料和加餐已记录。没有吃的餐次不需要补填。" })) return
    this.reportAction(() => api.put(`/health/days/${day.date}/status`, { fingerprint: day.fingerprint, complete: day.status !== "complete" }), "记录状态已更新")
  },
  async linkDuplicate(e) {
    const id = Number(e.currentTarget.dataset.id), record = Number(e.currentTarget.dataset.record)
    if (!await ui.confirm({ title: "确认是同一次食用？", content: "关联后只计一个食物项，原始记录仍保留。" })) return
    this.reportAction(async () => { const entry = await api.get(`/food-journal/${id}`); await api.put(`/food-journal/${id}`, { ...entry, linked_record_id: record }); this._journalLoaded = false }, "已关联同一食物")
  },
  toggleComparison(e) { const code = e.currentTarget.dataset.code; this.setData({ comparisonExpanded: this.data.comparisonExpanded === code ? "" : code }) },
  onPlanDate(e) { this.setData({ planDate: e.detail.value }) },
  onPlanMeal(e) { this.setData({ planMeal: Number(e.detail.value) === 0 ? "lunch" : "dinner" }) },
  acceptPlan(e) { this.reportAction(() => api.post("/health/plans", { dish_id: Number(e.currentTarget.dataset.id), meal_date: this.data.planDate, meal_type: this.data.planMeal, period_days: this.data.days }), "已加入菜单和买菜清单") },
  cancelPlan(e) { this.reportAction(() => api.delete(`/health/plans/${e.currentTarget.dataset.id}`), "已撤销计划") },
  editEntry(e) {
    if (this.data.saving || this.data.labelSaving) return
    const entry = (this._entries || []).find(x => x.id === Number(e.currentTarget.dataset.id))
    if (!entry) return
    const copy = !!e.currentTarget.dataset.copy
    this.loadNutritionFoods()
    this._requestKey = requestKey()
    this.setData({ formNutritionSnapshot: copy ? null : entry.nutrition || null, formNutritionMode: copy ? "clear" : "keep", formNutritionFood: null, formNutritionAmount: "", formPortionKey: "", formPortionCount: "", formStandardPortion: null, nutritionIndex: 0, labelOpen: false, catalogOpen: false, catalogResults: [], recipeOpen: false, showForm: true, editId: copy ? null : entry.id, formDate: copy ? healthDate() : entry.meal_date, formMeal: entry.meal_type, formDish: entry.dish_name, formCuisine: entry.cuisine, formNotes: entry.notes, formPortion: entry.portion || "", formLinked: copy ? null : entry.linked_record_id, formEventKey: copy ? "" : entry.event_key || "", groupDefs: GROUP_DEFS.map(item => ({ ...item, on: (entry.food_groups || []).includes(item.value) })) })
  },
  onPortionInput(e) { this.setData({ formPortion: e.detail.value, formNutritionMode: "clear" }) },
  unlinkRecord() { this.setData({ formLinked: null }) },

  async loadNutritionFoods() {
    const identity = api.token()
    try {
      const foods = await api.get("/health/foods")
      if (identity !== api.token()) return
      this.setData({ recipeFoods: foods.filter(f => (f.source === "package_label" || f.source === "standard_food")), recipeChoices: ["请选择标签", ...foods.filter(f => (f.source === "package_label" || f.source === "standard_food")).map(f => `${f.name} · ${f.basis_unit} · v${f.version}`)], nutritionFoods: foods, nutritionChoices: ["暂不记录营养", ...foods.map(f => `${f.name} · 每100${f.basis_unit} · ${(FOOD_STATES.find(s => s.value === f.food_state) || {}).label}`)] })
    } catch (error) { if (identity === api.token()) ui.toast(error.message || "营养标签暂时无法加载，可继续普通记餐") }
  },
  selectNutritionFood(e) {
    const index = Number(e.detail.value), food = this.data.nutritionFoods[index - 1] || null
    this.setData({ nutritionIndex: index, formNutritionFood: food, formNutritionMode: food ? "replace" : "clear", formNutritionAmount: "", formPortionKey: "", formPortionCount: "", formStandardPortion: null, formPortionSource: food && food.recipe ? "estimated" : "measured" })
  },
  chooseStandardPortion(e) {
    const food = this.data.formNutritionFood
    if (!food) return
    const portion = (food.portions || []).find(p => p.key === e.currentTarget.dataset.key) || null
    this.setData({ formPortionKey: portion ? portion.key : "", formStandardPortion: portion, formPortionCount: "", formNutritionAmount: "", formPortionSource: portion ? "estimated" : "measured" })
  },
  onStandardPortionCount(e) { this.setData({ formPortionCount: e.detail.value }) },
  onNutritionAmount(e) { this.setData({ formNutritionAmount: e.detail.value }) },
  onPortionSource(e) { if (this.data.formStandardPortion || (this.data.formNutritionFood && this.data.formNutritionFood.recipe)) return; this.setData({ formPortionSource: Number(e.detail.value) === 0 ? "measured" : "estimated" }) },
  clearNutrition() { this.setData({ formNutritionMode: "clear", formNutritionFood: null, formNutritionAmount: "", formPortionKey: "", formPortionCount: "", formStandardPortion: null, nutritionIndex: 0 }) },
  openLabel(e) {
    if (this.data.labelSaving || this.data.recipeOpen || this.data.catalogOpen) return
    const food = e.currentTarget.dataset.edit ? this.data.formNutritionFood : null
    this.setData({ labelOpen: true, labelId: food ? food.id : null, labelVersion: food ? food.version : 1,
      labelName: food ? food.name : "", labelSource: food ? food.source_reference : "", labelUnit: food ? food.basis_unit : "g",
      labelState: food ? food.food_state : "as_sold", labelStateIndex: food ? FOOD_STATES.findIndex(s => s.value === food.food_state) : 0, labelEnergyUnit: "kcal",
      labelFields: NUTRIENTS.map(f => ({ ...f, value: food && food.nutrients[f.key] != null ? String(food.nutrients[f.key]) : "" })) })
  },
  closeLabel() { if (!this.data.labelSaving) this.setData({ labelOpen: false }) },
  onLabelName(e) { this.setData({ labelName: e.detail.value }) },
  onLabelSource(e) { this.setData({ labelSource: e.detail.value }) },
  onLabelUnit(e) { this.setData({ labelUnit: Number(e.detail.value) === 0 ? "g" : "ml" }) },
  onLabelState(e) { const index = Number(e.detail.value); this.setData({ labelStateIndex: index, labelState: FOOD_STATES[index].value }) },
  onLabelEnergyUnit(e) { this.setData({ labelEnergyUnit: Number(e.detail.value) === 0 ? "kcal" : "kJ", labelFields: this.data.labelFields.map(f => f.key === "energy_kcal" ? { ...f, value: "" } : f) }) },
  onLabelNutrient(e) { this.setData({ labelFields: this.data.labelFields.map(f => f.key === e.currentTarget.dataset.key ? { ...f, value: e.detail.value } : f) }) },
  async deleteLabel() {
    const id = this.data.labelId, identity = api.token()
    if (!id || this.data.labelSaving || !await ui.confirm({ title: "删除标签？", content: "已保存的摄入快照仍保留。" }) || identity !== api.token()) return
    this.setData({ labelSaving: true })
    try { await api.delete(`/health/foods/${id}`); if (identity !== api.token()) return; await this.loadNutritionFoods(); if (identity !== api.token()) return; this.clearNutrition(); this.setData({ labelOpen: false }); ui.toast("标签已删除") }
    catch (error) { if (identity === api.token()) ui.toast(error.message || "删除失败") }
    finally { if (identity === api.token()) this.setData({ labelSaving: false }) }
  },
  async saveLabel() {
    if (this.data.labelSaving) return
    const nutrients = {}
    for (const f of this.data.labelFields) {
      nutrients[f.key] = f.value === "" ? null : Number(f.value)
      if (nutrients[f.key] !== null && !Number.isFinite(nutrients[f.key])) { ui.toast("请输入有效数值"); return }
    }
    if (this.data.labelEnergyUnit === "kJ" && nutrients.energy_kcal !== null) nutrients.energy_kcal /= 4.184
    const identity = api.token()
    this.setData({ labelSaving: true })
    try {
      const id = this.data.labelId
      const food = await api[id ? "put" : "post"](id ? `/health/foods/${id}` : "/health/foods", { name: this.data.labelName, source_reference: this.data.labelSource, basis_unit: this.data.labelUnit, food_state: this.data.labelState, nutrients, version: this.data.labelVersion })
      if (identity !== api.token()) return
      await this.loadNutritionFoods()
      if (identity !== api.token()) return
      this.setData({ labelOpen: false, formNutritionFood: food, formNutritionMode: "replace", formNutritionAmount: "", formPortionKey: "", formPortionCount: "", formStandardPortion: null, formPortionSource: "measured", nutritionIndex: this.data.nutritionFoods.findIndex(f => f.id === food.id) + 1 })
      ui.toast("标签已保存，请填写食用量", "success")
    } catch (error) { if (identity === api.token()) ui.toast(error.message || "保存标签失败") }
    finally { if (identity === api.token()) this.setData({ labelSaving: false }) }
  },

  openCatalog() {
    if (this.data.labelSaving || this.data.labelOpen || this.data.recipeOpen) return
    this.setData({ catalogOpen: true, catalogQuery: "", catalogState: "", catalogStateIndex: 0, catalogResults: [], catalogError: false })
    this.searchCatalog()
  },
  closeCatalog() { if (!this.data.labelSaving) { this._catalogRequest = (this._catalogRequest || 0) + 1; this.setData({ catalogOpen: false }) } },
  onCatalogQuery(e) { this.setData({ catalogQuery: e.detail.value }) },
  onCatalogState(e) { const index = Number(e.detail.value); this.setData({ catalogStateIndex: index, catalogState: this.data.catalogStates[index] }); this.searchCatalog() },
  async searchCatalog() {
    const identity = api.token(), request = this._catalogRequest = (this._catalogRequest || 0) + 1
    this.setData({ catalogLoading: true, catalogError: false })
    try {
      const rows = await api.get(`/health/catalog?q=${encodeURIComponent(this.data.catalogQuery.trim())}&state=${encodeURIComponent(this.data.catalogState)}`)
      if (identity !== api.token() || request !== this._catalogRequest || !this.data.catalogOpen) return
      this.setData({ catalogResults: rows.map(row => ({ ...row, stateLabel: (FOOD_STATES.find(s => s.value === row.food_state) || {}).label })) })
    } catch (error) { if (identity === api.token() && request === this._catalogRequest) this.setData({ catalogError: true }) }
    finally { if (identity === api.token() && request === this._catalogRequest) this.setData({ catalogLoading: false }) }
  },
  async adoptCatalog(e) {
    if (this.data.labelSaving) return
    const identity = api.token(), id = Number(e.currentTarget.dataset.id)
    this.setData({ labelSaving: true })
    try {
      const food = await api.post(`/health/catalog/${id}/adopt`)
      if (identity !== api.token()) return
      await this.loadNutritionFoods()
      if (identity !== api.token()) return
      this.setData({ catalogOpen: false, formNutritionFood: food, formNutritionMode: "replace", formNutritionAmount: "", formPortionKey: "", formPortionCount: "", formStandardPortion: null, formPortionSource: "measured", nutritionIndex: this.data.nutritionFoods.findIndex(f => f.id === food.id) + 1 })
      ui.toast("已添加，请填写可食部分食用量")
    } catch (error) { if (identity === api.token()) ui.toast(error.message || "添加失败") }
    finally { if (identity === api.token()) this.setData({ labelSaving: false }) }
  },
  deleteCatalogFood() { const food = this.data.formNutritionFood; if (!food || !food.catalog) return; this.setData({ labelId: food.id }); return this.deleteLabel() },

  openRecipe(e) {
    if (this.data.labelSaving || this.data.labelOpen || this.data.catalogOpen) return
    const food = e.currentTarget.dataset.edit ? this.data.formNutritionFood : null
    this.setData({ recipeOpen: true, recipeId: food ? food.id : null, recipeVersion: food ? food.version : 1, recipeName: food ? food.name : "", recipeSource: food ? food.source_reference : "", recipeYield: food ? String(food.recipe.yield_g) : "", recipeConfirmed: false,
      recipeRows: food ? food.recipe.ingredients.map((i, row) => { const index = this.data.recipeFoods.findIndex(f => f.id === i.food_id) + 1; return { rowKey: String(row), index, amount: String(i.amount), unit: index ? this.data.recipeFoods[index - 1].basis_unit : "" } }) : [{ rowKey: "0", index: 0, amount: "", unit: "" }, { rowKey: "1", index: 0, amount: "", unit: "" }] })
  },
  closeRecipe() { if (!this.data.labelSaving) this.setData({ recipeOpen: false }) },
  onRecipeField(e) { const key = e.currentTarget.dataset.key; if (["recipeName", "recipeSource", "recipeYield"].includes(key) && !this.data.labelSaving) this.setData({ [key]: e.detail.value }) },
  onRecipeConfirm(e) { if (!this.data.labelSaving) this.setData({ recipeConfirmed: !!e.detail.value.length }) },
  onRecipeFood(e) { if (this.data.labelSaving) return; const index = Number(e.detail.value), row = Number(e.currentTarget.dataset.row), food = this.data.recipeFoods[index - 1]; this.setData({ recipeRows: this.data.recipeRows.map((r, i) => i === row ? { ...r, index, amount: "", unit: food ? food.basis_unit : "" } : r) }) },
  onRecipeAmount(e) { if (this.data.labelSaving) return; const row = Number(e.currentTarget.dataset.row); this.setData({ recipeRows: this.data.recipeRows.map((r, i) => i === row ? { ...r, amount: e.detail.value } : r) }) },
  addRecipeRow() { if (!this.data.labelSaving && this.data.recipeRows.length < 20) this.setData({ recipeRows: [...this.data.recipeRows, { rowKey: requestKey(), index: 0, amount: "", unit: "" }] }) },
  removeRecipeRow(e) { if (!this.data.labelSaving && this.data.recipeRows.length > 2) this.setData({ recipeRows: this.data.recipeRows.filter((_, i) => i !== Number(e.currentTarget.dataset.row)) }) },
  async saveRecipe() {
    if (this.data.labelSaving || !this.data.recipeConfirmed) return
    const ingredients = this.data.recipeRows.map(r => { const food = this.data.recipeFoods[r.index - 1]; return { food_id: food ? food.id : 0, food_version: food ? food.version : 0, amount: Number(r.amount), unit: food ? food.basis_unit : "", food_state: food ? food.food_state : "" } })
    if (ingredients.some(i => !i.food_id || !Number.isFinite(i.amount) || i.amount <= 0) || !Number.isFinite(Number(this.data.recipeYield)) || Number(this.data.recipeYield) <= 0) { ui.toast("请选择原料并填写有效用量和成品重量"); return }
    const identity = api.token(), id = this.data.recipeId
    this.setData({ labelSaving: true })
    try {
      const food = await api[id ? "put" : "post"](id ? `/health/recipes/${id}` : "/health/recipes", { name: this.data.recipeName, source_reference: this.data.recipeSource, version: this.data.recipeVersion, yield_g: Number(this.data.recipeYield), method: "unheated_all_retained", ingredients })
      if (identity !== api.token()) return
      await this.loadNutritionFoods()
      if (identity !== api.token()) return
      this.setData({ recipeOpen: false, formNutritionFood: food, formNutritionMode: "replace", formNutritionAmount: "", formPortionKey: "", formPortionCount: "", formStandardPortion: null, formPortionSource: "estimated", nutritionIndex: this.data.nutritionFoods.findIndex(f => f.id === food.id) + 1 })
      ui.toast("配方已保存，请填写实际食用量")
    } catch (error) { if (identity === api.token()) ui.toast(error.message || "保存配方失败") }
    finally { if (identity === api.token()) this.setData({ labelSaving: false }) }
  },
  async deleteRecipe() {
    const id = this.data.recipeId, identity = api.token()
    if (!id || this.data.labelSaving || !await ui.confirm({ title: "删除配方？", content: "历史摄入快照仍保留。" }) || identity !== api.token()) return
    this.setData({ labelSaving: true })
    try { await api.delete(`/health/foods/${id}`); if (identity !== api.token()) return; await this.loadNutritionFoods(); if (identity !== api.token()) return; this.clearNutrition(); this.setData({ recipeOpen: false }); ui.toast("配方已删除") }
    catch (error) { if (identity === api.token()) ui.toast(error.message || "删除失败") }
    finally { if (identity === api.token()) this.setData({ labelSaving: false }) }
  },

  // ---------- 表单 ----------
  openForm() {
    if (this.data.saving || this.data.labelSaving) return
    ui.haptic()
    this._requestKey = requestKey()
    this.loadNutritionFoods()
    this.setData({
      editId: null, formPortion: "", formLinked: null, formEventKey: "", formNutritionMode: "", formNutritionSnapshot: null, formNutritionFood: null, formNutritionAmount: "", formPortionKey: "", formPortionCount: "", formStandardPortion: null, nutritionIndex: 0, labelOpen: false, catalogOpen: false, catalogResults: [], recipeOpen: false,
      showForm: true, formDate: healthDate(), formMeal: "lunch", formDish: "", formCuisine: "", formNotes: "",
      groupDefs: GROUP_DEFS.map((item) => ({ ...item, on: false })),
    })
  },
  closeForm() { if (!this.data.saving && !this.data.labelSaving) this.setData({ showForm: false }) },
  onDateChange(event) { this.setData({ formDate: event.detail.value }) },
  chooseMeal(event) { ui.haptic(); this.setData({ formMeal: event.currentTarget.dataset.value }) },
  onDishInput(event) { this.setData({ formDish: event.detail.value, formNutritionMode: "clear" }) },
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
    if (!dish || this.data.saving || this.data.labelSaving || this.data.labelOpen || this.data.recipeOpen || this.data.catalogOpen) return
    const identity = api.token()
    this.setData({ saving: true })
    try {
      await api[this.data.editId ? "put" : "post"](this.data.editId ? `/food-journal/${this.data.editId}` : "/food-journal", {
        meal_date: this.data.formDate,
        meal_type: this.data.formMeal,
        dish_name: dish,
        cuisine: this.data.formCuisine.trim(),
        food_groups: this.data.groupDefs.filter((item) => item.on).map((item) => item.value),
        notes: this.data.formNotes.trim(),
        nutrition_mode: this.data.formNutritionMode,
        nutrition_food_id: this.data.formNutritionFood ? this.data.formNutritionFood.id : 0,
        nutrition_portion_key: this.data.formPortionKey || undefined,
        nutrition_portion_count: this.data.formPortionKey && this.data.formPortionCount !== "" ? Number(this.data.formPortionCount) : undefined,
        nutrition_amount: this.data.formNutritionAmount === "" ? null : Number(this.data.formNutritionAmount),
        nutrition_unit: this.data.formNutritionFood ? this.data.formNutritionFood.basis_unit : "",
        food_state: this.data.formNutritionFood ? this.data.formNutritionFood.food_state : "",
        portion_source: this.data.formPortionSource,
        portion: this.data.formPortion.trim(), linked_record_id: this.data.formLinked, event_key: this.data.formEventKey, request_key: this._requestKey || (this._requestKey = requestKey()),
      })
      if (identity !== api.token()) return
      this.setData({ showForm: false })
      ui.toast("已记下这餐", "success")
      this._journalLoaded = false
      this._report7 = null
      this._report30 = null
      await this.load(true)
    } catch (error) {
      if (identity === api.token()) ui.toast(error.message || "保存失败")
    } finally {
      if (identity === api.token()) this.setData({ saving: false })
    }
  },

  async removeEntry(event) {
    const identity = api.token()
    const id = Number(event.currentTarget.dataset.id)
    const ok = await ui.confirm({ title: "删除这条记录？", content: "删除后健康报告会同步更新。", confirmText: "删除", danger: true })
    if (!ok || identity !== api.token()) return
    try {
      await api.delete(`/food-journal/${id}`)
      if (identity !== api.token()) return
      this._report7 = null
      this._report30 = null
      await this.loadJournal(true)
      ui.toast("已删除")
    } catch (error) {
      if (identity === api.token()) ui.toast(error.message || "删除失败")
    }
  },

  goHistory() { wx.switchTab({ url: "/pages/history/history" }) },
  goPlan() { wx.navigateTo({ url: "/pages/plan/plan" }) },
  goDish(event) { wx.navigateTo({ url: `/pages/dish/dish?id=${event.currentTarget.dataset.id}` }) },
  goChat() { wx.navigateTo({ url: "/pages/chat/chat" }) },
})
