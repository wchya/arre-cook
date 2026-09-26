const api = require("../../utils/api")
const session = require("../../utils/session")
const ui = require("../../utils/ui")
const media = require("../../utils/media")
const dishUtil = require("../../utils/dish")
const fmt = require("../../utils/format")

const SORTS = [
  { key: "favorite", label: "收藏时间" },
  { key: "time", label: "最快上桌" },
  { key: "repeat", label: "常吃优先" },
  { key: "name", label: "名称" },
]

function countLabel(count) {
  if (!count) return "暂无收藏"
  if (count < 6) return "轻量收藏"
  if (count < 16) return "稳定菜单"
  return "私人菜库"
}

function time(value) {
  const d = fmt.parseDate(value)
  return d ? d.getTime() : 0
}

function toItem(entry) {
  const dish = entry.dish
  const card = dishUtil.toCard(dish)
  const tags = media.asArray(dish.tags).filter((tag) => typeof tag === "string")
  return {
    ...card,
    tag: tags[0] || "",
    records: entry.record_count || 0,
    lastEaten: entry.last_eaten_at || "",
    favoritedAt: entry.favorite_created_at || "",
    sub: entry.last_eaten_at ? `上次吃：${fmt.monthDay(entry.last_eaten_at)}` : `收藏于：${fmt.monthDay(entry.favorite_created_at)}`,
    search: [dish.name, dish.category, dish.taste, dish.remark].concat(tags).filter(Boolean).join(" ").toLowerCase(),
  }
}

function shelfItem(entry, repeat) {
  const card = dishUtil.toCard(entry.dish)
  return { ...card, meta: repeat ? `${entry.record_count || 0} 次` : `${entry.dish.cook_time || "-"} 分钟` }
}

Page({
  data: {
    loading: true,
    empty: false,
    stats: null,
    label: "",
    quickDish: null,
    mostDish: null,
    shelves: [],
    categories: [],
    category: "全部",
    sorts: SORTS,
    sort: "favorite",
    keyword: "",
    list: [],
    total: 0,
    removing: 0,
  },

  onLoad() {
    this._items = []
    if (!session.requireLogin("/pages/favorites/favorites")) return
  },

  onShow() {
    if (session.hasSession()) this.load()
  },

  async onPullDownRefresh() {
    await this.load()
    wx.stopPullDownRefresh()
  },

  async load() {
    try {
      const overview = await api.get("/favorites/overview")
      const items = overview.items || []
      const stats = overview.stats || null
      this._items = items.map(toItem)
      if (!items.length || !stats) {
        this.setData({ empty: true, loading: false, stats: null })
        return
      }
      const shelves = [
        { key: "quick", title: "快手收藏", meta: "赶时间先看这里", items: (overview.quick_picks || []).slice(0, 10).map((entry) => shelfItem(entry, false)) },
        { key: "repeat", title: "复做清单", meta: "已经验证过的口味", items: (overview.most_cooked || []).slice(0, 10).map((entry) => shelfItem(entry, true)) },
        { key: "try", title: "还没吃过", meta: "收藏后还没记录", items: (overview.need_try || []).slice(0, 10).map((entry) => shelfItem(entry, false)) },
      ].filter((shelf) => shelf.items.length)
      const categories = [{ name: "全部", count: items.length }].concat((overview.categories || []).map((cat) => ({ name: cat.category, count: cat.count })))
      this.setData({
        loading: false,
        empty: false,
        stats: {
          total: stats.total,
          categories: stats.category_total,
          cooked: stats.cooked_count,
          never: stats.never_cooked_count,
          avg: stats.avg_cook_time ? `${stats.avg_cook_time}m` : "-",
        },
        label: countLabel(stats.total),
        quickDish: stats.quick_dish ? { id: stats.quick_dish.id, name: stats.quick_dish.name, detail: `${stats.quick_dish.category || "其他"} · ${stats.quick_dish.cook_time || "-"} 分钟` } : null,
        mostDish: stats.most_cooked_dish ? { id: stats.most_cooked_dish.id, name: stats.most_cooked_dish.name, detail: stats.most_cooked_dish.category || "其他" } : null,
        shelves,
        categories,
      })
      this.applyFilter()
    } catch (error) {
      this.setData({ loading: false })
      ui.toast(error.message || "收藏加载失败")
    }
  },

  applyFilter() {
    const { category, sort } = this.data
    const keyword = this.data.keyword.trim().toLowerCase()
    const list = this._items
      .filter((item) => category === "全部" || (item.category || "其他") === category)
      .filter((item) => !keyword || item.search.indexOf(keyword) >= 0)
      .sort((a, b) => {
        if (sort === "time") return (a.cookTime || 9999) - (b.cookTime || 9999) || a.name.localeCompare(b.name, "zh-Hans-CN")
        if (sort === "repeat") return b.records - a.records || time(b.lastEaten) - time(a.lastEaten)
        if (sort === "name") return a.name.localeCompare(b.name, "zh-Hans-CN")
        return time(b.favoritedAt) - time(a.favoritedAt)
      })
    this.setData({ list, total: list.length })
  },

  chooseCategory(event) {
    ui.haptic()
    this.setData({ category: event.currentTarget.dataset.name }, () => this.applyFilter())
  },

  chooseSort(event) {
    ui.haptic()
    this.setData({ sort: event.currentTarget.dataset.key }, () => this.applyFilter())
  },

  onSearch(event) {
    this.setData({ keyword: event.detail.value }, () => this.applyFilter())
  },

  clearSearch() {
    this.setData({ keyword: "" }, () => this.applyFilter())
  },

  showAll() {
    this.setData({ category: "全部" }, () => this.applyFilter())
  },

  openDish(event) {
    wx.navigateTo({ url: `/pages/dish/dish?id=${event.currentTarget.dataset.id}` })
  },

  async unfavorite(event) {
    const id = Number(event.currentTarget.dataset.id)
    if (this.data.removing) return
    this.setData({ removing: id })
    ui.haptic()
    try {
      await api.delete(`/favorites/${id}`)
      ui.toast("已取消收藏")
      await this.load()
    } catch (error) {
      ui.toast(error.message || "取消收藏失败")
    } finally {
      this.setData({ removing: 0 })
    }
  },

  goDishes() { wx.switchTab({ url: "/pages/dishes/dishes" }) },
})
