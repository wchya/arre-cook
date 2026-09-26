const api = require("../../utils/api")
const session = require("../../utils/session")
const ui = require("../../utils/ui")
const media = require("../../utils/media")

const SPICE = [
  { v: -1, label: "不限", emoji: "🙂" },
  { v: 0, label: "不吃辣", emoji: "🥛" },
  { v: 1, label: "微辣", emoji: "🌶" },
  { v: 2, label: "中辣", emoji: "🌶🌶" },
  { v: 3, label: "特辣", emoji: "🔥" },
]
const COOK_TIMES = [0, 20, 30, 45, 60]
const GOALS = ["减脂", "增肌", "控糖", "清淡养胃", "快手省事", "宝宝辅食"]
const COMMON_AVOID = ["香菜", "葱", "蒜", "内脏", "羊肉", "鱼", "肥肉", "芹菜", "苦瓜"]
const COMMON_ALLERGY = ["花生", "虾", "蟹", "贝类", "鸡蛋", "牛奶", "大豆", "芝麻", "坚果"]
const DEFAULT_TASTES = ["麻辣", "香辣", "酸辣", "鲜辣", "酸", "甜", "鲜", "清淡", "咸鲜", "蒜香", "酱香"]
const EMPTY = { avoid_ingredients: [], allergies: [], favorite_tastes: [], spice_level: -1, household_size: 0, max_cook_time: 0, goals: "", notes: "" }

function goalsOf(text) {
  return String(text || "").split(/[,，、\s]+/).filter(Boolean)
}

Page({
  data: {
    loading: true,
    saving: false,
    dirty: false,
    form: EMPTY,
    spiceOptions: SPICE,
    cookTimes: COOK_TIMES,
    commonAvoid: COMMON_AVOID,
    commonAllergy: COMMON_ALLERGY,
    tastes: [],
    goals: [],
  },

  onLoad() {
    this._tasteNames = DEFAULT_TASTES
    if (!session.requireLogin("/pages/preferences/preferences")) return
    this.load()
  },

  async load() {
    const [prefs, settings] = await Promise.allSettled([api.get("/me/preferences"), api.get("/settings")])
    if (settings.status === "fulfilled") {
      const list = media.asArray(settings.value && settings.value.tastes).filter((item) => typeof item === "string" && item)
      if (list.length) this._tasteNames = list
    }
    const form = { ...EMPTY, ...(prefs.status === "fulfilled" ? prefs.value : {}) }
    form.avoid_ingredients = form.avoid_ingredients || []
    form.allergies = form.allergies || []
    form.favorite_tastes = form.favorite_tastes || []
    this.setData({ form, loading: false, dirty: false })
    this.renderOptions()
    if (prefs.status === "rejected") ui.toast(prefs.reason.message || "偏好加载失败")
  },

  renderOptions() {
    const form = this.data.form
    const goals = goalsOf(form.goals)
    this.setData({
      tastes: this._tasteNames.map((name) => ({ name, on: form.favorite_tastes.indexOf(name) >= 0 })),
      goals: GOALS.map((name) => ({ name, on: goals.indexOf(name) >= 0 })),
    })
  },

  patch(values) {
    const form = { ...this.data.form, ...values }
    this.setData({ form, dirty: true })
    this.renderOptions()
  },

  onAllergies(event) { this.patch({ allergies: event.detail.value }) },
  onAvoid(event) { this.patch({ avoid_ingredients: event.detail.value }) },

  chooseSpice(event) {
    ui.haptic()
    this.patch({ spice_level: Number(event.currentTarget.dataset.v) })
  },

  toggleTaste(event) {
    const name = event.currentTarget.dataset.name
    const list = this.data.form.favorite_tastes
    ui.haptic()
    this.patch({ favorite_tastes: list.indexOf(name) >= 0 ? list.filter((item) => item !== name) : list.concat([name]) })
  },

  chooseCookTime(event) {
    ui.haptic()
    this.patch({ max_cook_time: Number(event.currentTarget.dataset.v) })
  },

  changeHousehold(event) {
    const step = Number(event.currentTarget.dataset.step)
    const next = Math.max(0, Math.min(20, (this.data.form.household_size || 0) + step))
    ui.haptic()
    this.patch({ household_size: next })
  },

  toggleGoal(event) {
    const name = event.currentTarget.dataset.name
    const goals = goalsOf(this.data.form.goals)
    ui.haptic()
    this.patch({ goals: (goals.indexOf(name) >= 0 ? goals.filter((item) => item !== name) : goals.concat([name])).join("、") })
  },

  onNotes(event) {
    this.setData({ "form.notes": event.detail.value, dirty: true })
  },

  async save() {
    if (!this.data.dirty || this.data.saving) return
    this.setData({ saving: true })
    try {
      const saved = await api.put("/me/preferences", this.data.form)
      this.setData({ form: { ...EMPTY, ...saved }, dirty: false })
      this.renderOptions()
      ui.toast("已保存，推荐会按新偏好来", "success")
    } catch (error) {
      ui.toast(error.message || "保存失败")
    } finally {
      this.setData({ saving: false })
    }
  },
})
