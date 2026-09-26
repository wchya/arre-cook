const api = require("../../utils/api")
const session = require("../../utils/session")
const ui = require("../../utils/ui")
const media = require("../../utils/media")
const dishUtil = require("../../utils/dish")
const fmt = require("../../utils/format")

const DEFAULT_TASTES = ["辣", "麻辣", "香辣", "酸辣", "鲜辣", "酸", "甜", "鲜", "清淡", "咸鲜", "蒜香", "酱香"]
const MOODS = [
  { key: "", label: "随便", icon: "sparkles" },
  { key: "spicy", label: "想吃辣", icon: "flame" },
  { key: "tired", label: "省事点", icon: "zap" },
  { key: "healthy", label: "清爽点", icon: "leaf" },
]
const COOK_TIMES = [
  { value: 0, label: "不限" },
  { value: 20, label: "≤20 分钟" },
  { value: 35, label: "≤35 分钟" },
  { value: 60, label: "≤60 分钟" },
]
const MEALS = [
  { key: "", label: "不限餐次" },
  { key: "lunch", label: "🍳 午餐" },
  { key: "dinner", label: "🍲 晚餐" },
]

function pct(value) {
  return `${Math.round((Number(value) || 0) * 100)}%`
}

function weightBars(items) {
  const list = (items || []).slice(0, 5)
  const max = Math.max(0.01, ...list.map((item) => item.weight || 0))
  return list.map((item) => ({ name: item.name, pct: pct(item.weight), width: Math.max(6, ((item.weight || 0) / max) * 100) }))
}

function splitWords(text) {
  return String(text || "").split(/[,，、\s]+/).map((word) => word.trim()).filter(Boolean)
}

