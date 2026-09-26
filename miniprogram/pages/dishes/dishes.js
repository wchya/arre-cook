const api = require("../../utils/api")
const session = require("../../utils/session")
const ui = require("../../utils/ui")
const media = require("../../utils/media")
const dishUtil = require("../../utils/dish")
const fmt = require("../../utils/format")

// 与 Web 端 DishList 默认值一致；站点设置里配置了分类 / 口味时以设置为准。
const DEFAULT_CATEGORIES = ["川菜", "湘菜", "贵州菜", "云南菜", "粤菜"]
const DEFAULT_TASTES = ["辣", "麻辣", "香辣", "酸辣", "鲜辣", "酸", "甜", "鲜", "清淡", "咸鲜", "蒜香", "酱香"]
const SCOPES = [
  { key: "", label: "全部菜谱", short: "全部" },
  { key: "mine", label: "我的私房菜", short: "私房" },
  { key: "family", label: "家庭菜谱", short: "家庭" },
  { key: "public", label: "公共菜谱", short: "公共" },
]
const PAGE_SIZE = 30

function stringList(raw, fallback) {
  const list = media.asArray(raw).filter((item) => typeof item === "string" && item)
  return list.length ? list : fallback
}

Page({
  data: {
    categories: [{ name: "全部", count: 0 }],
    tastes: ["全部"],
    scope: "",
    scopeLabel: "全部",
    category: "全部",
    taste: "全部",
    keyword: "",
    searchFocus: false,

    groups: [],
    grouped: true,
    total: 0,
    loaded: 0,
    loading: true,
    loadingMore: false,
    hasMore: false,
    refreshing: false,
    error: "",

    scrollTop: 0,
    scrollIntoView: "",
    highlight: "",

    lunchIds: {},
    dinnerIds: {},
    lunchCount: 0,
    dinnerCount: 0,
    todayOpen: false,
    todayLunch: [],
    todayDinner: [],
    lunchMinutes: 0,
    dinnerMinutes: 0,
    pending: {},
  },

  onLoad() {
    this._page = 1
    this._items = []
    this._dishMap = {}
    this._requestId = 0
  },

  onShow() {
    session.syncTabBar(this, 1)
    if (!session.requireLogin()) return
    if (!this._booted) {
      this._booted = true
      this.loadMeta()
      this.reload()
    } else {
      this.loadToday()
    }
  },

  onShareAppMessage() {
    return { title: "ss-menu · 选菜加餐，一键搞定", path: "/pages/dishes/dishes" }
  },

  // ---------- 数据 ----------

  async loadMeta() {
    const [settings, counts] = await Promise.all([
      api.get("/settings").catch(() => ({})),
      api.get("/dishes/category-counts").catch(() => null),
    ])
    const countMap = {}
    if (counts && counts.categories) counts.categories.forEach((item) => { countMap[item.category] = item.count })
    const names = stringList(settings && settings.categories, DEFAULT_CATEGORIES)
    const categories = [{ name: "全部", count: counts ? counts.total : 0 }].concat(names.map((name) => ({ name, count: countMap[name] || 0 })))
    this._countMap = countMap
    this.setData({ categories, tastes: ["全部"].concat(stringList(settings && settings.tastes, DEFAULT_TASTES)) })
  },

  params(page) {
    const params = { page, pageSize: PAGE_SIZE, sort: "sort_order", order: "asc", enabled: "true" }
    if (this.data.category !== "全部") params.category = this.data.category
    if (this.data.taste !== "全部") params.taste = this.data.taste
    if (this.data.keyword) params.search = this.data.keyword
    if (this.data.scope) params.scope = this.data.scope
    return params
  },

  async reload() {
    const requestId = ++this._requestId
    this._page = 1
    this.setData({ loading: true, error: "", scrollTop: this.data.scrollTop === 0 ? 0.01 : 0 })
    try {
      const [result] = await Promise.all([api.get("/dishes", this.params(1)), this.loadToday()])
      if (requestId !== this._requestId) return
      this._items = result.items || []
      this.render(result.total || 0)
    } catch (error) {
      if (requestId !== this._requestId) return
      this._items = []
      this.render(0)
      this.setData({ error: error.message || "菜谱加载失败，下拉重试" })
    } finally {
      if (requestId === this._requestId) this.setData({ loading: false })
    }
  },

  async loadMore() {
    if (this.data.loadingMore || !this.data.hasMore || this.data.loading) return
    const requestId = this._requestId
    this.setData({ loadingMore: true })
    try {
      const page = this._page + 1
      const result = await api.get("/dishes", this.params(page))
      if (requestId !== this._requestId) return
      this._page = page
      this._items = this._items.concat(result.items || [])
      this.render(result.total || 0)
    } catch (_) {
      ui.toast("加载更多失败")
    } finally {
      this.setData({ loadingMore: false })
    }
  },

  render(total) {
    const grouped = this.data.category === "全部" && this.data.taste === "全部" && !this.data.keyword
    const groups = []
    const index = {}
    this._items.forEach((raw) => {
      const card = dishUtil.toCard(raw)
      this._dishMap[card.id] = card
      const name = grouped ? card.category || "其他" : "result"
      if (index[name] === undefined) {
        index[name] = groups.length
        groups.push({ name, anchor: `cat-${groups.length}`, count: grouped ? (this._countMap && this._countMap[name]) || 0 : total, items: [] })
      }
      groups[index[name]].items.push(card)
    })
    groups.forEach((group) => { if (!group.count) group.count = group.items.length })
    this._anchors = groups.map((group) => group.anchor)
    this.setData({ groups, grouped, total, loaded: this._items.length, hasMore: this._items.length < total })
    if (grouped) setTimeout(() => this.measureSections(), 80)
  },

  async loadToday() {
    const today = fmt.dateKey()
    try {
      const result = await api.get("/records", { date_from: today, date_to: today, pageSize: 100 })
      this.applyToday(result.items || [])
    } catch (_) { /* ignore */ }
  },

  applyToday(records) {
    const lunchIds = {}
    const dinnerIds = {}
    const todayLunch = []
    const todayDinner = []
    records.forEach((record) => {
      const cached = this._dishMap[record.dish_id]
      const entry = {
        id: record.id,
        dishId: record.dish_id,
        name: record.dish_name,
        cover: cached ? cached.cover : media.assetUrl(record.dish_image_url),
        emoji: cached ? cached.emoji : record.dish_emoji || "🍽",
        meta: cached ? [cached.category, cached.cookTime ? `${cached.cookTime}分钟` : "", cached.diffLabel].filter(Boolean).join(" · ") : "",
        cookTime: cached ? cached.cookTime : 0,
      }
      if (record.meal_type === "lunch") { lunchIds[record.dish_id] = record.id; todayLunch.push(entry) }
      if (record.meal_type === "dinner") { dinnerIds[record.dish_id] = record.id; todayDinner.push(entry) }
    })
    const sum = (list) => list.reduce((total, item) => total + (item.cookTime || 0), 0)
    this._todayRecords = records
    this.setData({
      lunchIds, dinnerIds, todayLunch, todayDinner,
      lunchCount: todayLunch.length,
      dinnerCount: todayDinner.length,
      lunchMinutes: sum(todayLunch),
      dinnerMinutes: sum(todayDinner),
    })
  },

  // ---------- 分类侧栏联动 ----------

  measureSections() {
    if (!this.data.grouped) return
    const query = this.createSelectorQuery()
    query.select(".list").boundingClientRect()
    query.select(".list").scrollOffset()
    query.selectAll(".group").boundingClientRect()
    query.exec((res) => {
      const [box, offset, sections] = res || []
      if (!box || !offset || !sections) return
      this._sectionTops = sections.map((rect, i) => ({ top: rect.top - box.top + offset.scrollTop, name: this.data.groups[i] ? this.data.groups[i].name : "" }))
    })
  },

  onListScroll(event) {
    if (!this.data.grouped || !this._sectionTops || this._jumping) return
    const top = event.detail.scrollTop + 60
    let current = ""
    for (let i = 0; i < this._sectionTops.length; i++) {
      if (this._sectionTops[i].top <= top) current = this._sectionTops[i].name
    }
    if (current !== this.data.highlight) this.setData({ highlight: current })
  },

  selectCategory(event) {
    const name = event.currentTarget.dataset.name
    ui.haptic()
    if (this.data.grouped && name !== "全部") {
      const group = this.data.groups.find((item) => item.name === name)
      if (group) {
        this._jumping = true
        this.setData({ scrollIntoView: group.anchor, highlight: name })
        setTimeout(() => { this._jumping = false; this.setData({ scrollIntoView: "" }) }, 400)
        return
      }
    }
    if (name === this.data.category) {
      this.setData({ scrollTop: this.data.scrollTop === 0 ? 0.01 : 0 })
      return
    }
    this.setData({ category: name, keyword: "", highlight: "" }, () => this.reload())
  },

  selectTaste(event) {
    const taste = event.currentTarget.dataset.taste
    if (taste === this.data.taste) return
    ui.haptic()
    this.setData({ taste }, () => this.reload())
  },

  async chooseScope() {
    const index = await ui.actionSheet(SCOPES.map((item) => item.label))
    if (index < 0) return
    const scope = SCOPES[index].key
    if (scope === this.data.scope) return
    this.setData({ scope, scopeLabel: SCOPES[index].short, category: "全部", taste: "全部", keyword: "" }, () => this.reload())
  },

  // ---------- 搜索 ----------

  onSearchInput(event) {
    const keyword = String(event.detail.value || "").trim()
    clearTimeout(this._searchTimer)
    this._searchTimer = setTimeout(() => {
      if (keyword === this.data.keyword) return
      this.setData({ keyword, category: keyword ? "全部" : this.data.category, taste: keyword ? "全部" : this.data.taste }, () => this.reload())
    }, 320)
  },

  onSearchFocus() { this.setData({ searchFocus: true }) },
  onSearchBlur() { this.setData({ searchFocus: false }) },

  clearSearch() {
    clearTimeout(this._searchTimer)
    this.setData({ keyword: "" }, () => this.reload())
  },

  async onRefresh() {
    this.setData({ refreshing: true })
    await Promise.all([this.loadMeta(), this.reload()])
    this.setData({ refreshing: false })
  },

  // ---------- 行内操作 ----------

  openDish(event) {
    wx.navigateTo({ url: `/pages/dish/dish?id=${event.currentTarget.dataset.id}` })
  },

  async toggleFav(event) {
    const id = Number(event.currentTarget.dataset.id)
    const card = this._dishMap[id]
    if (!card || this._favBusy) return
    this._favBusy = true
    const active = card.favorite
    this.patchDish(id, { favorite: !active })
    ui.haptic(active ? "light" : "medium")
    try {
      if (active) await api.delete(`/favorites/${id}`)
      else await api.post(`/favorites/${id}`)
      ui.toast(active ? "已取消收藏" : "❤ 已收藏")
    } catch (error) {
      this.patchDish(id, { favorite: active })
      ui.toast(error.message || "操作失败")
    } finally {
      this._favBusy = false
    }
  },

  patchDish(id, patch) {
    Object.assign(this._dishMap[id], patch)
    const updates = {}
    this.data.groups.forEach((group, gi) => group.items.forEach((item, ii) => {
      if (item.id === id) Object.keys(patch).forEach((key) => { updates[`groups[${gi}].items[${ii}].${key}`] = patch[key] })
    }))
    this._items.forEach((raw) => { if (raw.id === id && patch.favorite !== undefined) raw.favorite = patch.favorite })
    this.setData(updates)
  },

  // 午餐 / 晚餐切换：已加入则删除当天记录，未加入则新增。对应 Web 端 MealToggle。
  async toggleMeal(event) {
    const id = Number(event.currentTarget.dataset.id)
    const meal = event.currentTarget.dataset.meal
    const card = this._dishMap[id]
    const pendingKey = `${meal}${id}`
    if (!card || this.data.pending[pendingKey]) return
    const recordId = (meal === "lunch" ? this.data.lunchIds : this.data.dinnerIds)[id]
    this.setData({ [`pending.${pendingKey}`]: true })
    ui.haptic()
    try {
      if (recordId) {
        await api.delete(`/records/${recordId}`)
      } else {
        await api.post("/records", { dish_id: id, dish_name: card.name, meal_type: meal, meal_date: fmt.dateKey() })
        ui.toast(`❤ 已加入${meal === "lunch" ? "午餐" : "晚餐"}`)
      }
      await this.loadToday()
    } catch (error) {
      ui.toast(error.message || "操作失败")
    } finally {
      this.setData({ [`pending.${pendingKey}`]: false })
    }
  },

  openToday() { this.setData({ todayOpen: true }) },
  closeToday() { this.setData({ todayOpen: false }) },

  openTodayDish(event) {
    this.setData({ todayOpen: false })
    wx.navigateTo({ url: `/pages/dish/dish?id=${event.currentTarget.dataset.id}` })
  },

  async removeToday(event) {
    const id = Number(event.currentTarget.dataset.id)
    try {
      await api.delete(`/records/${id}`)
      await this.loadToday()
      if (!this.data.lunchCount && !this.data.dinnerCount) this.setData({ todayOpen: false })
    } catch (error) {
      ui.toast(error.message || "删除失败")
    }
  },

  createDish() {
    wx.navigateTo({ url: "/pages/dish-edit/dish-edit" })
  },
})
