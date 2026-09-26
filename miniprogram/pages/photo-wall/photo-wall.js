const api = require("../../utils/api")
const ui = require("../../utils/ui")
const media = require("../../utils/media")
const session = require("../../utils/session")
const dishUtil = require("../../utils/dish")

const MOOD_EMOJI = dishUtil.MOOD_EMOJI
const MOOD_LABEL = { yum: "好吃", ok: "一般", no: "不想再吃", great: "超满足", meh: "还行吧" }
const DAY_MOOD_LABEL = { great: "超满足的一天", ok: "还行的一天", meh: "不太行的一天" }
const WEEKDAYS = ["周日", "周一", "周二", "周三", "周四", "周五", "周六"]
// 给每张照片一个稳定的轻微倾斜，营造手贴照片的回忆感。
const TILTS = [-2.5, 1.8, -1.2, 2.2, -2, 1.4, -1.6, 2.6]

Page({
  data: { loading: true, days: [], totalDays: 0, totalPhotos: 0 },

  onLoad() {
    if (!session.requireLogin("/pages/photo-wall/photo-wall")) return
    this.load()
  },

  onShow() {
    if (this._loadedOnce) this.load()
  },

  async onPullDownRefresh() {
    await this.load()
    wx.stopPullDownRefresh()
  },

  async load() {
    if (this._loading) return
    this._loading = true
    this.setData({ loading: !this.data.days.length })
    try {
      const data = (await api.get("/photo-wall")) || {}
      const days = (data.days || []).map((day) => this.mapDay(day)).filter(Boolean)
      this.setData({ days, totalDays: data.total_days || days.length, totalPhotos: data.total_photos || 0 })
      this._loadedOnce = true
    } catch (error) {
      ui.toast(error.message || "照片墙暂时无法加载")
    } finally {
      this._loading = false
      this.setData({ loading: false })
    }
  },

  mapDay(day) {
    const photos = media.photoList(day.photos).map((url) => media.assetUrl(url)).filter(Boolean)
    if (!photos.length) return null
    const parts = this.dateParts(day.meal_date)
    const shown = Math.min(4, photos.length)
    const cover = photos.slice(0, shown).map((url, i) => ({
      url,
      tilt: TILTS[(String(day.meal_date).length + i) % TILTS.length],
      overlay: i === shown - 1 && photos.length > 4 ? photos.length - 4 : 0,
    }))
    const homeMood = dishUtil.HOME_MOOD_MAP[day.home_mood] || null
    const records = (day.records || []).map((r) => ({
      dishId: r.dish_id,
      dishName: r.dish_name,
      mealLabel: r.meal_type === "lunch" ? "午餐" : "晚餐",
      isLunch: r.meal_type === "lunch",
      moodEmoji: MOOD_EMOJI[r.mood] || "",
      moodLabel: MOOD_LABEL[r.mood] || "",
      hasMood: Boolean(r.mood),
    }))
    return {
      key: day.meal_date,
      photos,
      cover,
      single: cover.length === 1,
      month: parts.month,
      day: parts.day,
      year: parts.year,
      weekday: parts.weekday,
      homeMoodEmoji: homeMood ? homeMood.emoji : "",
      homeMoodLabel: homeMood ? homeMood.label : "",
      dayMoodEmoji: day.day_mood ? MOOD_EMOJI[day.day_mood] || "🍚" : "",
      dayMoodLabel: day.day_mood ? DAY_MOOD_LABEL[day.day_mood] || "记录" : "",
      dayRemark: day.day_remark || "",
      records,
    }
  },

  dateParts(dateStr) {
    const [y, m, d] = String(dateStr || "").split("-").map(Number)
    let weekday = ""
    if (y && m && d) weekday = WEEKDAYS[new Date(y, m - 1, d).getDay()] || ""
    return {
      year: y ? String(y) : "",
      month: m ? String(m).padStart(2, "0") : "",
      day: d ? String(d).padStart(2, "0") : "",
      weekday,
    }
  },

  previewPhoto(event) {
    const { di, pi } = event.currentTarget.dataset
    const day = this.data.days[di]
    if (!day) return
    ui.preview(day.photos, day.photos[pi])
  },

  openDish(event) {
    const id = event.currentTarget.dataset.id
    if (id) wx.navigateTo({ url: `/pages/dish/dish?id=${id}` })
  },

  goHistory() { wx.switchTab({ url: "/pages/history/history" }) },
})
