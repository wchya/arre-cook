// 自定义导航栏：对应 Web 端 PageHeader（图标 + 标题 + 副标题 + 返回），自动避让右上角胶囊。
// theme: default（毛玻璃）| plain（无阴影）| transparent（悬浮在头图上，随 opacity 渐显）| dark（烹饪模式）
const { isTabPage } = require("../../utils/session")

Component({
  properties: {
    title: { type: String, value: "" },
    accent: { type: String, value: "" },
    subtitle: { type: String, value: "" },
    icon: { type: String, value: "" },
    theme: { type: String, value: "default" },
    fixed: { type: Boolean, value: false },
    opacity: { type: Number, value: 1 },
    hideBack: { type: Boolean, value: false },
  },

  data: {
    statusBarHeight: 20,
    navBarHeight: 44,
    capsuleSpace: 100,
    showBack: false,
    showHome: false,
  },

  lifetimes: {
    attached() {
      const nav = getApp().navMetrics()
      this.setData({ statusBarHeight: nav.statusBarHeight, navBarHeight: nav.navBarHeight, capsuleSpace: nav.capsuleSpace })
      this.syncButtons()
    },
    ready() {
      this.syncButtons()
    },
  },

  methods: {
    // 有上一页显示返回；从分享卡片等直接打开的子页面显示“回首页”；Tab 页都不显示。
    syncButtons() {
      const pages = getCurrentPages()
      if (!pages.length) return
      const route = pages[pages.length - 1].route
      const canBack = pages.length > 1
      const showBack = !this.data.hideBack && canBack
      const showHome = !this.data.hideBack && !canBack && !isTabPage(route) && route !== "pages/login/login"
      if (showBack !== this.data.showBack || showHome !== this.data.showHome) this.setData({ showBack, showHome })
    },
    onBack() {
      if (getCurrentPages().length > 1) wx.navigateBack({ delta: 1 })
      else wx.switchTab({ url: "/pages/home/home" })
    },
    onHome() {
      wx.switchTab({ url: "/pages/home/home" })
    },
  },
})
