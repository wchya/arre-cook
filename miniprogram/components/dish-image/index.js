// 菜品图片：对应 Web 端 DishImage —— 渐变底 + 图片淡入，加载失败或无图时显示菜系表情。
// src 需为完整地址（utils/media.assetUrl 处理过的）。webp 需显式开启，否则 iOS 不显示种子菜谱图片。
Component({
  properties: {
    src: { type: String, value: "" },
    emoji: { type: String, value: "🍽" },
    emojiSize: { type: Number, value: 64 },
    mode: { type: String, value: "aspectFill" },
    lazy: { type: Boolean, value: true },
  },

  data: { failed: false, loaded: false },

  observers: {
    // 父页面 setData 整个列表（如下拉加载更多）会把同一 src 重新赋值并触发观察器；
    // 只有地址真正变化才重置加载态，否则图片不会再触发 load 事件而一直停在透明状态。
    src(src) {
      if (src === this._src) return
      this._src = src
      if (this.data.failed || this.data.loaded) this.setData({ failed: false, loaded: false })
    },
  },

  methods: {
    onLoad() { this.setData({ loaded: true }) },
    onError() { this.setData({ failed: true }) },
  },
})
