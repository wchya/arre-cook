// 自定义 TabBar：对应 Web 端 MainLayout 底栏（首页 / 菜谱 / AI 助手 / 记录 / 我的）。
// 中间的 AI 助手不是 Tab 页，点击进入对话页。各 Tab 页在 onShow 里调用 session.syncTabBar 同步选中态。
const TABS = [
  { path: "/pages/home/home", text: "首页", icon: "house" },
  { path: "/pages/dishes/dishes", text: "菜谱", icon: "utensils-crossed" },
  { path: "/pages/history/history", text: "记录", icon: "calendar-days" },
  { path: "/pages/me/me", text: "我的", icon: "user-round" },
]

Component({
  data: {
    selected: 0,
    left: TABS.slice(0, 2).map((tab, index) => ({ ...tab, index })),
    right: TABS.slice(2).map((tab, index) => ({ ...tab, index: index + 2 })),
    isIOS: false,
  },

  lifetimes: {
    attached() {
      const app = getApp()
      if (app && app.globalData.isIOS) this.setData({ isIOS: true })
    },
  },

  methods: {
    switchTab(event) {
      const index = Number(event.currentTarget.dataset.index)
      const tab = TABS[index]
      if (!tab || index === this.data.selected) return
      if (typeof wx.vibrateShort === "function") wx.vibrateShort({ type: "light", fail: () => {} })
      wx.switchTab({ url: tab.path })
    },
    openAssistant() {
      if (typeof wx.vibrateShort === "function") wx.vibrateShort({ type: "medium", fail: () => {} })
      wx.navigateTo({ url: "/pages/chat/chat" })
    },
  },
})