Page({
  data: {
    profile: null,
    profileLoading: true,
    tasteBars: [],
    categoryBars: [],
    profileTags: [],

    meals: MEALS,
    moods: MOODS,
    cookTimes: COOK_TIMES,
    tasteOptions: DEFAULT_TASTES.map((name) => ({ name, on: false })),
    meal: "",
    mood: "",
    maxCookTime: 0,
    includeText: "",
    excludeText: "",

    items: [],
    summary: "",
    picking: false,
    busyId: 0,
  },

  onLoad() {
    this._rejected = []
    if (!session.requireLogin("/pages/assistant/assistant")) return
    this.loadProfile()
    this.loadTastes()
  },

  async loadProfile() {
    try {
      const profile = await api.get("/profile", { days: 90 })
      const tags = []
      ;(profile.top_dishes || []).slice(0, 3).forEach((dish) => tags.push({ key: `d${dish.dish_id}`, tone: "primary", text: `🍽 常吃 ${dish.dish_name} ×${dish.count}` }))
      ;(profile.top_ingredients || []).slice(0, 3).forEach((item) => tags.push({ key: `i${item.name}`, tone: "mint", text: `🥬 ${item.name}` }))
      const moods = Object.entries(profile.home_mood_counts || {}).sort((a, b) => b[1] - a[1])
      if (moods.length) {
        const meta = dishUtil.HOME_MOOD_MAP[moods[0][0]]
        tags.push({ key: "mood", tone: "yellow", text: `😃 常见心情 ${meta ? meta.label : moods[0][0]}` })
      }
      if ((profile.disliked_dishes || []).length) tags.push({ key: "dislike", tone: "red", text: `🙅 避开 ${profile.disliked_dishes.length} 道踩雷菜` })
      this.setData({
        profile: {
          summary: profile.summary || "继续记录每一餐，画像会逐渐贴近你的口味。",
          windowDays: profile.window_days,
          spicy: pct(profile.spicy_ratio),
          records: profile.window_records || 0,
          distinct: profile.distinct_dishes || 0,
          avgTime: profile.avg_cook_time > 0 ? `${Math.round(profile.avg_cook_time)}m` : "-",
        },
        tasteBars: weightBars(profile.taste_weights),
        categoryBars: weightBars(profile.category_weights),
        profileTags: tags,
      })
    } catch (_) {
      this.setData({ profile: null })
    } finally {
      this.setData({ profileLoading: false })
    }
  },

  async loadTastes() {
    try {
      const settings = await api.get("/settings")
      const list = media.asArray(settings && settings.tastes).filter((item) => typeof item === "string" && item)
      if (list.length) this.setData({ tasteOptions: list.map((name) => ({ name, on: false })) })
    } catch (_) { /* ignore */ }
  },

  // ---------- 条件 ----------

  pickMeal(event) { ui.haptic(); this.setData({ meal: event.currentTarget.dataset.key }) },
  pickMood(event) { ui.haptic(); this.setData({ mood: event.currentTarget.dataset.key }) },
  pickCookTime(event) { ui.haptic(); this.setData({ maxCookTime: Number(event.currentTarget.dataset.value) }) },
  toggleTaste(event) {
    const index = Number(event.currentTarget.dataset.index)
    ui.haptic()
    this.setData({ [`tasteOptions[${index}].on`]: !this.data.tasteOptions[index].on })
  },
  onInclude(event) { this.setData({ includeText: event.detail.value }) },
  onExclude(event) { this.setData({ excludeText: event.detail.value }) },

  buildRequest() {
    const req = { count: 4, mode: "assistant" }
    if (this.data.meal) req.meal_type = this.data.meal
    if (this.data.mood) req.mood = this.data.mood
    const tastes = this.data.tasteOptions.filter((item) => item.on).map((item) => item.name)
    if (tastes.length) req.tastes = tastes
    if (this.data.maxCookTime > 0) req.max_cook_time = this.data.maxCookTime
    const include = splitWords(this.data.includeText)
    if (include.length) req.include_ingredients = include
    const exclude = splitWords(this.data.excludeText)
    if (exclude.length) req.exclude_ingredients = exclude
    if (this._rejected.length) req.exclude_dish_ids = this._rejected.slice()
    return req
  },

  async recommend() {
    if (this.data.picking) return
    ui.haptic("medium")
    this.setData({ picking: true })
    try {
      const result = await api.post("/pick/smart", this.buildRequest())
      this._raw = result.items || []
      const items = this._raw.map((item) => ({ ...dishUtil.toCard(item.dish), score: Math.round(item.score || 0), reasons: item.reasons || [] }))
      this.setData({ items, summary: result.profile_summary || "" })
      if (!items.length) ui.toast("没有符合条件的菜，放宽一下试试")
      else wx.pageScrollTo({ selector: "#results", offsetTop: -120, duration: 300 })
    } catch (_) {
      ui.toast("暂无可推荐的菜品，放宽条件再试")
    } finally {
      this.setData({ picking: false })
    }
  },

  // ---------- 结果操作 ----------

  openDish(event) {
    wx.navigateTo({ url: `/pages/dish/dish?id=${event.currentTarget.dataset.id}` })
  },

  async eat(event) {
    const id = Number(event.currentTarget.dataset.id)
    const meal = event.currentTarget.dataset.meal
    const item = this.data.items.find((entry) => entry.id === id)
    if (!item || this.data.busyId) return
    this.setData({ busyId: id })
    try {
      await api.post("/records", { dish_id: id, dish_name: item.name, meal_type: meal, meal_date: fmt.dateKey() })
      ui.toast(`❤ 已记录到今日${meal === "lunch" ? "午餐" : "晚餐"}`)
      const confetti = this.selectComponent("#confetti")
      if (confetti) confetti.fire()
      api.behavior({ event_type: "accept", dish_id: id, dish_name: item.name, meta: { from: "assistant", meal_type: meal, score: item.score, client: "miniprogram" } })
    } catch (error) {
      ui.toast(error.message || "记录失败")
    } finally {
      this.setData({ busyId: 0 })
    }
  },

  reject(event) {
    const id = Number(event.currentTarget.dataset.id)
    const item = this.data.items.find((entry) => entry.id === id)
    if (!item) return
    ui.haptic()
    api.behavior({ event_type: "reject", dish_id: id, dish_name: item.name, meta: { from: "assistant", client: "miniprogram" } })
    this._rejected.push(id)
    this.setData({ items: this.data.items.filter((entry) => entry.id !== id) })
    ui.toast("👌 好的，下次少推这道")
  },

  goChat() { wx.navigateTo({ url: "/pages/chat/chat" }) },
  goProfile() { wx.navigateTo({ url: "/pages/taste-profile/taste-profile" }) },
})
