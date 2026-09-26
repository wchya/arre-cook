const api = require("../../utils/api")
const session = require("../../utils/session")
const ui = require("../../utils/ui")
const fmt = require("../../utils/format")

Page({
  data: {
    loading: true,
    list: [],
    unlocked: 0,
    total: 0,
    autoCount: 0,
    manualCount: 0,
    progress: 0,
    filter: "all",
    detail: null,
    detailOpen: false,
  },

  onLoad() {
    this._all = []
    if (!session.requireLogin("/pages/achievements/achievements")) return
    this.load()
  },

  async load() {
    try {
      const raw = await api.get("/achievements")
      const list = (Array.isArray(raw) ? raw : []).slice().sort((a, b) => (a.is_unlocked !== b.is_unlocked ? (a.is_unlocked ? -1 : 1) : a.id - b.id))
      this._all = list.map((item) => ({
        id: item.id,
        icon: item.icon || "🏅",
        name: item.name,
        desc: item.description || "",
        unlocked: Boolean(item.is_unlocked),
        auto: item.condition === "auto",
        date: item.unlocked_at ? fmt.monthDay(item.unlocked_at) : "",
        badge: item.is_unlocked ? `已解锁 ${item.unlocked_at ? fmt.monthDay(item.unlocked_at) : ""}` : item.condition === "auto" ? "自动检测" : "手动成就",
      }))
      const unlocked = this._all.filter((item) => item.unlocked).length
      const autoCount = this._all.filter((item) => item.auto).length
      this.setData({
        unlocked,
        total: this._all.length,
        autoCount,
        manualCount: this._all.length - autoCount,
        progress: this._all.length ? Math.round((unlocked / this._all.length) * 100) : 0,
      })
      this.applyFilter()
    } catch (error) {
      ui.toast(error.message || "成就加载失败")
    } finally {
      this.setData({ loading: false })
    }
  },

  applyFilter() {
    const filter = this.data.filter
    const list = this._all.filter((item) => filter === "all" || (filter === "done" ? item.unlocked : !item.unlocked))
    this.setData({ list })
  },

  chooseFilter(event) {
    ui.haptic()
    this.setData({ filter: event.currentTarget.dataset.key }, () => this.applyFilter())
  },

  openDetail(event) {
    const item = this._all.find((entry) => entry.id === Number(event.currentTarget.dataset.id))
    if (item) this.setData({ detail: item, detailOpen: true })
  },

  closeDetail() { this.setData({ detailOpen: false }) },

  onShareAppMessage() {
    return { title: `我在 ss-menu 解锁了 ${this.data.unlocked} 个美食成就 🏆`, path: "/pages/home/home" }
  },
})
