const api = require("../../utils/api")
const session = require("../../utils/session")
const ui = require("../../utils/ui")
const media = require("../../utils/media")
const dishUtil = require("../../utils/dish")
const fmt = require("../../utils/format")

const MAX_PER_MEAL = 5
const DEFAULT_TARGETS = { lunch: 2, dinner: 2 }
const PROFILES = [
  { key: "balanced", label: "均衡", hint: "正常搭配", icon: "sparkles" },
  { key: "quick", label: "快手", hint: "省时间", icon: "zap" },
  { key: "light", label: "清淡", hint: "少负担", icon: "leaf" },
  { key: "spicy", label: "想吃辣", hint: "重口味", icon: "flame" },
  { key: "favorite", label: "收藏", hint: "优先常吃", icon: "heart" },
]
const MEAL_META = {
  lunch: { label: "明天午餐", short: "午餐", helper: "中午吃得稳一点", icon: "chef-hat", tone: "primary" },
  dinner: { label: "明天晚餐", short: "晚餐", helper: "晚上吃得舒服一点", icon: "moon", tone: "mint" },
}

function tomorrow() {
  const d = fmt.addDays(new Date(), 1)
  const key = fmt.dateKey(d)
  return { key, label: `${key.slice(5)} ${fmt.WEEKDAYS[d.getDay()]}` }
}

function planCard(dish) {
  const card = dishUtil.toCard(dish)
  card.diffLabel = dish.difficulty === "hard" ? "费点劲" : card.diffLabel
  card.timeText = card.cookTime > 0 ? `${card.cookTime}分钟` : "时间未填"
  return card
}

function recordCard(record) {
  return {
    id: record.dish_id,
    name: record.dish_name,
    category: "",
    cookTime: 0,
    timeText: "时间未填",
    diffLabel: "",
    diffTone: "mint",
    cover: media.assetUrl(record.dish_image_url),
    emoji: record.dish_emoji || "🍽",
  }
}

function minutes(list) {
  return list.reduce((sum, dish) => sum + Math.max(0, dish.cookTime || 0), 0)
}

function mealRecords(records, meal) {
  return records.filter((record) => record.meal_type === meal).sort((a, b) => a.id - b.id)
}

function recordsKey(records) {
  return ["lunch", "dinner"].map((meal) => `${meal}:${mealRecords(records, meal).map((record) => record.dish_id).join(",")}`).join("|")
}

function searchText(dish) {
  const names = (list) => media.asArray(list).map((item) => (typeof item === "string" ? item : item && item.name) || "").join(" ")
  return [dish.name, dish.category, dish.taste, names(dish.ingredients), names(dish.seasonings), media.asArray(dish.tags).join(" ")].join(" ").toLowerCase()
}

