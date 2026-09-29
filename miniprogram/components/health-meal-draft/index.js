const api = require("../../utils/api")
const today = () => new Date(Date.now() + 8 * 3600 * 1000).toISOString().slice(0, 10)
const meals = ["", "breakfast", "lunch", "dinner", "snack"]
const cleared = () => ({ open: false, status: null, text: "", items: [], date: today(), today: today(), mealIndex: 0, confirmed: false, busy: false, saving: false, submitted: false, message: "", candidateIndex: -1, candidateQuery: "", candidates: { personal: [], catalog: [] }, candidateBusy: false, candidateMessage: "", invalidNutrition: false })
const stateLabels = { as_sold: "出售状态", ready_to_eat: "即食", raw: "生", cooked: "熟" }
const sourceLabels = ["称量或量取", "估算"]
const ready = item => !item.food || (item.portionKey ? Number(item.portionCount) > 0 && Number(item.portionCount) <= 100 : Number(item.amount) > 0 && Number(item.amount) <= 10000)
function batchItem(item) {
  const base = { dish_name: item.dish_name.trim(), portion: item.portion.trim() }
  if (!item.food) return base
  const nutrition = { nutrition_mode: "replace", nutrition_food_id: item.food.id, nutrition_unit: item.food.basis_unit, food_state: item.food.food_state, portion_source: item.portionKey ? "estimated" : item.portionSource }
  return item.portionKey ? { ...base, ...nutrition, nutrition_portion_key: item.portionKey, nutrition_portion_count: Number(item.portionCount) } : { ...base, ...nutrition, nutrition_amount: Number(item.amount) }
}
Component({
  data: { ...cleared(), mealLabels: ["请选择", "早餐", "午餐", "晚餐", "加餐"], sourceLabels },
  lifetimes: {
    attached() { this._identity = api.token(); this._request = 0; this._statusRequest = 0; this._candidateRequest = 0 },
    detached() { this.stopPending() },
  },
  pageLifetimes: {
    hide() { this.stopPending(); this.setData({ busy: false, saving: false, candidateBusy: false, candidateIndex: -1, candidates: { personal: [], catalog: [] } }) },
    show() {
      if (this._identity !== api.token()) { this.stopPending(); this._identity = api.token(); this._submission = null; this.setData(cleared()) }
      this.setData({ today: today() })
    },
  },
  methods: {
    current(id) { return id === this._request && this._identity === api.token() },
    stopPending() { this._request = (this._request || 0) + 1; this._statusRequest = (this._statusRequest || 0) + 1; this._candidateRequest = (this._candidateRequest || 0) + 1; if (this._task) this._task.abort(); if (this._candidateTask) this._candidateTask.abort(); this._task = null; this._candidateTask = null },
    setItems(items) { this.setData({ items, confirmed: false, invalidNutrition: items.some(item => !ready(item)) }) },
    toggle() {
      if (this.data.saving || this._identity !== api.token()) return
      if (this.data.open) this.stopPending()
      else this.loadStatus()
      this.setData({ open: !this.data.open, busy: false, candidateBusy: false, candidateIndex: -1, candidates: { personal: [], catalog: [] } })
    },
    async loadStatus() {
      const id = this._statusRequest = (this._statusRequest || 0) + 1
      try { const status = await api.get("/health/meal-drafts/status"); if (id === this._statusRequest && this._identity === api.token()) this.setData({ status }) }
      catch (_) { if (id === this._statusRequest && this._identity === api.token()) this.setData({ status: null }) }
    },
    onText(e) { if (!this.data.busy && !this.data.submitted) { this.stopPending(); this.setData({ text: e.detail.value, items: [], confirmed: false, candidateBusy: false, candidateIndex: -1, candidates: { personal: [], catalog: [] }, invalidNutrition: false }) } },
    onDate(e) { if (!this.data.submitted) this.setData({ date: e.detail.value, confirmed: false }) },
    onMeal(e) { if (!this.data.submitted) this.setData({ mealIndex: Number(e.detail.value), confirmed: false }) },
    onItem(e) {
      if (this.data.submitted) return
      const index = Number(e.currentTarget.dataset.index), key = e.currentTarget.dataset.field
      if (key !== "dish_name" && key !== "portion") return
      this.setItems(this.data.items.map((item, i) => i === index ? { ...item, [key]: e.detail.value, ...(key === "dish_name" ? { food: null, amount: "", portionKey: "", portionCount: "" } : {}) } : item))
      if (key === "dish_name" && this.data.candidateIndex === index) this.closeCandidates()
    },
    removeItem(e) { if (!this.data.submitted) { this.closeCandidates(); this.setItems(this.data.items.filter((_, i) => i !== Number(e.currentTarget.dataset.index))) } },
    onConsent(e) { if (!this.data.submitted) this.setData({ confirmed: e.detail.value.includes("confirmed") }) },
    cancel() { this.stopPending(); this.setData({ busy: false, message: "已停止整理，描述已保留。" }); this.loadStatus() },
    async parse() {
      if (this.data.busy || this.data.candidateBusy || this.data.submitted || !this.data.text.trim() || this._identity !== api.token()) return
      const id = ++this._request
      this.closeCandidates()
      this.setData({ busy: true, message: "", items: [], confirmed: false })
      try {
        const draft = await api.post("/health/meal-drafts/parse", { text: this.data.text }, { timeout: 45000, onTask: task => { this._task = task } })
        if (!this.current(id)) return
        this.setData({ items: draft.items.map(item => ({ ...item, food: null, amount: "", portionKey: "", portionCount: "", portionSource: "estimated", portionIndex: 0, sourceIndex: 1, portionChoices: [] })), invalidNutrition: false, message: draft.items.length ? "" : "未找到可确认的一次实际饮食，请只描述自己已吃过的一餐，或手动记录。" })
      } catch (error) { if (this.current(id)) this.setData({ message: error.message || "整理失败，请重试或手动记餐" }) }
      finally { if (this.current(id)) { this._task = null; this.setData({ busy: false }); this.loadStatus() } }
    },
    async save() {
      if (this.data.saving || this.data.candidateBusy || this.data.invalidNutrition || !this.data.confirmed || !this.data.items.length || !this.data.date || !meals[this.data.mealIndex] || this.data.items.some(i => !i.dish_name.trim()) || this._identity !== api.token()) return
      if (!this._submission) this._submission = { request_key: `draft_${Date.now()}_${Math.random().toString(36).slice(2)}`, confirmed: true, meal_date: this.data.date, meal_type: meals[this.data.mealIndex], items: this.data.items.map(batchItem) }
      const id = ++this._request
      this.closeCandidates()
      this.setData({ saving: true, submitted: true, message: "" })
      try {
        await api.post("/health/meal-drafts/confirm", this._submission)
        if (!this.current(id)) return
        this._submission = null
        this.setData({ submitted: false, items: [], text: "", confirmed: false, mealIndex: 0, candidateIndex: -1, invalidNutrition: false, message: "已保存为饮食日记。未选择营养来源的食物仍可在日记中补充。" })
        this.triggerEvent("saved")
      } catch (error) {
        if (this.current(id)) {
          if (error.status === 400) { this._submission = null; this.setData({ submitted: false, confirmed: false, message: error.message || "请修改日期、餐次和食物后重新确认" }) }
          else this.setData({ message: `${error.message || "保存失败"}。可重试确认，同一份提交不会重复保存。` })
        }
      }
      finally { if (this.current(id)) this.setData({ saving: false }) }
    },
    closeCandidates() { this._candidateRequest = (this._candidateRequest || 0) + 1; if (this._candidateTask) this._candidateTask.abort(); this._candidateTask = null; this.setData({ candidateIndex: -1, candidateBusy: false, candidates: { personal: [], catalog: [] }, candidateMessage: "" }) },
    openCandidates(e) {
      if (this.data.submitted || this._identity !== api.token()) return
      const index = Number(e.currentTarget.dataset.index)
      this.closeCandidates()
      this.setData({ candidateIndex: index, candidateQuery: this.data.items[index].dish_name })
    },
    onCandidateQuery(e) { this._candidateRequest++; this.setData({ candidateQuery: e.detail.value, candidates: { personal: [], catalog: [] }, candidateBusy: false, candidateMessage: "" }) },
    async searchCandidates() {
      if (this.data.candidateIndex < 0 || !this.data.candidateQuery.trim() || this.data.candidateBusy || this._identity !== api.token()) return
      const id = ++this._candidateRequest
      this.setData({ candidateBusy: true, candidates: { personal: [], catalog: [] }, candidateMessage: "" })
      try {
        const result = await api.get("/health/meal-drafts/candidates", { q: this.data.candidateQuery.trim() })
        if (id !== this._candidateRequest || this._identity !== api.token()) return
        this.setData({ candidates: { personal: result.personal.map(food => ({ ...food, stateLabel: stateLabels[food.food_state] || food.food_state })), catalog: result.catalog.map(food => ({ ...food, stateLabel: stateLabels[food.food_state] || food.food_state })) }, candidateMessage: !result.personal.length && !result.catalog.length ? "没有可用来源，可只保存文字，稍后在日记中补充。" : "" })
      } catch (error) { if (id === this._candidateRequest && this._identity === api.token()) this.setData({ candidateMessage: error.message || "食物候选读取失败" }) }
      finally { if (id === this._candidateRequest && this._identity === api.token()) this.setData({ candidateBusy: false }) }
    },
    selectFood(index, food) {
      if (index < 0 || !this.data.items[index] || this._identity !== api.token()) return
      food = { ...food, stateLabel: stateLabels[food.food_state] || food.food_state }
      const portionChoices = [{ key: "", label: `直接填写${food.basis_unit}` }, ...(food.portions || []).map(portion => ({ key: portion.key, label: `${portion.label} · 每份${portion.amount}${food.basis_unit}` }))]
      this.setItems(this.data.items.map((item, i) => i === index ? { ...item, food, amount: "", portionKey: "", portionCount: "", portionSource: food.recipe ? "estimated" : "measured", portionIndex: 0, sourceIndex: food.recipe ? 1 : 0, portionChoices } : item))
      this.closeCandidates()
    },
    choosePersonal(e) {
      if (this.data.submitted || this.data.candidateBusy || this.data.candidateIndex < 0 || this._identity !== api.token()) return
      const food = (this.data.candidates && this.data.candidates.personal || []).find(row => row.id === Number(e.currentTarget.dataset.id))
      if (food) this.selectFood(this.data.candidateIndex, food)
    },
    async adoptCatalog(e) {
      if (this.data.submitted || this.data.candidateBusy || this._identity !== api.token()) return
      const index = this.data.candidateIndex, request = this._candidateRequest
      this.setData({ candidateBusy: true, candidateMessage: "" })
      try {
        const food = await api.post(`/health/catalog/${Number(e.currentTarget.dataset.id)}/adopt`)
        if (request === this._candidateRequest && this._identity === api.token() && index === this.data.candidateIndex) this.selectFood(index, food)
      } catch (error) { if (request === this._candidateRequest && this._identity === api.token()) this.setData({ candidateMessage: error.message || "目录食物添加失败" }) }
      finally { if (request === this._candidateRequest && this._identity === api.token()) this.setData({ candidateBusy: false }) }
    },
    clearNutrition(e) {
      if (this.data.submitted) return
      const index = Number(e.currentTarget.dataset.index)
      this.setItems(this.data.items.map((item, i) => i === index ? { ...item, food: null, amount: "", portionKey: "", portionCount: "", portionChoices: [] } : item))
    },
    onNutritionValue(e) {
      if (this.data.submitted) return
      const index = Number(e.currentTarget.dataset.index), field = e.currentTarget.dataset.field
      if (field !== "amount" && field !== "portionCount") return
      this.setItems(this.data.items.map((item, i) => i === index ? { ...item, [field]: e.detail.value } : item))
    },
    onPortionMode(e) {
      if (this.data.submitted) return
      const index = Number(e.currentTarget.dataset.index), portionIndex = Number(e.detail.value)
      this.setItems(this.data.items.map((item, i) => i === index ? { ...item, portionIndex, portionKey: item.portionChoices[portionIndex].key, portionCount: "", amount: "" } : item))
    },
    onSource(e) {
      if (this.data.submitted) return
      const index = Number(e.currentTarget.dataset.index), sourceIndex = Number(e.detail.value)
      this.setItems(this.data.items.map((item, i) => i === index ? { ...item, sourceIndex, portionSource: sourceIndex ? "estimated" : "measured" } : item))
    },
  },
})
