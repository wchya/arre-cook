const api = require("../../utils/api")
const today = () => new Date(Date.now() + 8 * 3600 * 1000).toISOString().slice(0, 10)
const meals = ["", "breakfast", "lunch", "dinner", "snack"]
const cleared = () => ({ open: false, status: null, text: "", items: [], date: today(), today: today(), mealIndex: 0, confirmed: false, busy: false, saving: false, submitted: false, message: "" })
Component({
  data: { ...cleared(), mealLabels: ["请选择", "早餐", "午餐", "晚餐", "加餐"] },
  lifetimes: {
    attached() { this._identity = api.token(); this._request = 0; this._statusRequest = 0 },
    detached() { this.stopPending() },
  },
  pageLifetimes: {
    hide() { this.stopPending(); this.setData({ busy: false, saving: false }) },
    show() {
      if (this._identity !== api.token()) { this.stopPending(); this._identity = api.token(); this._submission = null; this.setData(cleared()) }
      this.setData({ today: today() })
    },
  },
  methods: {
    current(id) { return id === this._request && this._identity === api.token() },
    stopPending() { this._request = (this._request || 0) + 1; this._statusRequest = (this._statusRequest || 0) + 1; if (this._task) this._task.abort(); this._task = null },
    toggle() {
      if (this.data.saving || this._identity !== api.token()) return
      if (this.data.open) this.stopPending()
      else this.loadStatus()
      this.setData({ open: !this.data.open, busy: false })
    },
    async loadStatus() {
      const id = this._statusRequest = (this._statusRequest || 0) + 1
      try { const status = await api.get("/health/meal-drafts/status"); if (id === this._statusRequest && this._identity === api.token()) this.setData({ status }) }
      catch (_) { if (id === this._statusRequest && this._identity === api.token()) this.setData({ status: null }) }
    },
    onText(e) { if (!this.data.busy && !this.data.submitted) this.setData({ text: e.detail.value, items: [], confirmed: false }) },
    onDate(e) { if (!this.data.submitted) this.setData({ date: e.detail.value, confirmed: false }) },
    onMeal(e) { if (!this.data.submitted) this.setData({ mealIndex: Number(e.detail.value), confirmed: false }) },
    onItem(e) {
      if (this.data.submitted) return
      const index = Number(e.currentTarget.dataset.index), key = e.currentTarget.dataset.field
      if (key !== "dish_name" && key !== "portion") return
      this.setData({ items: this.data.items.map((item, i) => i === index ? { ...item, [key]: e.detail.value } : item), confirmed: false })
    },
    removeItem(e) { if (!this.data.submitted) this.setData({ items: this.data.items.filter((_, i) => i !== Number(e.currentTarget.dataset.index)), confirmed: false }) },
    onConsent(e) { if (!this.data.submitted) this.setData({ confirmed: e.detail.value.includes("confirmed") }) },
    cancel() { this.stopPending(); this.setData({ busy: false, message: "已停止整理，描述已保留。" }); this.loadStatus() },
    async parse() {
      if (this.data.busy || this.data.submitted || !this.data.text.trim() || this._identity !== api.token()) return
      const id = ++this._request
      this.setData({ busy: true, message: "", items: [], confirmed: false })
      try {
        const draft = await api.post("/health/meal-drafts/parse", { text: this.data.text }, { timeout: 45000, onTask: task => { this._task = task } })
        if (!this.current(id)) return
        this.setData({ items: draft.items, message: draft.items.length ? "" : "未找到可确认的一次实际饮食，请只描述自己已吃过的一餐，或手动记录。" })
      } catch (error) { if (this.current(id)) this.setData({ message: error.message || "整理失败，请重试或手动记餐" }) }
      finally { if (this.current(id)) { this._task = null; this.setData({ busy: false }); this.loadStatus() } }
    },
    async save() {
      if (this.data.saving || !this.data.confirmed || !this.data.items.length || !this.data.date || !meals[this.data.mealIndex] || this.data.items.some(i => !i.dish_name.trim()) || this._identity !== api.token()) return
      if (!this._submission) this._submission = { request_key: `draft_${Date.now()}_${Math.random().toString(36).slice(2)}`, confirmed: true, meal_date: this.data.date, meal_type: meals[this.data.mealIndex], items: this.data.items.map(({ dish_name, portion }) => ({ dish_name, portion })) }
      const id = ++this._request
      this.setData({ saving: true, submitted: true, message: "" })
      try {
        await api.post("/health/meal-drafts/confirm", this._submission)
        if (!this.current(id)) return
        this._submission = null
        this.setData({ submitted: false, items: [], text: "", confirmed: false, mealIndex: 0, message: "已保存为饮食日记；需要营养数据时可在日记中编辑并补充来源与用量。" })
        this.triggerEvent("saved")
      } catch (error) {
        if (this.current(id)) {
          if (error.status === 400) { this._submission = null; this.setData({ submitted: false, confirmed: false, message: error.message || "请修改日期、餐次和食物后重新确认" }) }
          else this.setData({ message: `${error.message || "保存失败"}。可重试确认，同一份提交不会重复保存。` })
        }
      }
      finally { if (this.current(id)) this.setData({ saving: false }) }
    },
  },
})
