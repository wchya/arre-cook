const api = require("../../utils/api")
const session = require("../../utils/session")
const ui = require("../../utils/ui")
const media = require("../../utils/media")
const dishUtil = require("../../utils/dish")
const fmt = require("../../utils/format")

const PORTIONS = [1, 2, 3, 4]
const HERO_RPX = 620

function splitAmount(amount) {
  const match = String(amount || "").trim().match(/^(\d+(?:\.\d+)?)(.*)$/)
  return match ? { num: Number(match[1]), unit: match[2] } : null
}

function scaleAmount(amount, scale) {
  if (scale === 1) return amount
  const parsed = splitAmount(amount)
  if (!parsed) return amount
  return `${Math.round(parsed.num * scale * 100) / 100}${parsed.unit}`
}

// 与 Web 端 DishDetail.getMasteryLevel 一致
function mastery(count) {
  if (count >= 10) return { label: "大师", icon: "👨‍🍳", tone: "yellow" }
  if (count >= 6) return { label: "熟练", icon: "🔥", tone: "primary" }
  if (count >= 3) return { label: "入门", icon: "✨", tone: "mint" }
  return { label: "初学", icon: "🌱", tone: "purple" }
}

function stars(value) {
  const rounded = Math.round(Number(value) || 0)
  return [1, 2, 3, 4, 5].map((n) => ({ n, on: n <= rounded }))
}

