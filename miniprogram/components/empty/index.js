// 空状态：对应 Web 端各页面的“浮动表情 + 标题 + 说明”。
Component({
  properties: {
    emoji: { type: String, value: "🍽" },
    title: { type: String, value: "" },
    desc: { type: String, value: "" },
    compact: { type: Boolean, value: false },
  },
})
