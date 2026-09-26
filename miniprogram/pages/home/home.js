const api = require("../../utils/api")
const session = require("../../utils/session")
const ui = require("../../utils/ui")
const dishUtil = require("../../utils/dish")
const fmt = require("../../utils/format")
const theme = require("../../utils/theme")

// 转盘配色与 Web 端 Home.drawWheel 一致
const WHEEL_COLORS = ["#E8734A", "#6EC6B8", "#F5D76E", "#F4A8A0", "#8B5CF6", "#F0E6FF"]
const WHEEL_TEXT = ["#FFFFFF", "#FFFFFF", "#7A5A06", "#FFFFFF", "#FFFFFF", "#6B4FC8"]
const SPIN_MS = 3200

function buildWheel(dishes) {
  const n = dishes.length
  if (!n) return { background: "", labels: [] }
  const seg = 360 / n
  const stops = dishes.map((_, i) => `${WHEEL_COLORS[i % WHEEL_COLORS.length]} ${(i * seg).toFixed(2)}deg ${((i + 1) * seg).toFixed(2)}deg`)
  const labels = dishes.map((dish, i) => ({
    id: dish.id,
    name: dish.name.length > 5 ? `${dish.name.slice(0, 5)}…` : dish.name,
    angle: (i * seg + seg / 2 - 90).toFixed(2),
    color: WHEEL_TEXT[i % WHEEL_TEXT.length],
  }))
  return { background: `conic-gradient(${stops.join(", ")})`, labels }
}