Page({
  data: {
    id: 0,
    loading: true,
    notFound: false,
    heroHeight: HERO_RPX,
    navOpacity: 0,

    dish: null,
    gallery: [],
    galleryIndex: 0,
    chips: [],
    tags: [],
    favorite: false,
    favAnim: false,
    canEdit: false,
    deleteMode: "none",
    pendingRequestId: 0,
    deleting: false,
    videoMeta: null,

    portions: PORTIONS,
    portion: 1,
    ingredients: [],
    seasonings: [],
    preparedCount: 0,
    steps: [],

    stats: null,
    mastery: null,
    moodBar: [],
    ratingStars: [],
    avgRating: "",
    lastDate: "",
    records: [],
    photos: [],
    similar: [],

    todayLunch: false,
    todayDinner: false,
    recording: "",
  },

  onLoad(query) {
    const id = Number(query.id) || 0
    const nav = getApp().navMetrics()
    const heroPx = (HERO_RPX * nav.windowWidth) / 750
    this._fadeStart = 40
    this._fadeRange = Math.max(120, heroPx - nav.navHeight - this._fadeStart)
    this.setData({ id })
    if (!id) {
      this.setData({ loading: false, notFound: true })
      return
    }
    if (!session.requireLogin(`/pages/dish/dish?id=${id}`)) return
    this.load()
  },

  onShow() {
    // 从烹饪模式 / 记录页返回时刷新“做菜回忆”和今日记录状态
    if (this._loaded) {
      this.loadRecords()
      this.loadToday()
    }
  },

  onPageScroll(event) {
    const value = Math.max(0, Math.min(1, (event.scrollTop - this._fadeStart) / this._fadeRange))
    const opacity = Math.round(value * 20) / 20
    if (opacity !== this.data.navOpacity) this.setData({ navOpacity: opacity })
  },

  onShareAppMessage() {
    const dish = this.data.dish
    return {
      title: dish ? `${dish.name} · 今天就做这道` : "ss-menu · 今天吃什么",
      path: `/pages/dish/dish?id=${this.data.id}`,
      imageUrl: dish && dish.cover ? dish.cover : undefined,
    }
  },

  async load() {
    this.setData({ loading: true, notFound: false })
    try {
      const raw = await api.get(`/dishes/${this.data.id}`)
      this.applyDish(raw)
      this._loaded = true
      api.behavior({ event_type: "view", dish_id: raw.id, dish_name: raw.name, meta: { from: "detail", client: "miniprogram" } })
      this.loadRecords()
      this.loadToday()
      this.loadSimilar(raw)
    } catch (error) {
      this.setData({ notFound: error.status === 404, loading: false })
      if (error.status !== 404) ui.toast(error.message || "菜品加载失败")
    }
  },

  applyDish(raw) {
    const card = dishUtil.toCard(raw)
    const diff = dishUtil.difficulty(raw.difficulty)
    const chips = [{ text: card.category, tone: "primary" }]
    if (raw.cook_time) chips.push({ text: `${raw.cook_time}分钟`, tone: "mint" })
    chips.push({ text: diff.label, tone: "primary" })
    card.tasteTags.forEach((taste) => chips.push({ text: taste, tone: "pink" }))
    chips.push({ text: dishUtil.mealLabel(raw.meal_type), tone: "purple" })
    if (card.isPrivate) chips.push({ text: "🔒 私房菜", tone: "ghost" })
    if (card.isFamily) chips.push({ text: "🏠 家庭菜谱", tone: "ghost" })
    chips.forEach((chip, index) => { chip.key = `c${index}` })

    const gallery = media.dishGallery(raw)
    const steps = dishUtil.normalizeSteps(raw.steps).map((step, index) => ({
      index,
      text: step.text,
      time: step.time,
      image: media.assetUrl(step.image),
    }))

    this._ingredientsRaw = dishUtil.normalizeIngredients(raw.ingredients)
    this._seasoningsRaw = dishUtil.normalizeIngredients(raw.seasonings)
    this._prepared = {}

    const access = raw.access || {}
    wx.setNavigationBarTitle({ title: raw.name || "菜品详情" })
    this.setData({
      loading: false,
      dish: { ...card, remark: raw.remark || "", videoUrl: raw.video_url || "" },
      favorite: Boolean(raw.favorite),
      canEdit: Boolean(access.can_edit),
      deleteMode: access.delete_mode || "none",
      pendingRequestId: access.pending_request_id || 0,
      videoMeta: this.decorateMeta(raw.video_meta),
      gallery,
      galleryIndex: 0,
      chips,
      tags: media.asArray(raw.tags).filter((tag) => typeof tag === "string" && tag),
      steps,
    })
    this.renderIngredients()
  },

  // video_meta（服务端抓取的链接预览）转视图模型；封面转成可加载地址。
  decorateMeta(meta) {
    if (!meta || typeof meta !== "object") return null
    return {
      platformName: meta.platform_name || "链接",
      title: meta.title || "",
      cover: media.assetUrl(meta.cover),
      author: meta.author || "",
      duration: meta.duration || "",
      playable: Boolean(meta.playable),
    }
  },

  onVideoCoverError() {
    if (this.data.videoMeta) this.setData({ "videoMeta.cover": "" })
  },

  // ---------- 食材 ----------

  renderIngredients() {
    const portion = this.data.portion
    const map = (list, prefix) => list.map((item, index) => {
      const key = `${prefix}${index}`
      return { key, name: item.name, amount: scaleAmount(item.amount, portion), done: Boolean(this._prepared[key]) }
    })
    const ingredients = map(this._ingredientsRaw || [], "i")
    const seasonings = map(this._seasoningsRaw || [], "s")
    const preparedCount = ingredients.concat(seasonings).filter((item) => item.done).length
    this.setData({ ingredients, seasonings, preparedCount })
  },

  choosePortion(event) {
    const portion = Number(event.currentTarget.dataset.value)
    if (!portion || portion === this.data.portion) return
    ui.haptic()
    this.setData({ portion }, () => this.renderIngredients())
  },

  togglePrepared(event) {
    const key = event.currentTarget.dataset.key
    this._prepared[key] = !this._prepared[key]
    ui.haptic()
    this.renderIngredients()
  },

  // ---------- 附加数据 ----------

  async loadRecords() {
    try {
      const result = await api.get(`/dishes/${this.data.id}/records`)
      const stats = result.stats || {}
      const records = (result.records || []).slice(0, 10).map((record) => {
        const photos = [record.photo].concat(record.photos || []).filter((url) => media.isImageUrl(url)).map(media.assetUrl)
        const homeMood = dishUtil.HOME_MOOD_MAP[record.home_mood]
        return {
          id: record.id,
          date: fmt.relativeDate(record.meal_date),
          weekday: fmt.weekday(record.meal_date),
          lunch: record.meal_type === "lunch",
          mood: dishUtil.MOOD_EMOJI[record.mood] || "",
          stars: record.rating > 0 ? stars(record.rating) : [],
          homeMood: homeMood ? `${homeMood.emoji} ${homeMood.label}` : "",
          remark: record.remark || "",
          photos,
        }
      })
      const photos = []
      records.forEach((record) => record.photos.forEach((url) => photos.push(url)))
      const moodBar = []
      if (stats.yum_percent) moodBar.push({ key: "yum", tone: "mint", pct: stats.yum_percent, label: "😋 好吃" })
      if (stats.ok_percent) moodBar.push({ key: "ok", tone: "yellow", pct: stats.ok_percent, label: "😐 一般" })
      if (stats.no_percent) moodBar.push({ key: "no", tone: "pink", pct: stats.no_percent, label: "😵 不行" })
      this.setData({
        stats: stats.total_count ? stats : null,
        mastery: stats.total_count ? mastery(stats.total_count) : null,
        moodBar,
        ratingStars: stats.avg_rating > 0 ? stars(stats.avg_rating) : [],
        avgRating: stats.avg_rating > 0 ? stats.avg_rating.toFixed(1) : "",
        lastDate: stats.last_date ? fmt.relativeDate(stats.last_date) : "",
        records,
        photos,
      })
    } catch (_) {
      // 做菜记录是附加信息，失败时静默
    }
  },

  async loadToday() {
    const today = fmt.dateKey()
    try {
      const result = await api.get("/records", { date_from: today, date_to: today, pageSize: 50 })
      const mine = (result.items || []).filter((record) => record.dish_id === this.data.id)
      this.setData({
        todayLunch: mine.some((record) => record.meal_type === "lunch"),
        todayDinner: mine.some((record) => record.meal_type === "dinner"),
      })
    } catch (_) { /* ignore */ }
  },

  async loadSimilar(raw) {
    if (!raw.category) return
    try {
      const result = await api.get("/dishes", { category: raw.category, enabled: "true", sort: "random", pageSize: 8 })
      const similar = dishUtil.toCards((result.items || []).filter((item) => item.id !== raw.id).slice(0, 6))
      this.setData({ similar })
    } catch (_) { /* ignore */ }
  },

  // 删除 / 申请删除：direct 本人或家庭管理员直接删；request 家庭成员提交申请，管理员同意后才删。
  async deleteDish() {
    if (this.data.deleting || this.data.deleteMode === "none") return
    if (this.data.deleteMode === "request" && this.data.pendingRequestId) {
      ui.toast("删除申请审核中，请等家庭管理员处理")
      return
    }
    const direct = this.data.deleteMode === "direct"
    const ok = await ui.confirm({
      title: direct ? "删除这道菜？" : "申请删除这道菜？",
      content: direct ? "删除后无法恢复，已有的用餐记录不受影响。" : "这是家庭共享菜谱，需要家庭管理员同意后才会删除。",
      confirmText: direct ? "删除" : "提交申请",
      danger: direct,
    })
    if (!ok) return
    this.setData({ deleting: true })
    try {
      const res = await api.delete(`/dishes/${this.data.id}`)
      if (res && res.deleted) {
        ui.toast("已删除", "success")
        setTimeout(() => wx.navigateBack(), 600)
      } else if (res && res.pending) {
        this.setData({ pendingRequestId: (res.request && res.request.id) || 1 })
        ui.toast("删除申请已提交，等家庭管理员处理")
      }
    } catch (error) {
      ui.toast(error.message || "操作失败")
    } finally {
      this.setData({ deleting: false })
    }
  },

  // ---------- 交互 ----------

  onGalleryChange(event) {
    this.setData({ galleryIndex: event.detail.current })
  },

  previewGallery(event) {
    if (!this.data.gallery.length) return
    const index = Number(event.currentTarget.dataset.index) || 0
    ui.preview(this.data.gallery, this.data.gallery[index])
  },

  previewStep(event) {
    const url = event.currentTarget.dataset.url
    const all = this.data.steps.map((step) => step.image).filter(Boolean)
    ui.preview(all, url)
  },

  previewMemory(event) {
    ui.preview(this.data.photos, event.currentTarget.dataset.url)
  },

  async toggleFavorite() {
    if (this._favBusy || !this.data.dish) return
    this._favBusy = true
    const active = this.data.favorite
    this.setData({ favorite: !active, favAnim: !active })
    ui.haptic(active ? "light" : "medium")
    try {
      if (active) await api.delete(`/favorites/${this.data.id}`)
      else await api.post(`/favorites/${this.data.id}`)
      ui.toast(active ? "已取消收藏" : "❤ 已收藏")
    } catch (error) {
      this.setData({ favorite: active })
      ui.toast(error.message || "操作失败")
    } finally {
      this._favBusy = false
      setTimeout(() => this.setData({ favAnim: false }), 520)
    }
  },

  async recordMeal(event) {
    const meal = event.currentTarget.dataset.meal
    const dish = this.data.dish
    if (!dish || this.data.recording) return
    const label = meal === "lunch" ? "午餐" : "晚餐"
    if ((meal === "lunch" && this.data.todayLunch) || (meal === "dinner" && this.data.todayDinner)) {
      ui.toast(`${dish.name} 已在今日${label}中记录过啦~`)
      return
    }
    this.setData({ recording: meal })
    try {
      await api.post("/records", { dish_id: dish.id, dish_name: dish.name, meal_type: meal, meal_date: fmt.dateKey() })
      ui.haptic("medium")
      const confetti = this.selectComponent("#confetti")
      if (confetti) confetti.fire()
      ui.toast(`❤ 已记入今日${label}`)
      api.behavior({ event_type: "accept", dish_id: dish.id, dish_name: dish.name, meta: { from: "detail", meal_type: meal, client: "miniprogram" } })
      this.setData({ [meal === "lunch" ? "todayLunch" : "todayDinner"]: true })
      this.loadRecords()
    } catch (error) {
      ui.toast(error.message || "记录失败")
    } finally {
      this.setData({ recording: "" })
    }
  },

  startCooking(event) {
    const step = Number(event.currentTarget.dataset.step) || 0
    wx.navigateTo({ url: `/pages/cook/cook?id=${this.data.id}&step=${step}` })
  },

  copyVideo() {
    const url = this.data.dish && this.data.dish.videoUrl
    if (!url) return
    wx.setClipboardData({
      data: url,
      success: () => ui.toast("视频链接已复制，可在浏览器中打开"),
    })
  },

  editDish() {
    wx.navigateTo({ url: `/pages/dish-edit/dish-edit?id=${this.data.id}` })
  },

  goHome() {
    wx.switchTab({ url: "/pages/home/home" })
  },
})
