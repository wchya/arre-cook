const api = require("../../utils/api")
const SESSION_KEY = "ninimenu_session"

function todayKey() {
  const date = new Date()
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`
}

function greeting() {
  const hour = new Date().getHours()
  if (hour < 6) return "夜深了"
  if (hour < 11) return "早上好"
  if (hour < 14) return "中午好"
  if (hour < 18) return "下午好"
  return "晚上好"
}

Page({
  data: {
    greeting: greeting(),
    today: todayKey(),
    user: null,
    items: [],
    todayRecords: [],
    loading: true,
    refreshing: false,
    error: "",
    recordingId: 0,
  },

  onShow() {
    if (!wx.getStorageSync(SESSION_KEY)) {
      wx.reLaunch({ url: "/pages/login/login" })
      return
    }
    if (!this.data.loading || !this._started) this.load()
  },

  onPullDownRefresh() {
    this.setData({ refreshing: true })
    this.load().finally(() => {
      this.setData({ refreshing: false })
      wx.stopPullDownRefresh()
    })
  },

  async load() {
    if (this._loading) return
    this._loading = true
    this._started = true
    this.setData({ loading: true, error: "" })
    const date = todayKey()
    const results = await Promise.allSettled([
      api.get("/me"),
      api.post("/pick/smart", { count: 3, mode: "home_auto" }),
      api.get("/records", { date_from: date, date_to: date, pageSize: 8 }),
    ])
    const [userResult, pickResult, recordsResult] = results
    if (userResult.status === "fulfilled") this.setData({ user: userResult.value })
    if (pickResult.status === "fulfilled") this.setData({ items: pickResult.value.items || [] })
    if (recordsResult.status === "fulfilled") this.setData({ todayRecords: recordsResult.value.items || [] })
    const failures = results.filter((result) => result.status === "rejected")
    if (failures.length === results.length) this.setData({ error: "暂时无法加载，请下拉重试" })
    else if (pickResult.status === "rejected") this.setData({ error: "推荐暂不可用，可以稍后重试" })
    this.setData({ today: date, greeting: greeting(), loading: false })
    this._loading = false
  },

  async refreshRecommendations() {
    if (this.data.refreshing) return
    this.setData({ refreshing: true, error: "" })
    try {
      const result = await api.post("/pick/smart", { count: 3, mode: "home_auto" })
      this.setData({ items: result.items || [] })
    } catch (error) {
      this.setData({ error: error.message || "推荐暂不可用" })
    } finally {
      this.setData({ refreshing: false })
    }
  },

  async recordDish(event) {
    const dishId = Number(event.currentTarget.dataset.id)
    const item = this.data.items.find((entry) => entry.dish.id === dishId)
    if (!item || this.data.recordingId) return
    const result = await new Promise((resolve) => wx.showActionSheet({
      itemList: ["记为午餐", "记为晚餐"],
      success: (choice) => resolve(choice.tapIndex === 0 ? "lunch" : "dinner"),
      fail: () => resolve(""),
    }))
    if (!result) return
    this.setData({ recordingId: dishId })
    try {
      await api.post("/records", { dish_id: dishId, dish_name: item.dish.name, meal_type: result, meal_date: todayKey() })
      wx.showToast({ title: "已记下这餐", icon: "success" })
      const records = await api.get("/records", { date_from: todayKey(), date_to: todayKey(), pageSize: 8 })
      this.setData({ todayRecords: records.items || [] })
    } catch (error) {
      wx.showToast({ title: error.message || "记录失败", icon: "none" })
    } finally {
      this.setData({ recordingId: 0 })
    }
  },
})
