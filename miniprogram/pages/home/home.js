const api = require("../../utils/api")
const session = require("../../utils/session")
const ui = require("../../utils/ui")
const dishUtil = require("../../utils/dish")
const fmt = require("../../utils/format")
const theme = require("../../utils/theme")
const recommendation = require("../../utils/recommendation")

const SPIN_MS = 3200

function buildWheel(dishes) {
  const n = dishes.length
  if (!n) return { background: "", labels: [] }
  const t = theme.palette().tokens
  const colors = [t["primary-action"], t["primary-light"], t.card, t.inset]
  const texts = [t["on-primary"], t.text, t.text, t.text]
  const seg = 360 / n
  const stops = dishes.map((_, i) => `${colors[i % colors.length]} ${(i * seg).toFixed(2)}deg ${((i + 1) * seg).toFixed(2)}deg`)
  const labels = dishes.map((dish, i) => ({
    id: dish.id,
    name: dish.name.length > 5 ? `${dish.name.slice(0, 5)}…` : dish.name,
    angle: (i * seg + seg / 2 - 90).toFixed(2),
    color: texts[i % texts.length],
  }))
  return { background: `conic-gradient(${stops.join(", ")})`, labels }
}

require("../../utils/theme").page({
  data: {
    healthSummary: null, healthSummaryLoading: true, healthSummaryError: false,
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

    refreshing: false,
    error: "",
  },

  onLoad() {
    this._offTheme = theme.bindRefresher(this)
    this._today = []
    this._wheelDishes = []
    this._offWheelTheme = theme.subscribe(() => {
      const built = buildWheel(this._wheelDishes)
      this.setData({ wheelBg: built.background, wheelLabels: built.labels })
    })
    this._seen = []
    this._recommendationId = ""
    this.boot()
  },

  onShow() {
    this._summaryHidden = false
    this._recommendationHidden = false
    if (this._recommendationIdentity !== api.token()) {
      this._recommendationIdentity = api.token()
      this._seen = []
      this._recommendationId = ""
      this._bootDate = ""
      this._recRaw = []
      this._today = []
      this.setData({ current: null, recItems: [], currentReasons: [], recMeal: "", mood: "" })
    }
    if (this._summaryIdentity !== api.token()) { this._summaryIdentity = api.token(); this.setData({ healthSummary: null, healthSummaryLoading: true, healthSummaryError: false }) }
    session.syncTabBar(this, 0)
    if (!session.requireLogin()) return
    this.setData({ greeting: fmt.greeting() })
    if (this._booted) {
      this.loadToday()
      this.loadUnread()
      if (!this._booting) this.boot()
    }
  },

  onHide() { this._recommendationHidden = true; recommendation.disconnect(this); this._summaryHidden = true; this._summaryRequest = (this._summaryRequest || 0) + 1 },

  onUnload() {
    recommendation.disconnect(this)
    this._summaryHidden = true; this._summaryRequest = (this._summaryRequest || 0) + 1
    if (this._offWheelTheme) this._offWheelTheme()
    clearTimeout(this._spinTimer)
    if (this._offTheme) this._offTheme()
  },

  onShareAppMessage() {
    const dish = this.data.current
    if (dish) return { title: `今天就吃「${dish.name}」吧`, path: `/pages/dish/dish?id=${dish.id}`, imageUrl: dish.cover || undefined }
    return { title: "arre食谱推荐小助手 · 今天吃什么", path: "/pages/home/home" }
  },

  async boot() {
    if (!session.hasSession() || this._booting) return
    this._booted = true
    this._booting = true
    this.setData({ loadingRec: true, error: "" })
    const today = recommendation.dateKey()
    if (this._bootDate !== today) {
      this._bootDate = today
      this._seen = []
      this._recommendationId = ""
      this.setData({ mood: "", current: null, recMeal: "" })
    }
    try {
      const rating = await api.get("/day-rating", { date: today }).catch(() => null)
      const mood = rating && rating.home_mood || ""
      this.setData({ mood })
      const results = await Promise.allSettled([
        api.post("/pick/smart", { count: 3, mode: "home_auto", meal_date: today, meal_type: this.data.recMeal || undefined, mood: mood || undefined }),
        api.post("/pick/smart", { count: 10, mode: "wheel", meal_date: today, mood: mood || undefined }),
        this.loadToday(), this.loadUnread(),
      ])
      const [pick, wheel] = results
      if (pick.status === "fulfilled" && pick.value && pick.value.items) this.setRecommendations(pick.value, this.data.recMeal)
      else this.setData({ current: null, error: "暂时无法加载推荐，下拉重试或调整条件" })
      if (wheel.status === "fulfilled") {
        this._wheelResult = wheel.value
        this._wheelDishes = (wheel.value.items || []).map((item) => item.dish)
        const built = buildWheel(this._wheelDishes)
        this.setData({ wheelBg: built.background, wheelLabels: built.labels })
      }
      if (mood) this.loadMoodDishes(mood)
    } finally {
      this._booting = false
      this.setData({ loadingRec: false })
    }
  },

  async onRefresh() {
    this.setData({ refreshing: true, recItems: [], recIndex: 0, current: null, recMeal: "", recQuote: "" })
    await this.boot()
    this.setData({ refreshing: false })
  },

  async loadToday() {
    const today = recommendation.dateKey()
    try {
      const result = await api.get("/meal-plans", { date_from: today, date_to: today })
      this._today = result || []
    } catch (_) { /* ignore */ }
    await this.loadHealthSummary()
  },

  async loadHealthSummary() {
    if (this._summaryHidden || !api.token()) return
    const identity = api.token()
    const request = this._summaryRequest = (this._summaryRequest || 0) + 1
    this.setData({ healthSummary: null, healthSummaryLoading: true, healthSummaryError: false })
    try {
      const result = await api.get("/health/summary")
      if (identity === api.token() && request === this._summaryRequest && !this._summaryHidden) this.setData({ healthSummary: result, healthSummaryLoading: false })
    } catch (_) {
      if (identity === api.token() && request === this._summaryRequest && !this._summaryHidden) this.setData({ healthSummaryError: true, healthSummaryLoading: false })
    }
  },
  goHealthReport() { wx.navigateTo({ url: "/pages/diary/diary?view=report" }) },

  async loadUnread() {
    try {
      const result = await api.get("/notifications", { pageSize: 1 })
      this.setData({ unread: result.unread || 0 })
    } catch (_) { /* ignore */ }
  },

  // ---------- 推荐 ----------

  setRecommendations(result, meal) {
    if (this._recommendationId !== result.recommendation_id) this._seen = this._seen || []
    this._recommendationId = result.recommendation_id
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
    recommendation.disconnect(this)
    this._seen = Array.from(new Set([...(this._seen || []), item.dish.id]))
    this.setData({
      recIndex: index,
      current: dishUtil.toCard(item.dish),
      currentReasons: item.reasons || [],
      isSmart: true,
    }, () => recommendation.observe(this, ".rec-card", this._recommendationId, item.dish.id))
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
      const result = await api.post("/pick/smart", { meal_type: meal, meal_date: recommendation.dateKey(), count: 3, mood: this.data.mood || undefined, mode: "home_auto" })
      this._seen = []
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

  // Session skips are temporary and never become long-term dislikes.
  async changeRecommend() {
    if (this.data.changing) return
    const raw = this._recRaw && this._recRaw[this.data.recIndex]
    this.setData({ changing: true })
    try {
      if (raw && this.data.isSmart) await recommendation.feedback(this._recommendationId, raw.dish.id, "skip")
      ui.haptic()
      if (this.data.isSmart && this._recRaw && this.data.recIndex < this._recRaw.length - 1) {
        this.showItem(this.data.recIndex + 1)
        return
      }
      const result = await api.post("/pick/smart", {
        meal_type: this.data.recMeal || undefined, meal_date: recommendation.dateKey(),
        mood: this.data.mood || undefined, count: 3, exclude_dish_ids: this._seen || [],
        session_id: this._recommendationId || undefined, mode: "home_change", refresh: true,
      })
      if (!result || !result.items || !result.items.length) { ui.toast("符合条件的菜已经看完，可以调整条件再试"); return }
      this.setRecommendations(result, this.data.recMeal)
    } catch (error) {
      ui.toast(error.message || "暂时无法换菜，请重试")
    } finally { this.setData({ changing: false }) }
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

  async record(dish, meal) {
    if (this.data.recordingMeal) return
    const label = meal === "lunch" ? "午餐" : "晚餐"
    const today = recommendation.dateKey()
    if (this._today.some((record) => record.dish_id === dish.id && record.meal_type === meal && record.meal_date === today)) {
      ui.toast(`${dish.name} 已在今日${label}菜单中啦~`)
      return
    }
    this.setData({ recordingMeal: meal })
    try {
      await api.post("/meal-plans", { dish_id: dish.id, meal_type: meal, meal_date: today, recommendation_id: this.data.isSmart ? this._recommendationId : undefined })
      ui.haptic("medium")
      ui.toast("❤ 已加入菜单，吃过后再记餐")
      const confetti = this.selectComponent("#confetti")
      if (confetti) confetti.fire()
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
    api.post("/day-rating/home-mood", { meal_date: recommendation.dateKey(), home_mood: mood })
      .then(() => ui.toast(`心情：${meta ? meta.label : mood}，已保存并调整推荐~`))
      .catch(() => ui.toast("心情保存失败"))
    this.loadMoodDishes(mood)
    this._seen = []
    this.setData({ current: null, loadingRec: true })
    try {
      const result = await api.post("/pick/smart", { count: 3, meal_date: recommendation.dateKey(), meal_type: this.data.recMeal || undefined, mood, mode: "home_auto" })
      if (mood === this.data.mood) this.setRecommendations(result, this.data.recMeal)
    } catch (error) { ui.toast(error.message || "推荐调整失败，请重试") }
    finally { this.setData({ loadingRec: false }) }
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
      this._recRaw = [this._wheelResult.items[pick]]
      this._recommendationId = this._wheelResult.recommendation_id
      this.showItem(0)
      ui.haptic("heavy")
      ui.toast(`🎯 转到了：${dish.name}！`)
      this.scrollToRec()
    }, SPIN_MS + 80)
  },

  // ---------- 导航 ----------

  goAssistant() { wx.navigateTo({ url: "/pages/assistant/assistant" }) },
  goTomorrow() { wx.navigateTo({ url: "/pages/tomorrow/tomorrow" }) },
  goNotifications() { wx.navigateTo({ url: "/pages/notifications/notifications" }) },
})
