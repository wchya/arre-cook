const api = require("../../utils/api")
const session = require("../../utils/session")
const ui = require("../../utils/ui")
const media = require("../../utils/media")
const dishUtil = require("../../utils/dish")
const fmt = require("../../utils/format")
const theme = require("../../utils/theme")

const WEEK_HEAD = ["日", "一", "二", "三", "四", "五", "六"]
// 单道菜评价与整餐评价的选项与 Web 端 History 一致
const DISH_MOODS = [
  { key: "yum", emoji: "😋", label: "好吃" },
  { key: "ok", emoji: "😐", label: "一般" },
  { key: "no", emoji: "😵", label: "不想再吃" },
]
const MEAL_MOODS = [
  { key: "great", emoji: "😋", label: "超满足" },
  { key: "ok", emoji: "😐", label: "还行吧" },
  { key: "meh", emoji: "😕", label: "不太行" },
]
const MAX_DAY_PHOTOS = 9

function monthRange(year, month) {
  const last = new Date(year, month + 1, 0).getDate()
  const prefix = `${year}-${fmt.pad(month + 1)}`
  return { from: `${prefix}-01`, to: `${prefix}-${fmt.pad(last)}` }
}

function toRecordView(record) {
  return {
    id: record.id,
    dishId: record.dish_id,
    name: record.dish_name,
    date: record.meal_date,
    dateLabel: `${fmt.relativeDate(record.meal_date)} · ${record.meal_type === "lunch" ? "午餐" : record.meal_type === "dinner" ? "晚餐" : "加餐"}`,
    meal: record.meal_type,
    mealLabel: record.meal_type === "lunch" ? "午餐" : record.meal_type === "dinner" ? "晚餐" : "加餐",
    cover: media.assetUrl(record.dish_image_url),
    emoji: record.dish_emoji || "🍽",
    mood: record.mood || "",
    moodEmoji: dishUtil.MOOD_EMOJI[record.mood] || "",
    remark: record.remark || "",
    photoRaw: record.photo || "",
    photo: media.assetUrl(record.photo),
  }
}

