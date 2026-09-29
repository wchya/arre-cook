const api = require("../../utils/api")
const GOALS = ["", "balanced", "weight_management", "regular_meals"]
const PATTERNS = ["", "home", "eating_out", "mixed"]
const splitTerms = value => value.split(/[,，、;；\n]/).map(s => s.trim()).filter(Boolean)
const cleared = () => ({ open: false, profile: null, loading: false, loadError: "", busy: false, failure: "", conflict: false, message: "", goalIndex: 0, patternIndex: 0, allergies: "", exclusions: "", confirmed: false, erase: false })

Component({
  data: {
    ...cleared(),
    goalLabels: ["暂不选择", "均衡饮食", "体重管理", "规律吃饭"],
    patternLabels: ["暂不选择", "以在家做饭为主", "以外食为主", "在家与外食都有"],
  },
  lifetimes: {
    attached() { this._identity = api.token(); this._request = 0 },
    detached() { this._request = (this._request || 0) + 1 },
  },
  pageLifetimes: {
    show() {
      if (this._identity !== api.token()) {
        this._identity = api.token(); this._request = (this._request || 0) + 1
        this.setData(cleared())
      }
    },
  },
  methods: {
    current(id, identity) { return id === this._request && identity === api.token() },
    toggle() {
      if (this.data.busy) return
      if (this.data.open) { this._request++; this.setData({ open: false }); return }
      return this.loadProfile()
    },
    async loadProfile() {
      const identity = api.token(), id = this._request = (this._request || 0) + 1
      this.setData({ open: true, loading: true, loadError: "", failure: "", conflict: false, message: "" })
      try {
        const profile = await api.get("/health/profile")
        if (this.current(id, identity)) this.applyProfile(profile)
      } catch (error) {
        if (this.current(id, identity)) this.setData({ loadError: error.message || "档案暂时无法加载" })
      } finally {
        if (this.current(id, identity)) this.setData({ loading: false })
      }
    },
    applyProfile(profile) {
      this._profileIdentity = api.token()
      this.setData({ profile, goalIndex: Math.max(0, GOALS.indexOf(profile.goal)), patternIndex: Math.max(0, PATTERNS.indexOf(profile.eating_pattern)), allergies: (profile.allergies || []).join("、"), exclusions: (profile.dietary_exclusions || []).join("、"), confirmed: false, erase: false })
    },
    onGoal(e) { if (!this.data.busy) this.setData({ goalIndex: Number(e.detail.value), confirmed: false, erase: false }) },
    onPattern(e) { if (!this.data.busy) this.setData({ patternIndex: Number(e.detail.value), confirmed: false, erase: false }) },
    onAllergies(e) { if (!this.data.busy) this.setData({ allergies: e.detail.value, confirmed: false, erase: false }) },
    onExclusions(e) { if (!this.data.busy) this.setData({ exclusions: e.detail.value, confirmed: false, erase: false }) },
    onConsent(e) { if (!this.data.busy) this.setData({ confirmed: e.detail.value.includes("confirmed") }) },
    showErase() { if (!this.data.busy) this.setData({ erase: true }) },
    cancelErase() { if (!this.data.busy) this.setData({ erase: false }) },
    saveProfile() {
      if (!this.data.confirmed) return
      return this.mutate(false)
    },
    clearProfile() { if (this.data.erase) return this.mutate(true) },
    async mutate(clear) {
      if (this.data.busy || !this.data.profile) return
      if (this._profileIdentity !== api.token()) { this._request++; this.setData(cleared()); return }
      const identity = api.token(), id = this._request = (this._request || 0) + 1
      this.setData({ busy: true, failure: "", conflict: false, message: "" })
      try {
        const version = this.data.profile.version
        const profile = clear ? await api.delete(`/health/profile?version=${version}`) : await api.put("/health/profile", { version, confirmed: true, goal: GOALS[this.data.goalIndex], eating_pattern: PATTERNS[this.data.patternIndex], allergies: splitTerms(this.data.allergies), dietary_exclusions: splitTerms(this.data.exclusions) })
        if (!this.current(id, identity)) return
        this.applyProfile(profile)
        this.setData({ message: clear ? "已清除档案及历史，原有饮食偏好保持不变。" : "已保存，下一次推荐会使用当前忌口。" })
        this.triggerEvent("changed")
      } catch (error) {
        if (this.current(id, identity)) this.setData({ failure: error.message || "保存失败，请重试", conflict: error.status === 409 })
      } finally {
        if (this.current(id, identity)) this.setData({ busy: false })
      }
    },
  },
})
