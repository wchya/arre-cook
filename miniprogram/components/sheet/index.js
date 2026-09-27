// 底部弹层：对应 Web 端 AnimatedBottomSheet（遮罩淡入 + 面板上滑）。
// <sheet show="{{open}}" title="标题" bind:close="closeSheet">内容<view slot="footer">底部按钮</view></sheet>
// 把手 / 标题区支持下拉关闭：拖过 120px 或快速下滑即触发 close，否则回弹。
const sheetTabs = require("../../utils/sheet-tabs")
const DISMISS_DISTANCE = 120
const DISMISS_VELOCITY = 0.6 // px/ms
const TRANSITION_MS = 380

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
    solid: { type: Boolean, value: true },
    keyboardHeight: { type: Number, value: 0 },
  },

  data: { visible: false, active: false, dragY: 0, dragging: false, maskOpacity: 1, isIOS: false, autoKeyboardHeight: 0, effectiveKeyboardHeight: 0 },

  observers: {
    "keyboardHeight, autoKeyboardHeight"(manual, auto) { this.setData({ effectiveKeyboardHeight: Math.max(0, manual || auto || 0) }) },
    show(value) {
      if (value) this.open()
      else this.hide()
    },
  },

  lifetimes: {
    attached() {
      const app = getApp()
      if (app && app.globalData.isIOS) this.setData({ isIOS: true })
      this._keyboardListener = (event) => {
        this._keyboardHeight = event.height || 0
        if (this.data.visible) this.setData({ autoKeyboardHeight: this._keyboardHeight })
      }
      if (typeof wx.onKeyboardHeightChange === "function") wx.onKeyboardHeightChange(this._keyboardListener)
    },
    detached() {
      sheetTabs.unlock(this)
      if (typeof wx.offKeyboardHeightChange === "function") wx.offKeyboardHeightChange(this._keyboardListener)
      clearTimeout(this._timer)
      clearTimeout(this._openedTimer)
    },
  },

  pageLifetimes: {
    hide() { sheetTabs.unlock(this) },
    show() { if (this.data.show && this.data.visible) sheetTabs.lock(this) },
  },

  methods: {
    open() {
      sheetTabs.lock(this)
      clearTimeout(this._timer)
      clearTimeout(this._openedTimer)
      if (this.data.visible && this.data.active) return
      this.setData({ visible: true, dragY: 0, dragging: false, maskOpacity: 1, autoKeyboardHeight: this._keyboardHeight || 0 }, () => {
        this._timer = setTimeout(() => {
          this.setData({ active: true }, () => {
            this._openedTimer = setTimeout(() => this.triggerEvent("opened"), TRANSITION_MS)
          })
        }, 30)
      })
    },
    hide() {
      if (!this.data.visible) return
      clearTimeout(this._timer)
      clearTimeout(this._openedTimer)
      this.setData({ active: false, dragY: 0, dragging: false })
      this._timer = setTimeout(() => {
        this.setData({ visible: false, autoKeyboardHeight: 0 }, () => { sheetTabs.unlock(this); this.triggerEvent("closed") })
      }, TRANSITION_MS)
    },
    onMask() {
      if (this.data.maskClosable) this.triggerEvent("close")
    },
    onClose() {
      this.triggerEvent("close")
    },

    // ---------- 下拉关闭 ----------
    onDragStart(event) {
      if (!this.data.closable) return
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