Page({
  data: {
    greeting: fmt.greeting(),
    unread: 0,

    recItems: [],
    recIndex: 0,
    current: null,
    currentReasons: [],
    isSmart: false,
    recMeal: "",
    recQuote: "",
    profileSummary: "",
    loadingRec: true,
    pickingMeal: "",
    changing: false,
    recordingMeal: "",
    recScrollId: "",

    moods: dishUtil.HOME_MOODS,
    mood: "",
    moodDishes: [],
    moodLoading: false,

    wheelBg: "",
    wheelLabels: [],
    wheelDeg: 0,
    spinning: false,

    blindEnabled: true,
    blindFlipped: false,
    blindLoading: false,
    blindDish: null,
    blindHint: "",

    refreshing: false,
    error: "",
  },

  onLoad() {
    this._offTheme = theme.bindRefresher(this)
    this._today = []
    this._wheelDishes = []
    this._fallback = []
    this.boot()
  },

  onShow() {
    session.syncTabBar(this, 0)
    if (!session.requireLogin()) return
    this.setData({ greeting: fmt.greeting() })
    if (this._booted) {
      this.loadToday()
      this.loadUnread()
    }
  },

  onUnload() {
    clearTimeout(this._spinTimer)
    if (this._offTheme) this._offTheme()
  },

  onShareAppMessage() {
    const dish = this.data.current
    if (dish) return { title: `今天就吃「${dish.name}」吧`, path: `/pages/dish/dish?id=${dish.id}`, imageUrl: dish.cover || undefined }
    return { title: "ss-menu · 今天吃什么", path: "/pages/home/home" }
  },

  async boot() {
    if (!session.hasSession()) return
    this._booted = true
    this.setData({ loadingRec: true, error: "" })
    const today = fmt.dateKey()
    const results = await Promise.allSettled([
      api.post("/pick/smart", { count: 3, mode: "home_auto" }),
      api.get("/dishes", { enabled: "true", pageSize: 12, sort: "random" }),
      api.get("/day-rating", { date: today }),
      api.get("/settings"),
      this.loadToday(),
      this.loadUnread(),
    ])
    const [pick, wheel, rating, settings] = results
    if (pick.status === "fulfilled" && pick.value && pick.value.items && pick.value.items.length) {
      this.setRecommendations(pick.value, "")
    }
    if (wheel.status === "fulfilled") {
      const items = (wheel.value && wheel.value.items) || []
      this._wheelDishes = items
      this._fallback = items
      const built = buildWheel(items)
      this.setData({ wheelBg: built.background, wheelLabels: built.labels })
      if (!this.data.current && items.length) this.showSingle(items[0], false)
    }
    if (rating.status === "fulfilled" && rating.value && rating.value.home_mood) {
      this.setData({ mood: rating.value.home_mood })
      this.loadMoodDishes(rating.value.home_mood)
    }
    if (settings.status === "fulfilled" && settings.value) {
      this.setData({ blindEnabled: String(settings.value.blind_box_enabled || "1") !== "0" })
    }
    if (pick.status === "rejected" && wheel.status === "rejected") this.setData({ error: "暂时无法加载推荐，下拉重试" })
    this.setData({ loadingRec: false })
  },

  async onRefresh() {
    this.setData({ refreshing: true, recItems: [], recIndex: 0, current: null, recMeal: "", recQuote: "" })
    await this.boot()
    this.setData({ refreshing: false })
  },

  async loadToday() {
    const today = fmt.dateKey()
    try {
      const result = await api.get("/records", { date_from: today, date_to: today, pageSize: 50 })
      this._today = result.items || []
    } catch (_) { /* ignore */ }
  },

  async loadUnread() {
    try {
      const result = await api.get("/notifications", { pageSize: 1 })
      this.setData({ unread: result.unread || 0 })
    } catch (_) { /* ignore */ }
  },

  // ---------- 推荐 ----------

  setRecommendations(result, meal) {
    this._recRaw = result.items
    this.setData({
      recItems: result.items.map((item) => item.dish.id),
      recIndex: 0,
      recMeal: meal,
      recQuote: meal ? result.quote || "" : "",
      profileSummary: result.profile_summary || this.data.profileSummary,
    })
    this.showItem(0)
  },

  showItem(index) {
    const item = this._recRaw && this._recRaw[index]
    if (!item) return
    this.setData({
      recIndex: index,
      current: dishUtil.toCard(item.dish),
      currentReasons: item.reasons || [],
      isSmart: true,
    })
  },

  showSingle(dish, smart) {
    this.setData({ current: dishUtil.toCard(dish), currentReasons: [], isSmart: Boolean(smart) })
  },

  scrollToRec() {
    this.setData({ recScrollId: "" }, () => this.setData({ recScrollId: "rec-anchor" }))
  },

  async pickMeal(event) {
    const meal = event.currentTarget.dataset.meal
    if (this.data.pickingMeal) return
    ui.haptic()
    this.setData({ pickingMeal: meal })
    try {
      const result = await api.post("/pick/smart", { meal_type: meal, count: 3, mood: this.data.mood || undefined, mode: "home_meal" })
      if (!result || !result.items || !result.items.length) {
        ui.toast("暂无可推荐的菜品")
        return
      }
      this.setRecommendations(result, meal)
      ui.toast(`${meal === "lunch" ? "🍳 午餐" : "🍲 晚餐"}推荐：${result.items[0].dish.name}`)
      this.scrollToRec()
    } catch (_) {
      ui.toast("推荐失败，稍后再试")
    } finally {
      this.setData({ pickingMeal: "" })
    }
  },

  // 换一个：先在本批推荐里翻页，翻完了排除已看过的菜再要一批；被跳过的菜记一条 reject 事件
  async changeRecommend() {
    if (this.data.changing) return
    const raw = this._recRaw && this._recRaw[this.data.recIndex]
    if (raw && this.data.isSmart) {
      api.behavior({ event_type: "reject", dish_id: raw.dish.id, dish_name: raw.dish.name, meta: { from: "home_recommend", meal_type: this.data.recMeal, client: "miniprogram" } })
    }
    ui.haptic()
    if (this.data.isSmart && this._recRaw && this.data.recIndex < this._recRaw.length - 1) {
      this.showItem(this.data.recIndex + 1)
      return
    }
    this.setData({ changing: true })
    try {
      const seen = (this._recRaw || []).map((item) => item.dish.id)
      const result = await api.post("/pick/smart", {
        meal_type: this.data.recMeal || undefined,
        mood: this.data.mood || undefined,
        count: 3,
        exclude_dish_ids: seen,
        mode: "home_change",
      })
      if (!result || !result.items || !result.items.length) {
        ui.toast("没有更多推荐了")
        return
      }
      const quote = this.data.recQuote
      this.setRecommendations(result, this.data.recMeal)
      this.setData({ recQuote: quote })
    } catch (_) {
      // 回退：在已加载的菜单里顺序换
      const list = this._fallback
      if (!list.length) return
      const idx = this.data.current ? list.findIndex((dish) => dish.id === this.data.current.id) : -1
      this.showSingle(list[(idx + 1) % list.length], false)
    } finally {
      this.setData({ changing: false })
    }
  },

  openCurrent() {
    const dish = this.data.current
    if (dish) wx.navigateTo({ url: `/pages/dish/dish?id=${dish.id}` })
  },

  async recordCurrent(event) {
    const meal = event.currentTarget.dataset.meal
    const dish = this.data.current
    if (!dish) return
    await this.record(dish, meal, this.data.isSmart ? "home_recommend" : "home_pick")
  },

  async record(dish, meal, from) {
    if (this.data.recordingMeal) return
    const label = meal === "lunch" ? "午餐" : "晚餐"
    const today = fmt.dateKey()
    if (this._today.some((record) => record.dish_id === dish.id && record.meal_type === meal && record.meal_date === today)) {
      ui.toast(`${dish.name} 已在今日${label}中记录过啦~`)
      return
    }
    this.setData({ recordingMeal: meal })
    try {
      await api.post("/records", { dish_id: dish.id, dish_name: dish.name, meal_type: meal, meal_date: today })
      ui.haptic("medium")
      ui.toast("❤ 已记录！")
      const confetti = this.selectComponent("#confetti")
      if (confetti) confetti.fire()
      api.behavior({ event_type: "accept", dish_id: dish.id, dish_name: dish.name, meta: { from, meal_type: meal, client: "miniprogram" } })
      this.loadToday()
    } catch (error) {
      ui.toast(error.message || "记录失败")
    } finally {
      this.setData({ recordingMeal: "" })
    }
  },

  // ---------- 心情 ----------

  async chooseMood(event) {
    const mood = event.currentTarget.dataset.mood
    const meta = dishUtil.HOME_MOOD_MAP[mood]
    ui.haptic()
    this.setData({ mood })
    api.post("/day-rating/home-mood", { meal_date: fmt.dateKey(), home_mood: mood })
      .then(() => ui.toast(`心情：${meta ? meta.label : mood}，已保存并调整推荐~`))
      .catch(() => ui.toast("心情保存失败"))
    this.loadMoodDishes(mood)
  },

  async loadMoodDishes(mood) {
    this.setData({ moodLoading: true })
    try {
      const result = await api.post("/pick/mood", { mood })
      if (mood !== this.data.mood) return
      this.setData({ moodDishes: dishUtil.toCards((result.dishes || []).slice(0, 4)) })
    } catch (_) {
      this.setData({ moodDishes: [] })
    } finally {
      this.setData({ moodLoading: false })
    }
  },

  // ---------- 转盘 ----------

  spinWheel() {
    const dishes = this._wheelDishes
    if (this.data.spinning || !dishes.length) return
    ui.haptic("medium")
    const pick = Math.floor(Math.random() * dishes.length)
    const seg = 360 / dishes.length
    const target = (360 - (pick * seg + seg / 2)) % 360
    const base = this.data.wheelDeg - (this.data.wheelDeg % 360)
    this.setData({ spinning: true, wheelDeg: base + 360 * 5 + target })
    clearTimeout(this._spinTimer)
    this._spinTimer = setTimeout(() => {
      const dish = dishes[pick]
      this.setData({ spinning: false, recMeal: "", recQuote: "" })
      this.showSingle(dish, false)
      ui.haptic("heavy")
      ui.toast(`🎯 转到了：${dish.name}！`)
      this.scrollToRec()
    }, SPIN_MS + 80)
  },

  // ---------- 盲盒 ----------

  async openBlindBox() {
    if (this.data.blindFlipped || this.data.blindLoading) return
    this.setData({ blindLoading: true })
    try {
      const result = await api.post("/pick/blind-box")
      if (!result || !result.dish) throw new Error("暂无盲盒菜品")
      this.setData({ blindDish: dishUtil.toCard(result.dish), blindHint: result.hint || result.quote || "", blindFlipped: true })
      ui.haptic("medium")
      const confetti = this.selectComponent("#confetti")
      if (confetti) confetti.fire()
    } catch (error) {
      ui.toast(error.message || "暂无盲盒菜品")
    } finally {
      this.setData({ blindLoading: false })
    }
  },

  async eatBlindDish() {
    const dish = this.data.blindDish
    if (!dish) return
    this.showSingle(dish, false)
    await this.record(dish, "dinner", "home_blind_box")
    this.resetBlindBox()
  },

  openBlindDish() {
    const dish = this.data.blindDish
    if (dish) wx.navigateTo({ url: `/pages/dish/dish?id=${dish.id}` })
  },

  resetBlindBox() {
    this.setData({ blindFlipped: false })
    setTimeout(() => this.setData({ blindDish: null, blindHint: "" }), 500)
  },

  // ---------- 导航 ----------

  goAssistant() { wx.navigateTo({ url: "/pages/assistant/assistant" }) },
  goTomorrow() { wx.navigateTo({ url: "/pages/tomorrow/tomorrow" }) },
  goNotifications() { wx.navigateTo({ url: "/pages/notifications/notifications" }) },
})