Page({
  data: {
    weekHead: WEEK_HEAD,
    year: 0,
    month: 0,
    cells: [],
    selectedKey: "",
    selectedLabel: "",
    isToday: false,
    dayRecords: [],
    dayRating: null,
    dayPhotos: [],
    stats: null,
    topDish: "-",
    recent: [],
    loading: true,
    refreshing: false,
    swipeId: "",

    dishMoods: DISH_MOODS,
    mealMoods: MEAL_MOODS,

    rateOpen: false,
    rateRecord: null,
    rateMood: "",
    rateRemark: "",
    ratePhotoRaw: "",
    ratePhoto: "",
    rateSaving: false,
    rateUploading: false,

    dayOpen: false,
    dayMood: "",
    dayRemark: "",
    dayDraftPhotos: [],
    daySaving: false,
    dayUploading: false,
  },

  onLoad() {
    this._offTheme = theme.bindRefresher(this)
    const now = new Date()
    this._records = {}
    this._ratings = {}
    this.setData({ year: now.getFullYear(), month: now.getMonth(), selectedKey: fmt.dateKey(now) })
  },

  onUnload() {
    if (this._offTheme) this._offTheme()
  },

  onShow() {
    session.syncTabBar(this, 2)
    if (!session.requireLogin()) return
    this.loadAll()
  },

  async onRefresh() {
    this.setData({ refreshing: true })
    await this.loadAll()
    this.setData({ refreshing: false })
  },

  async loadAll() {
    await Promise.all([this.loadMonth(), this.loadStats(), this.loadRecent()])
    this.setData({ loading: false })
  },

  async loadMonth() {
    const { year, month } = this.data
    const range = monthRange(year, month)
    try {
      const [records, ratings] = await Promise.all([
        api.get("/records", { date_from: range.from, date_to: range.to, pageSize: 100 }),
        api.get("/day-ratings", { date_from: range.from, date_to: range.to }),
      ])
      if (year !== this.data.year || month !== this.data.month) return
      const byDate = {}
      ;(records.items || []).forEach((record) => {
        if (!byDate[record.meal_date]) byDate[record.meal_date] = []
        byDate[record.meal_date].push(record)
      })
      const ratingByDate = {}
      ;(ratings || []).forEach((rating) => { ratingByDate[rating.meal_date] = rating })
      this._records = byDate
      this._ratings = ratingByDate
    } catch (error) {
      ui.toast(error.message || "记录加载失败")
    }
    this.buildCalendar()
    this.applySelected()
  },

  async loadStats() {
    try {
      const stats = await api.get("/stats")
      const top = stats.top_dishes && stats.top_dishes[0]
      this.setData({ stats, topDish: top ? top.dish_name : "-" })
    } catch (_) { /* ignore */ }
  },

  async loadRecent() {
    try {
      const result = await api.get("/records", { pageSize: 5 })
      this.setData({ recent: (result.items || []).map(toRecordView) })
    } catch (_) { /* ignore */ }
  },

  // ---------- 日历 ----------

  buildCalendar() {
    const { year, month } = this.data
    const first = new Date(year, month, 1).getDay()
    const days = new Date(year, month + 1, 0).getDate()
    const prevDays = new Date(year, month, 0).getDate()
    const today = fmt.dateKey()
    const cells = []
    for (let i = first - 1; i >= 0; i--) cells.push({ key: `p${i}`, day: prevDays - i, other: true })
    for (let d = 1; d <= days; d++) {
      const key = `${year}-${fmt.pad(month + 1)}-${fmt.pad(d)}`
      const records = this._records[key] || []
      const lunch = records.some((record) => record.meal_type === "lunch")
      const dinner = records.some((record) => record.meal_type !== "lunch")
      const rating = this._ratings[key]
      const mood = rating && dishUtil.HOME_MOOD_MAP[rating.home_mood]
      cells.push({
        key,
        day: d,
        other: false,
        today: key === today,
        dot: lunch && dinner ? "both" : lunch ? "lunch" : dinner ? "dinner" : "",
        mood: mood ? mood.emoji : "",
      })
    }
    const rest = (7 - (cells.length % 7)) % 7
    for (let i = 1; i <= rest; i++) cells.push({ key: `n${i}`, day: i, other: true })
    this.setData({ cells })
  },

  changeMonth(event) {
    const step = Number(event.currentTarget.dataset.step)
    let { year, month } = this.data
    month += step
    if (month > 11) { month = 0; year += 1 }
    if (month < 0) { month = 11; year -= 1 }
    ui.haptic()
    const now = new Date()
    const selectedKey = year === now.getFullYear() && month === now.getMonth() ? fmt.dateKey(now) : ""
    this.setData({ year, month, selectedKey, swipeId: "" }, () => this.loadMonth())
  },

  selectDay(event) {
    const key = event.currentTarget.dataset.key
    if (!key || key.length < 10) return
    ui.haptic()
    this.setData({ selectedKey: key, swipeId: "" }, () => this.applySelected())
  },

  applySelected() {
    const key = this.data.selectedKey
    if (!key) {
      this.setData({ dayRecords: [], dayRating: null, dayPhotos: [], selectedLabel: "" })
      return
    }
    const rating = this._ratings[key] || null
    const mood = rating && dishUtil.HOME_MOOD_MAP[rating.home_mood]
    const d = fmt.parseDate(key)
    this.setData({
      selectedLabel: d ? `${d.getMonth() + 1}月${d.getDate()}日` : key,
      isToday: key === fmt.dateKey(),
      dayRecords: (this._records[key] || []).map(toRecordView),
      dayRating: rating ? {
        homeMood: mood ? `${mood.emoji} ${mood.label}` : "",
        mood: rating.mood || "",
        moodEmoji: dishUtil.MOOD_EMOJI[rating.mood] || "",
        remark: rating.remark || "",
      } : null,
      dayPhotos: media.photoList(rating && rating.photos).map((raw) => ({ raw, url: media.assetUrl(raw) })),
    })
  },

  // ---------- 左滑删除 ----------

  onTouchStart(event) {
    const touch = event.touches[0]
    this._touch = { x: touch.clientX, y: touch.clientY, id: event.currentTarget.dataset.swipe }
  },

  onTouchEnd(event) {
    const start = this._touch
    this._touch = null
    if (!start) return
    const touch = event.changedTouches[0]
    const dx = touch.clientX - start.x
    const dy = touch.clientY - start.y
    if (Math.abs(dx) < 36 || Math.abs(dx) < Math.abs(dy) * 1.2) return
    const swipeId = dx < 0 ? start.id : ""
    if (swipeId !== this.data.swipeId) this.setData({ swipeId })
  },

  closeSwipe() {
    if (this.data.swipeId) this.setData({ swipeId: "" })
  },

  async deleteRecord(event) {
    const id = Number(event.currentTarget.dataset.id)
    const ok = await ui.confirm({ title: "删除这条记录？", content: "删除后统计和成就会同步更新。", confirmText: "删除", danger: true })
    if (!ok) { this.closeSwipe(); return }
    try {
      await api.delete(`/records/${id}`)
      ui.toast("已删除", "success")
      this.setData({ swipeId: "" })
      await this.loadAll()
    } catch (error) {
      ui.toast(error.message || "删除失败")
    }
  },

  openDish(event) {
    if (this.data.swipeId) { this.closeSwipe(); return }
    wx.navigateTo({ url: `/pages/dish/dish?id=${event.currentTarget.dataset.id}` })
  },

  // ---------- 单道菜评价 ----------

  findRecord(id) {
    return this.data.dayRecords.concat(this.data.recent).find((record) => record.id === id)
  },

  openRate(event) {
    const record = this.findRecord(Number(event.currentTarget.dataset.id))
    if (!record) return
    this.setData({
      swipeId: "",
      rateOpen: true,
      rateRecord: record,
      rateMood: record.mood,
      rateRemark: record.remark,
      ratePhotoRaw: record.photoRaw,
      ratePhoto: record.photo,
    })
  },

  closeRate() { this.setData({ rateOpen: false }) },
  pickRateMood(event) { ui.haptic(); this.setData({ rateMood: event.currentTarget.dataset.mood }) },
  onRateRemark(event) { this.setData({ rateRemark: event.detail.value }) },

  async uploadRatePhoto() {
    if (this.data.rateUploading) return
    const [path] = await ui.chooseImages(1)
    if (!path) return
    this.setData({ rateUploading: true })
    try {
      const raw = await api.upload(path)
      this.setData({ ratePhotoRaw: raw, ratePhoto: media.assetUrl(raw) })
    } catch (error) {
      ui.toast(error.message)
    } finally {
      this.setData({ rateUploading: false })
    }
  },

  removeRatePhoto() { this.setData({ ratePhotoRaw: "", ratePhoto: "" }) },

  previewRatePhoto() { ui.preview([this.data.ratePhoto]) },

  async saveRate() {
    const record = this.data.rateRecord
    if (!record || !this.data.rateMood || this.data.rateSaving) return
    this.setData({ rateSaving: true })
    try {
      await api.put(`/records/${record.id}`, { mood: this.data.rateMood, remark: this.data.rateRemark.trim(), photo: this.data.ratePhotoRaw })
      ui.toast("评价已保存", "success")
      this.setData({ rateOpen: false })
      await Promise.all([this.loadMonth(), this.loadRecent()])
    } catch (error) {
      ui.toast(error.message || "评价失败")
    } finally {
      this.setData({ rateSaving: false })
    }
  },

  // ---------- 整餐评价 + 当天照片 ----------

  openDayRate() {
    const rating = this._ratings[this.data.selectedKey]
    this.setData({
      dayOpen: true,
      dayMood: (rating && rating.mood) || "",
      dayRemark: (rating && rating.remark) || "",
      dayDraftPhotos: this.data.dayPhotos.slice(),
    })
  },

  closeDayRate() { this.setData({ dayOpen: false }) },
  pickDayMood(event) { ui.haptic(); this.setData({ dayMood: event.currentTarget.dataset.mood }) },
  onDayRemark(event) { this.setData({ dayRemark: event.detail.value }) },

  async addDayPhotos() {
    const remaining = MAX_DAY_PHOTOS - this.data.dayDraftPhotos.length
    if (remaining <= 0) { ui.toast(`最多上传 ${MAX_DAY_PHOTOS} 张照片`); return }
    if (this.data.dayUploading) return
    const paths = await ui.chooseImages(remaining)
    if (!paths.length) return
    this.setData({ dayUploading: true })
    let failed = 0
    let lastError = ""
    for (const path of paths) {
      try {
        const raw = await api.upload(path)
        this.setData({ dayDraftPhotos: this.data.dayDraftPhotos.concat([{ raw, url: media.assetUrl(raw) }]) })
      } catch (error) {
        failed += 1
        lastError = error.message
      }
    }
    this.setData({ dayUploading: false })
    if (failed) ui.toast(lastError || `${failed} 张上传失败`)
  },

  removeDraftPhoto(event) {
    const index = Number(event.currentTarget.dataset.index)
    this.setData({ dayDraftPhotos: this.data.dayDraftPhotos.filter((_, i) => i !== index) })
  },

  async saveDayRate() {
    if (!this.data.dayMood || this.data.daySaving || !this.data.selectedKey) return
    this.setData({ daySaving: true })
    try {
      await api.post("/day-rating", {
        meal_date: this.data.selectedKey,
        mood: this.data.dayMood,
        remark: this.data.dayRemark.trim(),
        photos: JSON.stringify(this.data.dayDraftPhotos.map((photo) => photo.raw)),
      })
      ui.toast("今日评价已保存", "success")
      this.setData({ dayOpen: false })
      await this.loadMonth()
    } catch (error) {
      ui.toast(error.message || "评价保存失败")
    } finally {
      this.setData({ daySaving: false })
    }
  },

  async removeDayPhoto(event) {
    const index = Number(event.currentTarget.dataset.index)
    const ok = await ui.confirm({ title: "删除这张照片？", confirmText: "删除", danger: true })
    if (!ok) return
    const rating = this._ratings[this.data.selectedKey] || {}
    const photos = this.data.dayPhotos.filter((_, i) => i !== index).map((photo) => photo.raw)
    try {
      await api.post("/day-rating", { meal_date: this.data.selectedKey, mood: rating.mood || "", remark: rating.remark || "", photos: JSON.stringify(photos) })
      ui.toast("照片已删除", "success")
      await this.loadMonth()
    } catch (error) {
      ui.toast(error.message || "照片删除失败")
    }
  },

  previewDayPhoto(event) {
    const urls = this.data.dayPhotos.map((photo) => photo.url)
    ui.preview(urls, urls[Number(event.currentTarget.dataset.index) || 0])
  },

  previewDraftPhoto(event) {
    const urls = this.data.dayDraftPhotos.map((photo) => photo.url)
    ui.preview(urls, urls[Number(event.currentTarget.dataset.index) || 0])
  },

  goPhotoWall() { wx.navigateTo({ url: "/pages/photo-wall/photo-wall" }) },
  goHome() { wx.switchTab({ url: "/pages/home/home" }) },
})
