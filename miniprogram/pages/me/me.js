const api = require("../../utils/api")
const session = require("../../utils/session")
const ui = require("../../utils/ui")
const media = require("../../utils/media")

const REPEAT_OPTIONS = ["1", "2", "3", "5", "7", "10", "14", "0"]

Page({
  data: {
    topPad: 64,
    user: null,
    avatar: "",
    initial: "我",
    stats: null,
    unlocked: 0,
    achievementTotal: 0,
    unread: 0,
    pending: 0,
    voiceOn: true,
    blindOn: true,
    repeatDays: "",
    repeatOptions: REPEAT_OPTIONS,
    nameOpen: false,
    nickname: "",
    savingName: false,
    repeatOpen: false,
    uploadingAvatar: false,
    refreshing: false,
  },

  onLoad() {
    const nav = getApp().navMetrics()
    this.setData({ topPad: nav.statusBarHeight + 24 })
  },

  onShow() {
    session.syncTabBar(this, 3)
    if (!session.requireLogin()) return
    this.load()
  },

  async onRefresh() {
    this.setData({ refreshing: true })
    await this.load(true)
    this.setData({ refreshing: false })
  },

  async load(force) {
    const results = await Promise.allSettled([
      session.currentUser(force || !this._loaded),
      api.get("/stats"),
      api.get("/achievements"),
      api.get("/notifications", { pageSize: 1 }),
      api.get("/suggestions", { status: "pending" }),
      api.get("/settings"),
    ])
    this._loaded = true
    const [user, stats, achievements, notifications, suggestions, settings] = results
    if (user.status === "fulfilled") this.applyUser(user.value)
    if (stats.status === "fulfilled") this.setData({ stats: stats.value })
    if (achievements.status === "fulfilled") {
      const list = Array.isArray(achievements.value) ? achievements.value : []
      this.setData({ unlocked: list.filter((item) => item.is_unlocked).length, achievementTotal: list.length })
    }
    if (notifications.status === "fulfilled") this.setData({ unread: notifications.value.unread || 0 })
    if (suggestions.status === "fulfilled") this.setData({ pending: (suggestions.value || []).length })
    if (settings.status === "fulfilled") this.applySettings(settings.value || {})
  },

  applyUser(user) {
    if (!user) return
    this.setData({
      user,
      avatar: media.assetUrl(user.avatar),
      initial: (user.nickname || user.username || "我").slice(0, 1).toUpperCase(),
    })
  },

  applySettings(settings) {
    this.setData({
      voiceOn: String(settings.voice_enabled || "1") !== "0",
      blindOn: String(settings.blind_box_enabled || "1") !== "0",
      repeatDays: settings.repeat_days ? String(settings.repeat_days) : "",
    })
  },

  // ---------- 头像 / 昵称 ----------

  async onChooseAvatar(event) {
    const path = event.detail && event.detail.avatarUrl
    if (path) await this.uploadAvatar(path)
  },

  async uploadAvatar(path) {
    if (this.data.uploadingAvatar) return
    this.setData({ uploadingAvatar: true })
    try {
      const raw = await api.upload(path)
      const user = await api.put("/me", { avatar: raw })
      session.setUser(user)
      this.applyUser(user)
      ui.toast("头像已更新", "success")
    } catch (error) {
      ui.toast(error.message || "头像上传失败")
    } finally {
      this.setData({ uploadingAvatar: false })
    }
  },

  openName() {
    const user = this.data.user
    this.setData({ nameOpen: true, nickname: user ? user.nickname || "" : "" })
  },

  closeName() { this.setData({ nameOpen: false }) },
  onNameInput(event) { this.setData({ nickname: event.detail.value }) },

  async saveName() {
    const nickname = this.data.nickname.trim()
    if (!nickname || this.data.savingName) return
    this.setData({ savingName: true })
    try {
      const user = await api.put("/me", { nickname })
      session.setUser(user)
      this.applyUser(user)
      this.setData({ nameOpen: false })
      ui.toast("已更新", "success")
    } catch (error) {
      ui.toast(error.message || "保存失败")
    } finally {
      this.setData({ savingName: false })
    }
  },

  // ---------- 偏好设置 ----------

  async updateSetting(patch, rollback) {
    try {
      await api.put("/settings", { settings: patch })
    } catch (error) {
      this.setData(rollback)
      ui.toast(error.message || "保存失败")
    }
  },

  toggleVoice() {
    const next = !this.data.voiceOn
    ui.haptic()
    this.setData({ voiceOn: next })
    this.updateSetting({ voice_enabled: next ? "1" : "0" }, { voiceOn: !next })
  },

  toggleBlind() {
    const next = !this.data.blindOn
    ui.haptic()
    this.setData({ blindOn: next })
    this.updateSetting({ blind_box_enabled: next ? "1" : "0" }, { blindOn: !next })
  },

  openRepeat() { this.setData({ repeatOpen: true }) },
  closeRepeat() { this.setData({ repeatOpen: false }) },

  chooseRepeat(event) {
    const value = event.currentTarget.dataset.value
    const previous = this.data.repeatDays
    const next = value === "0" ? "" : value
    ui.haptic()
    this.setData({ repeatDays: next, repeatOpen: false })
    this.updateSetting({ repeat_days: next }, { repeatDays: previous })
  },

  // ---------- 导航 ----------

  go(event) {
    const url = event.currentTarget.dataset.url
    if (url) wx.navigateTo({ url })
  },

  adminTip() {
    ui.confirm({ title: "管理后台", content: "菜谱、用户、AI 模型与站点设置请在网页端（cook.arrebyte.top）管理。", showCancel: false, confirmText: "知道了" })
  },

  async logout() {
    const ok = await ui.confirm({ title: "退出当前账号？", content: "退出后需要重新登录才能查看饮食数据。", confirmText: "退出", danger: true })
    if (ok) session.logout("logout")
  },
})
