// 底部弹层：对应 Web 端 AnimatedBottomSheet（遮罩淡入 + 面板上滑）。
// <sheet show="{{open}}" title="标题" bind:close="closeSheet">内容<view slot="footer">底部按钮</view></sheet>
Component({
  options: { multipleSlots: true },

  properties: {
    show: { type: Boolean, value: false },
    title: { type: String, value: "" },
    subtitle: { type: String, value: "" },
    closable: { type: Boolean, value: true },
    handle: { type: Boolean, value: true },
    maskClosable: { type: Boolean, value: true },
    zIndex: { type: Number, value: 500 },
  },

  data: { visible: false, active: false },

  observers: {
    show(value) {
      if (value) this.open()
      else this.hide()
    },
  },

  lifetimes: {
    detached() { clearTimeout(this._timer) },
  },

  methods: {
    open() {
      clearTimeout(this._timer)
      if (this.data.visible && this.data.active) return
      this.setData({ visible: true }, () => {
        this._timer = setTimeout(() => this.setData({ active: true }), 30)
      })
    },
    hide() {
      if (!this.data.visible) return
      clearTimeout(this._timer)
      this.setData({ active: false })
      this._timer = setTimeout(() => this.setData({ visible: false }), 300)
    },
    onMask() {
      if (this.data.maskClosable) this.triggerEvent("close")
    },
    onClose() {
      this.triggerEvent("close")
    },
    noop() {},
  },
})
