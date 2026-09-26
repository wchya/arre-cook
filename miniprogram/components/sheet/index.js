// 底部弹层：对应 Web 端 AnimatedBottomSheet（遮罩淡入 + 面板上滑）。
// <sheet show="{{open}}" title="标题" bind:close="closeSheet">内容<view slot="footer">底部按钮</view></sheet>
// 把手 / 标题区支持下拉关闭：拖过 120px 或快速下滑即触发 close，否则回弹。
const DISMISS_DISTANCE = 120
const DISMISS_VELOCITY = 0.6 // px/ms

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

  data: { visible: false, active: false, dragY: 0, dragging: false, maskOpacity: 1, isIOS: false },

  observers: {
    show(value) {
      if (value) this.open()
      else this.hide()
    },
  },

  lifetimes: {
    attached() {
      const app = getApp()
      if (app && app.globalData.isIOS) this.setData({ isIOS: true })
    },
    detached() { clearTimeout(this._timer) },
  },

  methods: {
    open() {
      clearTimeout(this._timer)
      if (this.data.visible && this.data.active) return
      this.setData({ visible: true, dragY: 0, dragging: false }, () => {
        this._timer = setTimeout(() => this.setData({ active: true }), 30)
      })
    },
    hide() {
      if (!this.data.visible) return
      clearTimeout(this._timer)
      this.setData({ active: false, dragY: 0, dragging: false })
      this._timer = setTimeout(() => this.setData({ visible: false }), 320)
    },
    onMask() {
      if (this.data.maskClosable) this.triggerEvent("close")
    },
    onClose() {
      this.triggerEvent("close")
    },

    // ---------- 下拉关闭 ----------
    onDragStart(event) {
      const touch = event.touches && event.touches[0]
      if (!touch) return
      this._drag = { startY: touch.clientY, startAt: Date.now(), lastY: touch.clientY }
    },
    onDragMove(event) {
      const touch = event.touches && event.touches[0]
      if (!this._drag || !touch) return
      const delta = touch.clientY - this._drag.startY
      this._drag.lastY = touch.clientY
      // 向上拖给一点阻尼，向下跟手
      const dragY = delta > 0 ? delta : 0
      if (Math.abs(dragY - this.data.dragY) < 2) return
      this.setData({ dragging: true, dragY, maskOpacity: Math.max(0.2, 1 - dragY / 400) })
    },
    onDragEnd() {
      const drag = this._drag
      this._drag = null
      if (!drag) return
      const distance = drag.lastY - drag.startY
      const velocity = distance / Math.max(1, Date.now() - drag.startAt)
      if (distance > DISMISS_DISTANCE || (distance > 24 && velocity > DISMISS_VELOCITY)) {
        if (typeof wx.vibrateShort === "function") wx.vibrateShort({ type: "light", fail: () => {} })
        this.setData({ dragging: false })
        this.triggerEvent("close")
        return
      }
      this.setData({ dragging: false, dragY: 0, maskOpacity: 1 })
    },
    noop() {},
  },
})
