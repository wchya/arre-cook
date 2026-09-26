const api = require("../../utils/api")
const session = require("../../utils/session")
const ui = require("../../utils/ui")

const LAST_EMAIL_KEY = "ninimenu_last_email"
const OPTIONS_CACHE_KEY = "ninimenu_auth_options"
const DEFAULT_DOMAINS = ["qq.com", "foxmail.com"]
const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/
const MODE_META = {
  wechat: { key: "wechat", label: "微信登录", icon: "wechat" },
  code: { key: "code", label: "验证码登录", icon: "mail" },
  password: { key: "password", label: "密码登录", icon: "key-round" },
}

function wxLoginCode() {
  return new Promise((resolve, reject) => {
    wx.login({
      success: (result) => (result.code ? resolve(result.code) : reject(new Error("微信登录暂不可用"))),
      fail: () => reject(new Error("无法获取微信登录凭证")),
    })
  })
}

function readStorage(key, fallback) {
  try {
    const value = wx.getStorageSync(key)
    return value === "" || value === undefined || value === null ? fallback : value
  } catch (_) {
    return fallback
  }
}

function codeCells(code) {
  return [0, 1, 2, 3, 4, 5].map((i) => ({ i, v: code.charAt(i) }))
}

function buildModes(options) {
  const keys = []
  if (options.wechat) keys.push("wechat")
  if (options.email) keys.push("code")
  keys.push("password")
  const modes = keys.map((key) => ({ ...MODE_META[key] }))
  // 只有两种方式时和 Web 端保持一致的文案
  if (modes.length === 2 && keys[0] === "code") modes[0].label = "邮箱验证码"
  return modes
}

