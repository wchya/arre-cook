const api = require("../../utils/api")
const ui = require("../../utils/ui")
const session = require("../../utils/session")
const fmt = require("../../utils/format")
const media = require("../../utils/media")

Page({
  data: {
    loading: true,
    hasFamily: false,
    isOwner: false,
    family: null,
    members: [],
    transferables: [],
    invitations: [],
    memberCount: 0,
    shopping: [],
    boughtCount: 0,
    familyDishes: [],
    requests: [],
    // 表单
    createName: "",
    joinToken: "",
    inviteEmail: "",
    inviteLink: "",
    itemName: "",
    itemAmount: "",
    // 弹层
    renameOpen: false,
    renameValue: "",
    savingRename: false,
    transferOpen: false,
    importOpen: false,
    busy: false,
  },

  onLoad(options) {
    if (!session.requireLogin("/pages/family/family")) return
    if (options && options.invite) this.setData({ joinToken: decodeURIComponent(options.invite) })
    this._scrollToRequests = Boolean(options && options.tab === "requests")
    this.load()
  },

  onShow() {
    if (this._loaded && session.hasSession()) this.load()
  },

  async load() {
    this.setData({ loading: !this._loaded })
    try {
      const info = await api.get("/family")
      const hasFamily = Boolean(info && info.family)
      const isOwner = info && info.role === "owner"
      const members = ((info && info.members) || []).map((m) => ({
        user_id: m.user_id,
        nickname: m.nickname || "成员",
        email: m.email || "",
        isOwner: m.role === "owner",
        initial: (m.nickname || m.email || "?").slice(0, 1).toUpperCase(),
      }))
      const invitations = ((info && info.invitations) || []).map((v) => ({
        id: v.id,
        email: v.email,
        expiresLabel: v.expires_at ? `${fmt.monthDay(v.expires_at)}前有效` : "",
      }))
      this.setData({
        hasFamily,
        isOwner,
        family: (info && info.family) || null,
        members,
        transferables: members.filter((m) => !m.isOwner),
        invitations,
        memberCount: members.length,
      })
      if (hasFamily) await Promise.all([this.loadShopping(), this.loadRequests()])
    } catch (error) {
      ui.toast(error.message || "家庭信息加载失败")
    } finally {
      this._loaded = true
      this.setData({ loading: false })
      wx.stopPullDownRefresh()
    }
  },

  async onPullDownRefresh() {
    await this.load()
  },

  async loadShopping() {
    try {
      const items = await api.get("/family/shopping")
      const shopping = (items || []).map((i) => ({ id: i.id, name: i.name, amount: i.amount || "", checked: Boolean(i.checked) }))
      this.setData({ shopping, boughtCount: shopping.filter((i) => i.checked).length })
    } catch (_) { /* 静默 */ }
  },

  // 家庭共享菜谱的删除申请：管理员看到全家的（可同意/拒绝），普通成员看到自己的（可撤回）。
  async loadRequests() {
    try {
      const items = await api.get("/family/dish-requests")
      const requests = (items || []).map((r) => ({
        id: r.id,
        dishName: r.dish_name,
        image: media.assetUrl(r.dish_image),
        by: r.requester_name || "家人",
        when: fmt.relativeDate(r.created_at),
      }))
      this.setData({ requests }, () => {
        // 从“菜谱删除申请”站内信进来时直接定位到申请列表
        if (!this._scrollToRequests || !requests.length) return
        this._scrollToRequests = false
        wx.pageScrollTo({ selector: "#requests", offsetTop: -120, duration: 300 })
      })
    } catch (_) { /* 静默 */ }
  },

  onReqCoverError(e) {
    const id = Number(e.currentTarget.dataset.id)
    const requests = this.data.requests.map((r) => (r.id === id ? { ...r, image: "" } : r))
    this.setData({ requests })
  },

  async approveRequest(e) {
    const { id, name } = e.currentTarget.dataset
    if (this.data.busy) return
    const ok = await ui.confirm({ title: `同意删除「${name}」？`, content: "删除后无法恢复，家庭菜单与买菜清单里的这道菜也会移除。", confirmText: "同意删除", danger: true })
    if (!ok) return
    this.setData({ busy: true })
    try {
      await api.post(`/family/dish-requests/${Number(id)}/approve`)
      ui.toast("已删除", "success")
      await this.load()
    } catch (error) {
      ui.toast(error.message || "操作失败")
    } finally {
      this.setData({ busy: false })
    }
  },

  rejectRequest(e) {
    const { id, name } = e.currentTarget.dataset
    if (this.data.busy) return
    wx.showModal({
      title: `拒绝删除「${name}」`,
      editable: true,
      placeholderText: "填写拒绝理由（可选）",
      confirmText: "拒绝",
      success: async (res) => {
        if (!res.confirm) return
        this.setData({ busy: true })
        try {
          await api.post(`/family/dish-requests/${Number(id)}/reject`, { reason: (res.content || "").trim() })
          ui.toast("已拒绝申请")
          await this.loadRequests()
        } catch (error) {
          ui.toast(error.message || "操作失败")
        } finally {
          this.setData({ busy: false })
        }
      },
    })
  },

  async cancelRequest(e) {
    const id = Number(e.currentTarget.dataset.id)
    const ok = await ui.confirm({ title: "撤回删除申请？", confirmText: "撤回" })
    if (!ok) return
    try {
      await api.delete(`/family/dish-requests/${id}`)
      ui.toast("已撤回")
      await this.loadRequests()
    } catch (error) {
      ui.toast(error.message || "撤回失败")
    }
  },

  onFieldFocus(e) { this.setData({ [e.currentTarget.dataset.k]: true }) },
  onFieldBlur(e) { this.setData({ [e.currentTarget.dataset.k]: false }) },

  // ---------- 无家庭：创建 / 加入 ----------
  onCreateInput(e) { this.setData({ createName: e.detail.value }) },
  onJoinInput(e) { this.setData({ joinToken: e.detail.value }) },

  async createFamily() {
    const name = this.data.createName.trim()
    if (!name || this.data.busy) return
    ui.haptic()
    this.setData({ busy: true })
    try {
      await api.post("/family", { name })
      this.setData({ createName: "" })
      ui.toast("家庭已创建", "success")
      await this.load()
    } catch (error) {
      ui.toast(error.message || "创建失败")
    } finally {
      this.setData({ busy: false })
    }
  },

  extractToken(raw) {
    const text = String(raw || "").trim()
    const i = text.indexOf("invite=")
    return i >= 0 ? text.slice(i + 7).split(/[&#\s]/)[0] : text
  },

  async joinFamily() {
    const token = this.extractToken(this.data.joinToken)
    if (!token || this.data.busy) return
    ui.haptic()
    this.setData({ busy: true })
    try {
      await api.post("/family/join", { token })
      this.setData({ joinToken: "" })
      ui.toast("已加入家庭", "success")
      await this.load()
    } catch (error) {
      ui.toast(error.message || "加入失败，请确认邀请链接")
    } finally {
      this.setData({ busy: false })
    }
  },

  // ---------- 重命名 ----------
  openRename() { this.setData({ renameOpen: true, renameValue: this.data.family ? this.data.family.name : "" }) },
  closeRename() { this.setData({ renameOpen: false }) },
  onRenameInput(e) { this.setData({ renameValue: e.detail.value }) },

  async saveRename() {
    const name = this.data.renameValue.trim()
    if (!name || this.data.savingRename) return
    this.setData({ savingRename: true })
    try {
      await api.patch("/family", { name })
      this.setData({ renameOpen: false })
      ui.toast("已更新", "success")
      await this.load()
    } catch (error) {
      ui.toast(error.message || "保存失败")
    } finally {
      this.setData({ savingRename: false })
    }
  },

  // ---------- 邀请 ----------
  onInviteInput(e) { this.setData({ inviteEmail: e.detail.value }) },

  async sendInvite() {
    const email = this.data.inviteEmail.trim()
    if (!email || this.data.busy) return
    this.setData({ busy: true })
    try {
      const res = await api.post("/family/invitations", { email })
      this.setData({ inviteEmail: "", inviteLink: (res && res.link) || "" })
      ui.toast(res && res.sent ? "邀请邮件已发送" : "邀请已创建，复制链接发给对方", "success")
      await this.load()
    } catch (error) {
      ui.toast(error.message || "邀请失败")
    } finally {
      this.setData({ busy: false })
    }
  },

  copyInvite() {
    if (!this.data.inviteLink) return
    wx.setClipboardData({ data: this.data.inviteLink, success: () => ui.toast("邀请链接已复制", "success") })
  },

  async revokeInvite(e) {
    const id = Number(e.currentTarget.dataset.id)
    const ok = await ui.confirm({ title: "撤销这个邀请？", confirmText: "撤销", danger: true })
    if (!ok) return
    try {
      await api.delete(`/family/invitations/${id}`)
      ui.toast("邀请已撤销", "success")
      await this.load()
    } catch (error) {
      ui.toast(error.message || "撤销失败")
    }
  },

  // ---------- 成员 ----------
  async removeMember(e) {
    const { id, name } = e.currentTarget.dataset
    const ok = await ui.confirm({ title: `移除 ${name}？`, content: "移除后对方将无法访问家庭菜单和买菜清单。", confirmText: "移除", danger: true })
    if (!ok) return
    try {
      await api.delete(`/family/members/${id}`)
      ui.toast("成员已移除", "success")
      await this.load()
    } catch (error) {
      ui.toast(error.message || "移除失败")
    }
  },

  openTransfer() { this.setData({ transferOpen: true }) },
  closeTransfer() { this.setData({ transferOpen: false }) },

  async pickTransfer(e) {
    const { id, name } = e.currentTarget.dataset
    const ok = await ui.confirm({ title: `转让给 ${name}？`, content: "转让后你将成为普通成员。", confirmText: "转让" })
    if (!ok) return
    this.setData({ transferOpen: false })
    try {
      await api.post("/family/transfer", { user_id: Number(id) })
      ui.toast("已转让创建者身份", "success")
      await this.load()
    } catch (error) {
      ui.toast(error.message || "转让失败")
    }
  },

  async leaveFamily() {
    const ok = await ui.confirm({ title: "退出家庭？", content: "退出后需重新受邀才能加入。", confirmText: "退出", danger: true })
    if (!ok) return
    try {
      await api.post("/family/leave")
      ui.toast("已退出家庭", "success")
      await this.load()
    } catch (error) {
      ui.toast(error.message || "退出失败")
    }
  },

  async deleteFamily() {
    const ok = await ui.confirm({ title: "解散家庭？", content: "解散后家庭菜单、买菜清单与家庭菜谱都会删除，且无法恢复。", confirmText: "解散", danger: true })
    if (!ok) return
    try {
      await api.delete("/family", { confirm: "解散家庭" })
      ui.toast("家庭已解散", "success")
      await this.load()
    } catch (error) {
      ui.toast(error.message || "解散失败")
    }
  },

  // ---------- 买菜清单 ----------
  onItemNameInput(e) { this.setData({ itemName: e.detail.value }) },
  onItemAmountInput(e) { this.setData({ itemAmount: e.detail.value }) },

  async addItem() {
    const name = this.data.itemName.trim()
    if (!name || this.data.busy) return
    this.setData({ busy: true })
    try {
      await api.post("/family/shopping", { name, amount: this.data.itemAmount.trim() })
      this.setData({ itemName: "", itemAmount: "" })
      await this.loadShopping()
    } catch (error) {
      ui.toast(error.message || "添加失败")
    } finally {
      this.setData({ busy: false })
    }
  },

  async toggleItem(e) {
    const { id, checked } = e.currentTarget.dataset
    ui.haptic()
    try {
      await api.patch(`/family/shopping/${id}`, { checked: !checked })
      await this.loadShopping()
    } catch (error) {
      ui.toast(error.message || "更新失败")
    }
  },

  async removeItem(e) {
    const id = Number(e.currentTarget.dataset.id)
    try {
      await api.delete(`/family/shopping/${id}`)
      await this.loadShopping()
    } catch (error) {
      ui.toast(error.message || "删除失败")
    }
  },

  async openImport() {
    this.setData({ importOpen: true })
    if (this._dishesLoaded) return
    try {
      const res = await api.get("/dishes", { scope: "family", pageSize: 100 })
      const list = (res && res.items) || (Array.isArray(res) ? res : [])
      this._dishesLoaded = true
      this.setData({ familyDishes: list.map((d) => ({ id: d.id, name: d.name })) })
    } catch (_) { /* 静默 */ }
  },

  closeImport() { this.setData({ importOpen: false }) },

  async importDish(e) {
    const id = Number(e.currentTarget.dataset.id)
    if (this.data.busy) return
    this.setData({ busy: true })
    try {
      const res = await api.post("/family/shopping/import", { dish_id: id })
      ui.toast(res && res.added ? `已加入 ${res.added} 种食材` : "已加入买菜清单", "success")
      this.setData({ importOpen: false })
      await this.loadShopping()
    } catch (error) {
      ui.toast(error.message || "导入失败")
    } finally {
      this.setData({ busy: false })
    }
  },

  goPlan() { ui.haptic(); wx.navigateTo({ url: "/pages/plan/plan" }) },
})