Page({
  data: {
    dateKey: "",
    dateLabel: "",
    loading: true,
    picking: false,
    saving: false,
    profiles: PROFILES,
    profile: "balanced",
    meals: [],
    targets: DEFAULT_TARGETS,
    totalCount: 0,
    lunchCount: 0,
    dinnerCount: 0,
    lunchMinutes: 0,
    dinnerMinutes: 0,
    hasSaved: false,
    hasChanges: false,

    pickerOpen: false,
    pickerMeal: "lunch",
    pickerTitle: "",
    pickerLoading: false,
    pickerCategories: ["全部"],
    pickerCategory: "全部",
    pickerSearch: "",
    pickerList: [],
  },

  onLoad() {
    const day = tomorrow()
    this._plan = { lunch: [], dinner: [] }
    this._original = []
    this.setData({ dateKey: day.key, dateLabel: day.label })
    if (!session.requireLogin("/pages/tomorrow/tomorrow")) return
    this.init()
  },

  async init() {
    try {
      const result = await api.get("/records", { date_from: this.data.dateKey, date_to: this.data.dateKey, pageSize: 100 })
      const records = result.items || []
      this._original = records
      if (records.length) {
        this._plan.lunch = mealRecords(records, "lunch").map(recordCard)
        this._plan.dinner = mealRecords(records, "dinner").map(recordCard)
        this.setData({
          targets: {
            lunch: Math.min(MAX_PER_MEAL, Math.max(this._plan.lunch.length, DEFAULT_TARGETS.lunch)),
            dinner: Math.min(MAX_PER_MEAL, Math.max(this._plan.dinner.length, DEFAULT_TARGETS.dinner)),
          },
        })
        this.render()
      } else {
        await this.generateAll()
      }
    } catch (error) {
      ui.toast(error.message || "加载失败")
      this.render()
    } finally {
      this.setData({ loading: false })
    }
  },

  // ---------- 渲染 ----------

  render() {
    const { lunch, dinner } = this._plan
    const targets = this.data.targets
    const meals = ["lunch", "dinner"].map((key) => {
      const list = this._plan[key]
      const meta = MEAL_META[key]
      const missing = Math.max(0, targets[key] - list.length)
      return { key, ...meta, dishes: list, target: targets[key], missing, minutes: minutes(list) }
    })
    const currentKey = `lunch:${lunch.map((dish) => dish.id).join(",")}|dinner:${dinner.map((dish) => dish.id).join(",")}`
    const hasSaved = this._original.length > 0
    this.setData({
      meals,
      totalCount: lunch.length + dinner.length,
      lunchCount: lunch.length,
      dinnerCount: dinner.length,
      lunchMinutes: minutes(lunch),
      dinnerMinutes: minutes(dinner),
      hasSaved,
      hasChanges: currentKey !== recordsKey(this._original),
    })
    if (this.data.pickerOpen) this.renderPicker()
  },

  // ---------- 推荐 ----------

  async request(meal, count, excludeIds) {
    if (count <= 0) return []
    const result = await api.post("/pick/tomorrow", { meal_type: meal, profile: this.data.profile, count, exclude_ids: excludeIds || [] })
    return (result.dishes || []).map(planCard)
  },

  async generateAll() {
    if (this.data.picking) return
    this.setData({ picking: true })
    try {
      const { targets } = this.data
      const lunch = targets.lunch > 0 ? await this.request("lunch", targets.lunch) : []
      const dinner = targets.dinner > 0 ? await this.request("dinner", targets.dinner, lunch.map((dish) => dish.id)) : []
      if (!lunch.length && !dinner.length) {
        ui.toast("暂无可推荐的菜品")
        return
      }
      this._plan = { lunch, dinner }
      this.render()
    } catch (_) {
      ui.toast("暂无可推荐的菜品")
    } finally {
      this.setData({ picking: false })
    }
  },

  regenerate() {
    ui.haptic("medium")
    this.generateAll()
  },

  chooseProfile(event) {
    const profile = event.currentTarget.dataset.key
    if (profile === this.data.profile && !this.data.picking) { this.generateAll(); return }
    ui.haptic()
    this.setData({ profile }, () => this.generateAll())
  },

  other(meal) {
    return this._plan[meal === "lunch" ? "dinner" : "lunch"]
  },

  async fillMeal(event) {
    const meal = event.currentTarget.dataset.meal
    const current = this._plan[meal]
    const needed = Math.max(0, this.data.targets[meal] - current.length)
    if (!needed || this.data.picking) return
    this.setData({ picking: true })
    try {
      const exclude = this.other(meal).concat(current).map((dish) => dish.id)
      const picks = await this.request(meal, needed, exclude)
      if (!picks.length) { ui.toast("没有更多合适的菜了"); return }
      const seen = {}
      this._plan[meal] = current.concat(picks).filter((dish) => (seen[dish.id] ? false : (seen[dish.id] = true))).slice(0, MAX_PER_MEAL)
      this.render()
    } catch (_) {
      ui.toast("没有更多合适的菜了")
    } finally {
      this.setData({ picking: false })
    }
  },

  async replaceMeal(event) {
    const meal = event.currentTarget.dataset.meal
    if (this.data.picking) return
    const count = Math.max(1, this.data.targets[meal] || this._plan[meal].length || 1)
    this.setData({ picking: true })
    try {
      const picks = await this.request(meal, count, this.other(meal).map((dish) => dish.id))
      if (!picks.length) { ui.toast("暂无可推荐的菜品"); return }
      this._plan[meal] = picks
      this.render()
    } catch (_) {
      ui.toast("暂无可推荐的菜品")
    } finally {
      this.setData({ picking: false })
    }
  },

  async swapDish(event) {
    const meal = event.currentTarget.dataset.meal
    const id = Number(event.currentTarget.dataset.id)
    if (this.data.picking) return
    const current = this._plan[meal]
    const exclude = current.map((dish) => dish.id).concat(this.other(meal).map((dish) => dish.id))
    this.setData({ picking: true })
    ui.haptic()
    try {
      const [pick] = await this.request(meal, 1, exclude)
      if (!pick) { ui.toast("没有可替换的菜了"); return }
      this._plan[meal] = current.map((dish) => (dish.id === id ? pick : dish))
      this.render()
    } catch (_) {
      ui.toast("没有可替换的菜了")
    } finally {
      this.setData({ picking: false })
    }
  },

  removeDish(event) {
    const meal = event.currentTarget.dataset.meal
    const id = Number(event.currentTarget.dataset.id)
    ui.haptic()
    this._plan[meal] = this._plan[meal].filter((dish) => dish.id !== id)
    this.render()
  },

  changeTarget(event) {
    const meal = event.currentTarget.dataset.meal
    const step = Number(event.currentTarget.dataset.step)
    const next = Math.max(0, Math.min(MAX_PER_MEAL, this.data.targets[meal] + step))
    if (next === this.data.targets[meal]) return
    ui.haptic()
    if (this._plan[meal].length > next) this._plan[meal] = this._plan[meal].slice(0, next)
    this.setData({ [`targets.${meal}`]: next }, () => this.render())
  },

  openDish(event) {
    wx.navigateTo({ url: `/pages/dish/dish?id=${event.currentTarget.dataset.id}` })
  },

  goPlan() {
    wx.navigateTo({ url: "/pages/plan/plan?tab=shopping" })
  },

  // ---------- 手动选菜 ----------

  async openPicker(event) {
    const meal = event.currentTarget.dataset.meal
    this.setData({ pickerOpen: true, pickerMeal: meal, pickerTitle: `挑选${MEAL_META[meal].short}`, pickerSearch: "", pickerCategory: "全部" })
    if (this._pickerCache && this._pickerCache[meal]) {
      this.renderPicker()
      return
    }
    this.setData({ pickerLoading: true, pickerList: [] })
    try {
      const result = await api.get("/dishes", { enabled: "true", meal_type: meal, pageSize: 100, sort: "sort_order", order: "asc" })
      const items = (result.items || []).filter((dish) => !dish.meal_type || dish.meal_type === "all" || dish.meal_type === meal)
      this._pickerCache = this._pickerCache || {}
      this._pickerCache[meal] = items.map((dish) => ({ card: planCard(dish), text: searchText(dish), category: dish.category || "" }))
      const categories = ["全部"]
      items.forEach((dish) => { if (dish.category && categories.indexOf(dish.category) < 0) categories.push(dish.category) })
      this._pickerCategories = this._pickerCategories || {}
      this._pickerCategories[meal] = categories
      this.renderPicker()
    } catch (error) {
      ui.toast(error.message || "菜品加载失败")
    } finally {
      this.setData({ pickerLoading: false })
    }
  },

  closePicker() { this.setData({ pickerOpen: false }) },

  renderPicker() {
    const meal = this.data.pickerMeal
    const entries = (this._pickerCache && this._pickerCache[meal]) || []
    const query = this.data.pickerSearch.trim().toLowerCase()
    const selected = {}
    this._plan.lunch.forEach((dish) => { selected[dish.id] = "lunch" })
    this._plan.dinner.forEach((dish) => { selected[dish.id] = "dinner" })
    const list = entries
      .filter((entry) => (this.data.pickerCategory === "全部" || entry.category === this.data.pickerCategory) && (!query || entry.text.indexOf(query) >= 0))
      .map((entry) => ({ ...entry.card, selectedMeal: selected[entry.card.id] || "", selectedLabel: selected[entry.card.id] ? MEAL_META[selected[entry.card.id]].short : "" }))
    this.setData({ pickerList: list, pickerCategories: (this._pickerCategories && this._pickerCategories[meal]) || ["全部"] })
  },

  onPickerSearch(event) {
    this.setData({ pickerSearch: event.detail.value }, () => this.renderPicker())
  },

  choosePickerCategory(event) {
    this.setData({ pickerCategory: event.currentTarget.dataset.name }, () => this.renderPicker())
  },

  pickerAdd(event) {
    const id = Number(event.currentTarget.dataset.id)
    const meal = this.data.pickerMeal
    const entry = ((this._pickerCache && this._pickerCache[meal]) || []).find((item) => item.card.id === id)
    if (!entry) return
    if (this._plan[meal].length >= MAX_PER_MEAL) { ui.toast(`每餐最多 ${MAX_PER_MEAL} 道菜`); return }
    ui.haptic()
    this._plan[meal] = this._plan[meal].concat([entry.card])
    const target = Math.max(this.data.targets[meal], this._plan[meal].length)
    this.setData({ [`targets.${meal}`]: target }, () => this.render())
  },

  pickerRemove(event) {
    const id = Number(event.currentTarget.dataset.id)
    const meal = this.data.pickerMeal
    this._plan[meal] = this._plan[meal].filter((dish) => dish.id !== id)
    this.render()
  },

  // ---------- 保存 ----------

  async save() {
    const { lunch, dinner } = this._plan
    if (!lunch.length && !dinner.length) { ui.toast("至少选一道菜"); return }
    if (this.data.hasSaved && !this.data.hasChanges) { ui.toast("菜单没有变化"); return }
    if (this.data.saving) return
    const wasSaved = this.data.hasSaved
    this.setData({ saving: true })
    try {
      await Promise.all(this._original.map((record) => api.delete(`/records/${record.id}`).catch(() => null)))
      const records = []
      lunch.forEach((dish) => records.push({ dish_id: dish.id, dish_name: dish.name, meal_type: "lunch", meal_date: this.data.dateKey }))
      dinner.forEach((dish) => records.push({ dish_id: dish.id, dish_name: dish.name, meal_type: "dinner", meal_date: this.data.dateKey }))
      const result = await api.post("/records/batch", { records })
      this._original = result.created || []
      this.render()
      const confetti = this.selectComponent("#confetti")
      if (confetti) confetti.fire()
      ui.haptic("medium")
      ui.toast(wasSaved ? "明天菜单已更新" : "明天菜单已保存", "success")
    } catch (error) {
      ui.toast(error.message || "保存失败，请重试")
    } finally {
      this.setData({ saving: false })
    }
  },
})
