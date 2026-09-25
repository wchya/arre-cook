const app = getApp()
const SESSION_KEY = "ninimenu_session"

function request(path, data) {
  return new Promise((resolve, reject) => {
    wx.request({
      url: `${app.globalData.apiBase}${path}`,
      method: data ? "POST" : "GET",
      data,
      header: { "content-type": "application/json" },
      success(response) {
        const body = response.data || {}
        if (response.statusCode < 200 || response.statusCode >= 300 || body.code !== 0) {
          reject(new Error(body.message || "服务暂时不可用"))
          return
        }
        resolve(body.data)
      },
      fail() {
        reject(new Error("网络连接失败，请稍后重试"))
      },
    })
  })
}

function wxLoginCode() {
  return new Promise((resolve, reject) => {
    wx.login({
      success: (result) => result.code ? resolve(result.code) : reject(new Error("微信登录暂不可用")),
      fail: () => reject(new Error("无法获取微信登录凭证")),
    })
  })
}

Page({
  data: {
    loginMode: "email",
    email: "",
    code: "",
    account: "",
    password: "",
    passwordVisible: false,
    agreed: false,
    cooldown: 0,
    loading: false,
    sending: false,
    error: "",
    message: "",
    emailDev: false,
    emailEnabled: true,
    wechatEnabled: true,
    registerOpen: true,
    canUseWechat: false,
  },

  onLoad(options) {
    if (options.reason) {
      try { wx.removeStorageSync(SESSION_KEY) } catch (_) { /* ignore */ }
      app.globalData.pendingToken = ""
      this.setData({ message: options.reason === "expired" ? "登录已过期，请重新登录" : "已退出登录" })
    }
    this.loadOptions()
    this.resumeSession()
  },

  onShow() {
    const timer = this.data.timer
    if (timer) clearInterval(timer)
    if (this.data.cooldown > 0) {
      const id = setInterval(() => {
        const cooldown = Math.max(0, this.data.cooldown - 1)
        this.setData({ cooldown })
        if (cooldown === 0) clearInterval(id)
      }, 1000)
      this.setData({ timer: id })
    }
  },

  onUnload() {
    if (this.data.timer) clearInterval(this.data.timer)
  },

  async loadOptions() {
    try {
      const options = await request("/auth/options")
      this.setData({
        emailEnabled: Boolean(options.email),
        emailDev: Boolean(options.email_dev),
        wechatEnabled: Boolean(options.wechat),
        registerOpen: options.register_open !== false,
      })
    } catch (_) {
      this.setData({ error: "暂时无法连接服务，请检查网络后重试" })
    }
  },

  async resumeSession() {
    let token = ""
    try { token = wx.getStorageSync(SESSION_KEY) || "" } catch (_) { /* ignore */ }
    if (!token) return
    // Do not validate an existing session (and therefore fetch account data)
    // until the user has already granted the current privacy agreement.
    let consented = false
    try { consented = wx.getStorageSync("ninimenu_privacy_consent_v1") === "1" } catch (_) { /* ignore */ }
    if (!consented) return
    try {
      await new Promise((resolve, reject) => wx.request({
        url: `${app.globalData.apiBase}/me`,
        header: { Authorization: `Bearer ${token}` },
        success: (response) => response.statusCode === 200 && response.data?.code === 0 ? resolve(response.data.data) : reject(new Error("登录已过期")),
        fail: () => reject(new Error("网络连接失败")),
      }))
      this.openWeb(token)
    } catch (_) {
      try { wx.removeStorageSync(SESSION_KEY) } catch (_) { /* ignore */ }
    }
  },

  onEmailInput(event) {
    this.setData({ email: event.detail.value.trim(), error: "" })
  },

  onCodeInput(event) {
    this.setData({ code: event.detail.value.replace(/\D/g, "").slice(0, 6), error: "" })
  },

  onAccountInput(event) {
    this.setData({ account: event.detail.value.trim(), error: "" })
  },

  onPasswordInput(event) {
    this.setData({ password: event.detail.value, error: "" })
  },

  onConsentChange(event) {
    this.setData({ agreed: event.detail.value.includes("agreed"), error: "" })
  },

  switchLoginMode(event) {
    const mode = event.currentTarget.dataset.mode === "password" ? "password" : "email"
    this.setData({ loginMode: mode, passwordVisible: false, error: "", message: "" })
  },

  togglePasswordMode() {
    this.switchLoginMode({ currentTarget: { dataset: { mode: this.data.loginMode === "password" ? "email" : "password" } } })
  },

  togglePasswordVisibility() {
    this.setData({ passwordVisible: !this.data.passwordVisible })
  },

  async ensurePrivacyConsent() {
    if (!this.data.agreed) {
      throw new Error("请先阅读并同意《用户服务协议》和《隐私政策》")
    }
    let saved = false
    try { saved = wx.getStorageSync("ninimenu_privacy_consent_v1") === "1" } catch (_) { /* ignore */ }
    if (!saved && typeof wx.requirePrivacyAuthorize === "function") {
      await new Promise((resolve, reject) => wx.requirePrivacyAuthorize({ success: resolve, fail: reject }))
    }
    try { wx.setStorageSync("ninimenu_privacy_consent_v1", "1") } catch (_) { /* ignore */ }
  },

  async sendCode() {
    try { await this.ensurePrivacyConsent() } catch (error) {
      this.setData({ error: error.message || "请先同意隐私政策" })
      return
    }
    const email = this.data.email.trim()
    if (!/^[^\s@]+@(?:qq\.com|foxmail\.com)$/i.test(email)) {
      this.setData({ error: "请输入 QQ 邮箱或 Foxmail 邮箱" })
      return
    }
    if (this.data.cooldown > 0 || this.data.sending) return
    this.setData({ sending: true, error: "", message: "" })
    try {
      const result = await request("/auth/email/code", { email })
      this.setData({ cooldown: result.cooldown || 60, message: this.data.emailDev ? "开发模式验证码已写入服务端日志" : "验证码已发送，请查收邮件" })
      this.onShow()
    } catch (error) {
      this.setData({ error: error.message || "验证码发送失败" })
    } finally {
      this.setData({ sending: false })
    }
  },

  async submitEmail() {
    try { await this.ensurePrivacyConsent() } catch (error) {
      this.setData({ error: error.message || "请先同意隐私政策" })
      return
    }
    if (!/^[^\s@]+@(?:qq\.com|foxmail\.com)$/i.test(this.data.email.trim())) {
      this.setData({ error: "请输入 QQ 邮箱或 Foxmail 邮箱" })
      return
    }
    if (!/^\d{6}$/.test(this.data.code)) {
      this.setData({ error: "请输入 6 位邮箱验证码" })
      return
    }
    this.setData({ loading: true, error: "", message: "" })
    try {
      const code = this.data.wechatEnabled ? await wxLoginCode() : ""
      const result = await request("/auth/email/login", { email: this.data.email.trim(), code: this.data.code, wechat_code: code })
      this.openWeb(result.token)
    } catch (error) {
      this.setData({ error: error.message || "登录失败" })
    } finally {
      this.setData({ loading: false })
    }
  },

  async submitPassword() {
    try { await this.ensurePrivacyConsent() } catch (error) {
      this.setData({ error: error.message || "请先同意隐私政策" })
      return
    }
    if (!this.data.account.trim() || !this.data.password) {
      this.setData({ error: "请输入账号和密码" })
      return
    }
    this.setData({ loading: true, error: "", message: "" })
    try {
      const result = await request("/auth/login", { account: this.data.account.trim(), password: this.data.password })
      this.openWeb(result.token)
    } catch (error) {
      this.setData({ error: error.message || "登录失败" })
    } finally {
      this.setData({ loading: false })
    }
  },

  async loginWithWechat() {
    if (this.data.loading || !this.data.wechatEnabled) return
    try { await this.ensurePrivacyConsent() } catch (error) {
      this.setData({ error: error.message || "请先同意隐私政策" })
      return
    }
    this.setData({ loading: true, error: "", message: "" })
    try {
      const code = await wxLoginCode()
      const result = await request("/auth/wechat", { code })
      if (result.need_bind) {
        this.setData({ error: "该微信尚未绑定账号，请使用 QQ 邮箱验证码登录并完成绑定" })
      } else if (result.token) {
        this.openWeb(result.token)
      } else {
        throw new Error("微信登录没有返回有效凭证")
      }
    } catch (error) {
      this.setData({ error: error.message || "微信登录失败" })
    } finally {
      this.setData({ loading: false })
    }
  },

  openWeb(token) {
    try { wx.setStorageSync(SESSION_KEY, token) } catch (_) { /* ignore */ }
    app.globalData.pendingToken = token
    wx.reLaunch({ url: "/pages/webview/webview" })
  },
})