Page({
  data: {
    topPad: 64,
    modes: buildModes({ wechat: true, email: true }),
    mode: "wechat",
    modeIndex: 0,
    options: { email: true, wechat: true, register_open: true, email_dev: false, email_domains: [] },

    email: "",
    emailValid: false,
    domainHints: [],
    code: "",
    codeCells: codeCells(""),
    codeFocus: false,
    account: "",
    password: "",
    passwordVisible: false,
    focus: "",

    agreed: false,
    consentSheet: false,
    consentShake: false,
    consentHint: false,

    cooldown: 0,
    codeSent: false,
    sending: false,
    loading: "",
    notice: null,
    emailHint: "",
  },

  onLoad(query) {
    const nav = getApp().navMetrics()
    const cached = readStorage(OPTIONS_CACHE_KEY, null)
    const email = readStorage(LAST_EMAIL_KEY, "")
    this._redirect = query.redirect ? decodeURIComponent(query.redirect) : ""
    this.setData({ topPad: nav.statusBarHeight + 40 })
    if (cached && typeof cached === "object") this.applyOptions(cached, true)
    if (email) this.setEmail(email)
    if (query.reason) {
      try { wx.removeStorageSync(api.SESSION_KEY) } catch (_) { /* ignore */ }
      this.showNotice(query.reason === "expired" ? "warn" : "info", query.reason === "expired" ? "登录已过期，请重新登录" : "已安全退出登录，期待你再来")
    }
    this.loadOptions()
    this.resumeSession()
  },

  onUnload() {
    clearInterval(this._cooldownTimer)
    clearTimeout(this._shakeTimer)
    clearTimeout(this._autoTimer)
  },

  // ---------- 登录方式 ----------

  async loadOptions() {
    try {
      const options = await api.get("/auth/options")
      try { wx.setStorageSync(OPTIONS_CACHE_KEY, options) } catch (_) { /* ignore */ }
      this.applyOptions(options, false)
    } catch (_) {
      if (!this.data.notice) this.showNotice("error", "暂时无法连接服务，请检查网络后重试")
    }
  },

  applyOptions(raw, fromCache) {
    const options = {
      email: raw.email !== false,
      wechat: Boolean(raw.wechat),
      register_open: raw.register_open !== false,
      email_dev: Boolean(raw.email_dev),
      email_domains: Array.isArray(raw.email_domains) ? raw.email_domains.filter(Boolean) : [],
    }
    const modes = buildModes(options)
    // 用户已手动切换过就保留当前方式；否则默认第一种（优先微信）
    let mode = this._modeTouched ? this.data.mode : modes[0].key
    if (!modes.some((item) => item.key === mode)) mode = modes[0].key
    this.setData({ options, modes, mode, modeIndex: modes.findIndex((item) => item.key === mode) }, () => this.refreshEmailState())
  },

  switchMode(event) {
    this.changeMode(event.currentTarget.dataset.mode)
  },

  changeMode(mode) {
    const modeIndex = this.data.modes.findIndex((item) => item.key === mode)
    if (modeIndex < 0 || mode === this.data.mode) return
    this._modeTouched = true
    ui.haptic()
    this.setData({ mode, modeIndex, focus: "", codeFocus: false, passwordVisible: false })
    if (this.data.notice && this.data.notice.type === "error") this.setData({ notice: null })
  },

  goCodeMode() {
    this.changeMode("code")
  },

  // ---------- 输入 ----------

  onFocus(event) {
    this.setData({ focus: event.currentTarget.dataset.field })
  },

  onBlur(event) {
    if (this.data.focus === event.currentTarget.dataset.field) this.setData({ focus: "" })
  },

  onEmailInput(event) {
    this.setEmail(event.detail.value)
    this.clearError()
  },

  setEmail(value) {
    this.setData({ email: String(value || "").trim() }, () => this.refreshEmailState())
  },

  refreshEmailState() {
    const { email, options } = this.data
    const domains = options.email_domains.length ? options.email_domains : DEFAULT_DOMAINS
    const [, domain = ""] = email.split("@")
    let emailValid = EMAIL_RE.test(email)
    if (emailValid && options.email_domains.length) emailValid = options.email_domains.indexOf(domain.toLowerCase()) >= 0
    let domainHints = []
    if (email && !emailValid) {
      domainHints = email.indexOf("@") < 0 ? domains : domains.filter((item) => item.indexOf(domain.toLowerCase()) === 0 && item !== domain)
    }
    const supported = options.email_domains.length ? `（支持 ${options.email_domains.map((item) => `@${item}`).join("、")}）` : ""
    let emailHint = `未注册的邮箱验证后自动创建账号${supported}`
    if (options.wechat) emailHint += "，并绑定当前微信"
    if (!options.register_open) emailHint = "当前暂停新用户注册，已有账号可以正常登录"
    this.setData({ emailValid, domainHints: domainHints.slice(0, 3), emailHint })
  },

  applyDomain(event) {
    const local = this.data.email.split("@")[0]
    if (!local) return
    this.setEmail(`${local}@${event.currentTarget.dataset.domain}`)
    ui.haptic()
  },

  clearEmail() {
    this.setEmail("")
  },

  focusCodeInput() {
    this.setData({ codeFocus: true, focus: "code" })
  },

  onCodeInput(event) {
    const code = String(event.detail.value || "").replace(/\D/g, "").slice(0, 6)
    this.setData({ code, codeCells: codeCells(code) })
    this.clearError()
    clearTimeout(this._autoTimer)
    if (code.length === 6) {
      wx.hideKeyboard({ fail: () => {} })
      // 协议已勾选时，输满 6 位自动登录，和常见验证码体验一致
      if (this.data.agreed && this.data.emailValid) this._autoTimer = setTimeout(() => this.submitCode(), 220)
    }
    return code
  },

  onCodeFocus() {
    this.setData({ focus: "code", codeFocus: true })
  },

  onCodeBlur() {
    this.setData({ focus: this.data.focus === "code" ? "" : this.data.focus, codeFocus: false })
  },

  onAccountInput(event) {
    this.setData({ account: String(event.detail.value || "").trim() })
    this.clearError()
  },

  onPasswordInput(event) {
    this.setData({ password: event.detail.value })
    this.clearError()
  },

  togglePasswordVisible() {
    this.setData({ passwordVisible: !this.data.passwordVisible })
  },

  // ---------- 协议 ----------

  toggleConsent() {
    ui.haptic()
    this.setData({ agreed: !this.data.agreed, consentHint: false })
  },

  openTerms() {
    wx.navigateTo({ url: "/pages/legal/legal?type=terms" })
  },

  openPrivacy() {
    wx.navigateTo({ url: "/pages/legal/legal?type=privacy" })
  },

  closeConsentSheet() {
    this._pendingAction = ""
    this.setData({ consentSheet: false })
  },

  agreeAndContinue() {
    const action = this._pendingAction
    this._pendingAction = ""
    this.setData({ agreed: true, consentHint: false, consentSheet: false })
    if (action) setTimeout(() => this.runWithConsent(action), 320)
  },

  // 未勾选协议时：底部协议行抖动提示，并弹出“同意并继续”确认，同意后继续刚才的操作。
  async runWithConsent(action) {
    if (!this.data.agreed) {
      this._pendingAction = action
      ui.haptic("medium")
      this.setData({ consentShake: true, consentHint: true, consentSheet: true })
      clearTimeout(this._shakeTimer)
      this._shakeTimer = setTimeout(() => this.setData({ consentShake: false }), 500)
      return
    }
    try {
      await this.ensurePrivacyAuthorize()
    } catch (_) {
      this.showNotice("error", "需要同意隐私保护指引后才能继续登录")
      return
    }
    this[action]()
  },

  async ensurePrivacyAuthorize() {
    const saved = readStorage(session.CONSENT_KEY, "") === "1"
    if (!saved && typeof wx.requirePrivacyAuthorize === "function") {
      await new Promise((resolve, reject) => wx.requirePrivacyAuthorize({ success: resolve, fail: reject }))
    }
    try { wx.setStorageSync(session.CONSENT_KEY, "1") } catch (_) { /* ignore */ }
  },

  // ---------- 提示 ----------

  showNotice(type, text) {
    const icons = { error: "circle-alert", warn: "triangle-alert", info: "info", success: "circle-check" }
    const colors = { error: "red", warn: "yellow-dark", info: "primary-dark", success: "mint-ink" }
    this.setData({ notice: { type, text, icon: icons[type] || "info", color: colors[type] || "text2" } })
  },

  clearError() {
    if (this.data.notice && this.data.notice.type === "error") this.setData({ notice: null })
  },

  // ---------- 动作入口（先过协议） ----------

  loginWithWechat() {
    if (!this.data.loading) this.runWithConsent("doWechatLogin")
  },

  sendCode() {
    if (this.data.sending || this.data.cooldown > 0) return
    if (!this.data.emailValid) {
      this.showNotice("error", this.data.options.email_domains.length ? "请输入支持的邮箱地址" : "请输入正确的邮箱地址")
      return
    }
    this.runWithConsent("doSendCode")
  },

  submitCode() {
    if (this.data.loading) return
    if (!this.data.emailValid) { this.showNotice("error", "请输入正确的邮箱地址"); return }
    if (!/^\d{6}$/.test(this.data.code)) { this.showNotice("error", "请输入 6 位邮箱验证码"); this.focusCodeInput(); return }
    this.runWithConsent("doSubmitCode")
  },

  submitPassword() {
    if (this.data.loading) return
    if (!this.data.account || !this.data.password) { this.showNotice("error", "请输入账号和密码"); return }
    this.runWithConsent("doSubmitPassword")
  },

  // ---------- 实际请求 ----------

  async doWechatLogin() {
    this.setData({ loading: "wechat", notice: null })
    try {
      const code = await wxLoginCode()
      const result = await api.post("/auth/wechat", { code })
      if (result.need_bind) {
        this.changeMode("code")
        this.showNotice("info", "这个微信还没有绑定账号。用邮箱验证码登录一次，之后就能一键进入。")
        return
      }
      if (!result.token) throw new Error("微信登录没有返回有效凭证")
      this.enterApp(result)
    } catch (error) {
      this.showNotice("error", error.message || "微信登录失败")
    } finally {
      this.setData({ loading: "" })
    }
  },

  async doSendCode() {
    const email = this.data.email
    this.setData({ sending: true, notice: null })
    try {
      const result = await api.post("/auth/email/code", { email })
      try { wx.setStorageSync(LAST_EMAIL_KEY, email) } catch (_) { /* ignore */ }
      this.startCooldown(result.cooldown || 60)
      this.setData({ codeSent: true })
      ui.toast(this.data.options.email_dev ? "开发模式：验证码在服务端日志" : "验证码已发送，请查收邮件")
      setTimeout(() => this.focusCodeInput(), 300)
    } catch (error) {
      if (error.data && error.data.cooldown) this.startCooldown(error.data.cooldown)
      this.showNotice("error", error.message || "验证码发送失败")
    } finally {
      this.setData({ sending: false })
    }
  },

  startCooldown(seconds) {
    clearInterval(this._cooldownTimer)
    this.setData({ cooldown: seconds })
    this._cooldownTimer = setInterval(() => {
      const cooldown = Math.max(0, this.data.cooldown - 1)
      this.setData({ cooldown })
      if (cooldown === 0) clearInterval(this._cooldownTimer)
    }, 1000)
  },

  async doSubmitCode() {
    this.setData({ loading: "submit", notice: null })
    try {
      let wechatCode = ""
      if (this.data.options.wechat) {
        try { wechatCode = await wxLoginCode() } catch (_) { /* 邮箱登录不依赖微信凭证 */ }
      }
      const result = await api.post("/auth/email/login", { email: this.data.email, code: this.data.code, wechat_code: wechatCode })
      try { wx.setStorageSync(LAST_EMAIL_KEY, this.data.email) } catch (_) { /* ignore */ }
      this.enterApp(result)
    } catch (error) {
      this.showNotice("error", error.message || "登录失败")
    } finally {
      this.setData({ loading: "" })
    }
  },

  async doSubmitPassword() {
    this.setData({ loading: "submit", notice: null })
    try {
      const result = await api.post("/auth/login", { account: this.data.account, password: this.data.password })
      this.enterApp(result)
    } catch (error) {
      this.showNotice("error", error.message || "登录失败")
    } finally {
      this.setData({ loading: "" })
    }
  },

  // 已登录且授权过隐私协议：静默校验令牌后直接进入。
  async resumeSession() {
    const app = getApp()
    if (!api.token() || app.globalData.sessionRestoreAttempted) return
    app.globalData.sessionRestoreAttempted = true
    if (readStorage(session.CONSENT_KEY, "") !== "1") return
    try {
      const user = await api.get("/me")
      session.setUser(user)
      session.enterAfterLogin(this._redirect)
    } catch (_) {
      try { wx.removeStorageSync(api.SESSION_KEY) } catch (_) { /* ignore */ }
    }
  },

  enterApp(result) {
    const app = getApp()
    app.globalData.sessionRestoreAttempted = true
    try { wx.setStorageSync(api.SESSION_KEY, result.token) } catch (_) { /* ignore */ }
    session.setUser(result.user || null)
    const name = result.user && result.user.nickname
    ui.toast(result.created ? "欢迎加入 ss-menu 🎉" : name ? `欢迎回来，${name}` : "登录成功")
    session.enterAfterLogin(this._redirect)
  },
})
