const api = require("../../utils/api")
const session = require("../../utils/session")
const ui = require("../../utils/ui")
const media = require("../../utils/media")
const dishUtil = require("../../utils/dish")
const recommendation = require("../../utils/recommendation")

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

require("../../utils/theme").page({
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
    this._seen = []
    this._recommendationId = ""
    if (!session.requireLogin("/pages/assistant/assistant")) return
    this.loadProfile()
    this.loadTastes()
  },

  onShow() {
    this._recommendationHidden = false
    ;(this.data.items || []).forEach((item) => recommendation.observe(this, `#recommendation-${item.id}`, this._recommendationId, item.id))
  },
  onHide() { this._recommendationHidden = true; recommendation.disconnect(this) },
  onUnload() { recommendation.disconnect(this) },

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
    const context = JSON.stringify([recommendation.dateKey(), this.data.meal, this.data.mood, this.data.maxCookTime, this.data.includeText, this.data.excludeText, this.data.tasteOptions])
    const sameContext = this._recommendationContext === context
    if (!sameContext) this._seen = []
    this._recommendationContext = context
    const req = { count: 4, mode: "assistant", meal_date: recommendation.dateKey(), refresh: true, session_id: sameContext ? this._recommendationId || undefined : undefined }
    if (this.data.meal) req.meal_type = this.data.meal
    if (this.data.mood) req.mood = this.data.mood
    const tastes = this.data.tasteOptions.filter((item) => item.on).map((item) => item.name)
    if (tastes.length) req.tastes = tastes
    if (this.data.maxCookTime > 0) req.max_cook_time = this.data.maxCookTime
    const include = splitWords(this.data.includeText)
    if (include.length) req.include_ingredients = include
    const exclude = splitWords(this.data.excludeText)
    if (exclude.length) req.exclude_ingredients = exclude
    if (this._rejected.length || this._seen.length) req.exclude_dish_ids = Array.from(new Set(this._rejected.concat(this._seen)))
    return req
  },

  async recommend() {
    if (this.data.picking) return
    ui.haptic("medium")
    this.setData({ picking: true })
    try {
      const result = await api.post("/pick/smart", this.buildRequest())
      this._recommendationId = result.recommendation_id
      this._raw = result.items || []
      this._seen = Array.from(new Set(this._seen.concat(this._raw.map((item) => item.dish.id))))
      recommendation.disconnect(this)
      const items = this._raw.map((item) => ({ ...dishUtil.toCard(item.dish), reasons: item.reasons || [] }))
      this.setData({ items, summary: result.profile_summary || "" }, () => items.forEach((item) => recommendation.observe(this, `#recommendation-${item.id}`, this._recommendationId, item.id)))
      if (!items.length) ui.toast("没有符合条件的菜，放宽一下试试")
      else wx.pageScrollTo({ selector: "#results", offsetTop: -120, duration: 300 })
    } catch (_) {
      this._recommendationContext = ""
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
      await api.post("/meal-plans", { dish_id: id, meal_type: meal, meal_date: recommendation.dateKey(), recommendation_id: this._recommendationId })
      ui.toast(`❤ 已加入今日${meal === "lunch" ? "午餐" : "晚餐"}`)
      const confetti = this.selectComponent("#confetti")
      if (confetti) confetti.fire()
    } catch (error) {
      ui.toast(error.message || "记录失败")
    } finally {
      this.setData({ busyId: 0 })
    }
  },

  async reject(event) {
    const id = Number(event.currentTarget.dataset.id)
    const kind = event.currentTarget.dataset.kind === "dislike" ? "dislike" : "not_today"
    const item = this.data.items.find((entry) => entry.id === id)
    if (!item || this.data.busyId) return
    this.setData({ busyId: id })
    try {
      await recommendation.feedback(this._recommendationId, id, kind)
      this._rejected.push(id)
      this.setData({ items: this.data.items.filter((entry) => entry.id !== id) })
      if (kind === "dislike") {
        this._lastDislike = { id, recommendationId: this._recommendationId }
        this.setData({ canUndoDislike: true })
        ui.toast("已减少推荐，可以撤销")
      } else ui.toast("👌 这餐先不推荐这道")
    } catch (error) { ui.toast(error.message || "反馈保存失败，请重试") }
    finally { this.setData({ busyId: 0 }) }
  },

  async undoDislike() {
    const last = this._lastDislike
    if (!last || this.data.busyId) return
    this.setData({ busyId: last.id })
    try {
      await recommendation.feedback(last.recommendationId, last.id, "undo_dislike")
      this.setData({ canUndoDislike: false })
      ui.toast("已撤销不喜欢")
    } catch (error) { ui.toast(error.message || "撤销失败，请重试") }
    finally { this.setData({ busyId: 0 }) }
  },

  goChat() { wx.navigateTo({ url: "/pages/chat/chat" }) },
  goProfile() { wx.navigateTo({ url: "/pages/taste-profile/taste-profile" }) },
})
