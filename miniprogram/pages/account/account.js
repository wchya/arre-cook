const api = require("../../utils/api")
const ui = require("../../utils/ui")
const session = require("../../utils/session")
const fmt = require("../../utils/format")

// 导出数据里各分类的计数，用于导出弹层的摘要展示。
const EXPORT_FIELDS = [
  { key: "meal_records", label: "用餐记录" },
  { key: "my_dishes", label: "私房菜" },
  { key: "favorites", label: "收藏" },
  { key: "day_ratings", label: "每日心情" },
  { key: "suggestions", label: "AI 建议" },
  { key: "chat_messages", label: "AI 对话" },
  { key: "food_journal", label: "饮食日记" },
]

Page({
  data: {
    loading: true,
    user: null,
    initial: "我",
    hasPassword: false,
    memberSince: "",
    lastLogin: "",
    // 密码弹层
    pwOpen: false,
    oldPw: "",
    newPw: "",
    confirmPw: "",
    showOld: false,
    showNew: false,
    savingPw: false,
    pwError: "",
    focusKey: "",
    // 退出所有设备
    logoutAllBusy: false,
    // 导出
    exportOpen: false,
    exportBusy: false,
    exportSummary: [],
    exportedAt: "",
    // 注销
    deleteOpen: false,
    deleteConfirm: "",
    deleting: false,
  },

  onLoad() {
    if (!session.requireLogin("/pages/account/account")) return
    this.load()
  },

  async load() {
    try {
      const user = await session.currentUser(true)
      this.applyUser(user)
    } catch (error) {
      ui.toast(error.message || "账号信息暂时无法加载")
    } finally {
      this.setData({ loading: false })
    }
  },

  applyUser(user) {
    if (!user) return
    this.setData({
      user,
      hasPassword: Boolean(user.has_password),
      initial: (user.nickname || user.username || "我").slice(0, 1).toUpperCase(),
      memberSince: user.created_at ? fmt.monthDay(user.created_at) : "",
      lastLogin: user.last_login_at ? fmt.relativeDate(user.last_login_at) : "",
    })
  },

  // ---------- 修改 / 设置密码 ----------

  openPassword() {
    this.setData({ pwOpen: true, oldPw: "", newPw: "", confirmPw: "", showOld: false, showNew: false, pwError: "" })
  },
  closePassword() { if (!this.data.savingPw) this.setData({ pwOpen: false }) },
  onOldInput(e) { this.setData({ oldPw: e.detail.value, pwError: "" }) },
  onNewInput(e) { this.setData({ newPw: e.detail.value, pwError: "" }) },
  onConfirmInput(e) { this.setData({ confirmPw: e.detail.value, pwError: "" }) },
  toggleOld() { this.setData({ showOld: !this.data.showOld }) },
  toggleNew() { this.setData({ showNew: !this.data.showNew }) },
  onFocus(e) { this.setData({ focusKey: e.currentTarget.dataset.key || "" }) },
  onBlur() { this.setData({ focusKey: "" }) },

  pwValid() {
    const { hasPassword, oldPw, newPw, confirmPw } = this.data
    if (hasPassword && !oldPw) return false
    return newPw.length >= 8 && newPw === confirmPw
  },

  async savePassword() {
    if (this.data.savingPw) return
    const { hasPassword, oldPw, newPw, confirmPw } = this.data
    if (newPw.length < 8) { this.setData({ pwError: "新密码至少 8 位" }); return }
    if (newPw !== confirmPw) { this.setData({ pwError: "两次输入的密码不一致" }); return }
    if (hasPassword && !oldPw) { this.setData({ pwError: "请输入当前密码" }); return }
    this.setData({ savingPw: true, pwError: "" })
    try {
      const result = await api.put("/me/password", { old_password: oldPw, new_password: newPw })
      // 改密后旧令牌失效，服务端签发新令牌，需要写回本地，避免下一次请求被判定为登录过期。
      if (result && result.token) {
        try { wx.setStorageSync(api.SESSION_KEY, result.token) } catch (_) { /* ignore */ }
      }
      if (result && result.user) { session.setUser(result.user); this.applyUser(result.user) }
      this.setData({ pwOpen: false })
      ui.toast(hasPassword ? "密码已更新" : "密码已设置", "success")
    } catch (error) {
      this.setData({ pwError: error.message || "密码更新失败" })
    } finally {
      this.setData({ savingPw: false })
    }
  },

  // ---------- 退出所有设备 ----------

  async logoutAll() {
    if (this.data.logoutAllBusy) return
    const ok = await ui.confirm({ title: "退出所有设备？", content: "所有设备上的登录都会失效，你需要重新登录。", confirmText: "退出", danger: true })
    if (!ok) return
    this.setData({ logoutAllBusy: true })
    try {
      await api.post("/me/logout-all")
      ui.toast("已退出所有设备", "success")
      setTimeout(() => session.logout("logout"), 600)
    } catch (error) {
      ui.toast(error.message || "操作失败")
      this.setData({ logoutAllBusy: false })
    }
  },

  // ---------- 导出数据 ----------

  async exportData() {
    if (this.data.exportBusy) return
    this.setData({ exportBusy: true })
    try {
      const data = await api.get("/me/export")
      this._exportJson = JSON.stringify(data, null, 2)
      const summary = EXPORT_FIELDS.map((field) => ({
        label: field.label,
        count: Array.isArray(data[field.key]) ? data[field.key].length : 0,
      }))
      this.setData({ exportOpen: true, exportSummary: summary, exportedAt: fmt.shortDateTime(data.exported_at) })
    } catch (error) {
      ui.toast(error.message || "导出失败")
    } finally {
      this.setData({ exportBusy: false })
    }
  },

  closeExport() { this.setData({ exportOpen: false }) },

  copyExport() {
    if (!this._exportJson) return
    wx.setClipboardData({
      data: this._exportJson,
      success: () => ui.toast("数据已复制到剪贴板", "success"),
      fail: () => ui.toast("复制失败"),
    })
  },

  // ---------- 注销账号 ----------

  openDelete() { this.setData({ deleteOpen: true, deleteConfirm: "" }) },
  closeDelete() { if (!this.data.deleting) this.setData({ deleteOpen: false }) },
  onDeleteInput(e) { this.setData({ deleteConfirm: e.detail.value }) },

  async confirmDelete() {
    if (this.data.deleting) return
    if (this.data.deleteConfirm.trim() !== "注销") { ui.toast("请输入“注销”确认"); return }
    const ok = await ui.confirm({ title: "永久注销账号？", content: "账号和个人数据将被永久删除，无法恢复。", confirmText: "注销", danger: true })
    if (!ok) return
    this.setData({ deleting: true })
    try {
      await api.delete("/me", { confirm: "注销" })
      ui.toast("账号已注销", "success")
      setTimeout(() => session.logout("deleted"), 800)
    } catch (error) {
      ui.toast(error.message || "注销失败")
      this.setData({ deleting: false })
    }
  },
})
