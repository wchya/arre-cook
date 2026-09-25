const api = require("../../utils/api")
const SESSION_KEY = "ninimenu_session"

const MEALS = [
  { value: "breakfast", label: "早餐" },
  { value: "lunch", label: "午餐" },
  { value: "dinner", label: "晚餐" },
  { value: "snack", label: "加餐" },
]
const GROUPS = [
  { value: "vegetable", label: "蔬菜" },
  { value: "fruit", label: "水果" },
  { value: "protein", label: "蛋白质" },
  { value: "whole_grain", label: "全谷物" },
  { value: "dairy", label: "奶类" },
]

function todayKey() {
  const date = new Date()
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`
}

Page({
  data: {
    view: "journal",
    days: 7,
    meals: MEALS,
    groups: GROUPS.map((group) => ({ ...group, checked: false })),
    mealIndex: 1,
    mealDate: todayKey(),
    dishName: "",
    cuisine: "",
    notes: "",
    foodGroups: [],
    entries: [],
    report: null,
    loading: true,
    saving: false,
    showForm: false,
    error: "",
  },

  onShow() {
    if (!wx.getStorageSync(SESSION_KEY)) {
      wx.reLaunch({ url: "/pages/login/login" })
      return
    }
    this.load()
  },

  async load() {
    if (this._loading) return
    this._loading = true
    this.setData({ loading: true, error: "" })
    const date = todayKey()
    const from = `${date.slice(0, 8)}01`
    try {
      const [entries, report] = await Promise.all([
        api.get("/food-journal", { from, to: date }),
        api.get("/health-report", { days: this.data.days }),
      ])
      this.setData({ entries: entries || [], report: report || null })
    } catch (error) {
      this.setData({ error: error.message || "饮食数据暂时无法加载" })
    } finally {
      this._loading = false
      this.setData({ loading: false })
    }
  },

  switchView(event) {
    const view = event.currentTarget.dataset.view
    if (view !== "journal" && view !== "report") return
    this.setData({ view }, () => this.load())
  },

  choosePeriod(event) {
    const days = Number(event.currentTarget.dataset.days)
    if (days !== 7 && days !== 30) return
    this.setData({ days }, () => this.load())
  },

  showNewEntry() {
    this.setData({ showForm: true, mealDate: todayKey(), mealIndex: 1, dishName: "", cuisine: "", notes: "", foodGroups: [], groups: GROUPS.map((group) => ({ ...group, checked: false })), error: "" })
  },

  onMealChange(event) { this.setData({ mealIndex: Number(event.detail.value) }) },
  onDateChange(event) { this.setData({ mealDate: event.detail.value }) },
  onDishInput(event) { this.setData({ dishName: event.detail.value }) },
  onCuisineInput(event) { this.setData({ cuisine: event.detail.value }) },
  onNotesInput(event) { this.setData({ notes: event.detail.value }) },
  onFoodGroupsChange(event) {
    const foodGroups = event.detail.value || []
    this.setData({ foodGroups, groups: GROUPS.map((group) => ({ ...group, checked: foodGroups.includes(group.value) })) })
  },

  async saveEntry() {
    if (this.data.saving) return
    if (!this.data.dishName.trim()) {
      this.setData({ error: "请填写这餐吃了什么" })
      return
    }
    this.setData({ saving: true, error: "" })
    try {
      await api.post("/food-journal", {
        meal_date: this.data.mealDate,
        meal_type: MEALS[this.data.mealIndex].value,
        dish_name: this.data.dishName.trim(),
        cuisine: this.data.cuisine.trim(),
        food_groups: this.data.foodGroups,
        notes: this.data.notes.trim(),
      })
      this.setData({ showForm: false, dishName: "", cuisine: "", notes: "", foodGroups: [], groups: GROUPS.map((group) => ({ ...group, checked: false })) })
      wx.showToast({ title: "饮食已记录", icon: "success" })
      await this.load()
    } catch (error) {
      this.setData({ error: error.message || "保存失败" })
    } finally {
      this.setData({ saving: false })
    }
  },

  removeEntry(event) {
    const id = Number(event.currentTarget.dataset.id)
    wx.showModal({
      title: "删除这条记录？",
      content: "删除后，健康报告也会同步更新。",
      confirmColor: "#b85539",
      success: async (result) => {
        if (!result.confirm) return
        try {
          await api.delete(`/food-journal/${id}`)
          await this.load()
        } catch (error) {
          wx.showToast({ title: error.message || "删除失败", icon: "none" })
        }
      },
    })
  },

  goChat() { wx.navigateTo({ url: "/pages/chat/chat" }) },
})
