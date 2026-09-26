const api = require("../../utils/api")
const session = require("../../utils/session")

function weightList(items) {
  const list = (items || []).slice(0, 6)
  const max = Math.max(1, ...list.map((item) => item.weight || 0))
  return list.map((item) => ({ name: item.name, count: item.count || 0, width: Math.max(4, ((item.weight || 0) / max) * 100) }))
}

Page({
  data: { loading: true, error: false, profile: null, tastes: [], categories: [], topDishes: [], disliked: [] },

  onLoad() {
    if (!session.requireLogin("/pages/taste-profile/taste-profile")) return
    this.load()
  },

  async load() {
    try {
      const profile = await api.get("/profile", { days: 90 })
      this.setData({
        profile: {
          windowDays: profile.window_days,
          summary: profile.summary || "继续记录每一餐，画像会逐渐贴近你的口味。",
          records: profile.window_records || 0,
          distinct: profile.distinct_dishes || 0,
          avg: profile.avg_cook_time > 0 ? `${Math.round(profile.avg_cook_time)} 分` : "-",
          spicy: Math.round((profile.spicy_ratio || 0) * 100),
          repeatDays: profile.repeat_days || 0,
        },
        tastes: weightList(profile.taste_weights),
        categories: weightList(profile.category_weights),
        topDishes: (profile.top_dishes || []).slice(0, 5).map((dish) => ({ id: dish.dish_id, name: dish.dish_name, count: dish.count })),
        disliked: (profile.disliked_dishes || []).map((dish) => ({ id: dish.id, name: dish.name })),
      })
    } catch (_) {
      this.setData({ error: true })
    } finally {
      this.setData({ loading: false })
    }
  },

  openDish(event) {
    wx.navigateTo({ url: `/pages/dish/dish?id=${event.currentTarget.dataset.id}` })
  },

  goPreferences() { wx.navigateTo({ url: "/pages/preferences/preferences" }) },
  goAssistant() { wx.navigateTo({ url: "/pages/assistant/assistant" }) },
})
