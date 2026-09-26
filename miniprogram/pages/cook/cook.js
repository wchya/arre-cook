const api = require("../../utils/api")
const session = require("../../utils/session")
const ui = require("../../utils/ui")
const media = require("../../utils/media")
const dishUtil = require("../../utils/dish")
const fmt = require("../../utils/format")

Page({
  data: {
    id: 0,
    name: "",
    loading: true,
    steps: [],
    current: 0,
    progress: 0,
    running: false,
    remaining: 0,
    total: 0,
    ringPct: 0,
    clock: "0:00",
    listOpen: false,
    done: false,
  },

  onLoad(query) {
    const id = Number(query.id) || 0
    this.setData({ id, current: Math.max(0, Number(query.step) || 0) })
    if (!session.requireLogin(`/pages/cook/cook?id=${id}`)) return
    wx.setKeepScreenOn({ keepScreenOn: true, fail: () => {} })
    this.load()
  },

  onUnload() {
    this.stopTimer()
    wx.setKeepScreenOn({ keepScreenOn: false, fail: () => {} })
  },

  onHide() {
    // 切到后台时记住截止时间，回来后按真实流逝时间继续倒计时
    if (this.data.running) this._hiddenAt = Date.now()
  },

  onShow() {
    if (this._hiddenAt && this.data.running) {
      const passed = Math.floor((Date.now() - this._hiddenAt) / 1000)
      this._hiddenAt = 0
      this.tick(passed)
    }
  },

  async load() {
    try {
      const dish = await api.get(`/dishes/${this.data.id}`)
      const steps = dishUtil.normalizeSteps(dish.steps).map((step, index) => ({
        index,
        text: step.text,
        time: step.time,
        image: media.assetUrl(step.image),
      }))
      const current = Math.min(this.data.current, Math.max(0, steps.length - 1))
      this.setData({ name: dish.name, steps, loading: false, current })
      this.prepareStep(current)
    } catch (error) {
      this.setData({ loading: false })
      ui.toast(error.message || "菜谱加载失败")
    }
  },

  prepareStep(index) {
    const step = this.data.steps[index]
    const total = step && step.time ? step.time * 60 : 0
    this.stopTimer()
    this.setData({
      current: index,
      progress: this.data.steps.length ? Math.round(((index + 1) / this.data.steps.length) * 100) : 0,
      total,
      remaining: total,
      running: false,
      ringPct: 0,
      clock: fmt.clock(total),
    })
  },

  onSwiperChange(event) {
    if (event.detail.source !== "touch") return
    ui.haptic()
    this.prepareStep(event.detail.current)
  },

  prev() {
    if (this.data.current <= 0) return
    ui.haptic()
    this.prepareStep(this.data.current - 1)
  },

  next() {
    if (this.data.current >= this.data.steps.length - 1) {
      this.finish()
      return
    }
    ui.haptic()
    this.prepareStep(this.data.current + 1)
  },

  jumpTo(event) {
    const index = Number(event.currentTarget.dataset.index)
    this.setData({ listOpen: false })
    this.prepareStep(index)
  },

  openList() { this.setData({ listOpen: true }) },
  closeList() { this.setData({ listOpen: false }) },

  // ---------- 计时 ----------

  toggleTimer() {
    if (!this.data.total) {
      ui.toast("这一步没有计时，完成后点“下一步”")
      return
    }
    ui.haptic("medium")
    if (this.data.running) {
      this.stopTimer()
      this.setData({ running: false })
      return
    }
    const remaining = this.data.remaining > 0 ? this.data.remaining : this.data.total
    this.setData({ running: true, remaining })
    this._timer = setInterval(() => this.tick(1), 1000)
  },

  resetTimer() {
    if (!this.data.total) return
    this.stopTimer()
    this.setData({ running: false, remaining: this.data.total, ringPct: 0, clock: fmt.clock(this.data.total) })
  },

  tick(seconds) {
    const remaining = Math.max(0, this.data.remaining - seconds)
    const total = this.data.total || 1
    this.setData({ remaining, clock: fmt.clock(remaining), ringPct: Math.round(((total - remaining) / total) * 1000) / 10 })
    if (remaining > 0) return
    this.stopTimer()
    this.setData({ running: false })
    wx.vibrateLong({ fail: () => {} })
    ui.toast("⏰ 时间到！该进行下一步了~")
    clearTimeout(this._advanceTimer)
    this._advanceTimer = setTimeout(() => {
      if (this.data.current < this.data.steps.length - 1) this.prepareStep(this.data.current + 1)
    }, 3000)
  },

  stopTimer() {
    clearInterval(this._timer)
    clearTimeout(this._advanceTimer)
    this._timer = null
  },

  // ---------- 完成 ----------

  async finish() {
    this.stopTimer()
    this.setData({ done: true, running: false })
    wx.vibrateShort({ type: "heavy", fail: () => {} })
    const index = await ui.actionSheet(["记为今天午餐", "记为今天晚餐"], "🎉 全部步骤完成！记一笔吗？")
    if (index < 0) return
    const meal = index === 0 ? "lunch" : "dinner"
    try {
      await api.post("/records", { dish_id: this.data.id, dish_name: this.data.name, meal_type: meal, meal_date: fmt.dateKey() })
      api.behavior({ event_type: "accept", dish_id: this.data.id, dish_name: this.data.name, meta: { from: "cook_mode", meal_type: meal, client: "miniprogram" } })
      ui.toast("❤ 已记录！", "success")
    } catch (error) {
      ui.toast(error.message || "记录失败")
    }
  },

  previewImage(event) {
    ui.preview([event.currentTarget.dataset.url])
  },

  close() {
    this.stopTimer()
    if (getCurrentPages().length > 1) wx.navigateBack()
    else wx.redirectTo({ url: `/pages/dish/dish?id=${this.data.id}` })
  },
})
