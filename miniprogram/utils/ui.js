// 交互封装：统一 Toast / 确认框 / 操作菜单 / 选图，调用方写成 async 流程。

function toast(title, type) {
  if (!title) return
  wx.showToast({ title: String(title), icon: type === "success" ? "success" : "none", duration: type === "success" ? 1500 : 2200 })
}

function confirm(options) {
  return new Promise((resolve) => {
    wx.showModal({
      title: options.title || "提示",
      content: options.content || "",
      confirmText: options.confirmText || "确定",
      cancelText: options.cancelText || "取消",
      confirmColor: options.danger ? "#DC2626" : "#E8734A",
      showCancel: options.showCancel !== false,
      success: (result) => resolve(Boolean(result.confirm)),
      fail: () => resolve(false),
    })
  })
}

function actionSheet(itemList, alertText) {
  return new Promise((resolve) => {
    wx.showActionSheet({
      itemList,
      alertText,
      success: (result) => resolve(result.tapIndex),
      fail: () => resolve(-1),
    })
  })
}

// 确保已获得微信隐私授权：与本地登录标记解耦，避免历史 consent 标记（或隐私协议改版后重置）
// 导致相册 / 拍照被隐私能力拦截。已授权时立即放行，不会重复弹窗。
function ensurePrivacyAuthorized() {
  return new Promise((resolve) => {
    if (typeof wx.getPrivacySetting !== "function") return resolve(true)
    wx.getPrivacySetting({
      success: (res) => {
        if (!res || !res.needAuthorization) return resolve(true)
        if (typeof wx.requirePrivacyAuthorize === "function") {
          wx.requirePrivacyAuthorize({ success: () => resolve(true), fail: () => resolve(false) })
        } else {
          resolve(true)
        }
      },
      fail: () => resolve(true), // 查询失败不阻断，交给具体 API 的 fail 反馈
    })
  })
}

// 选择一张或多张图片，返回本地临时路径数组；用户取消返回 []。
// 先确保隐私授权通过再调起系统选图；失败时（非主动取消）给出可见提示，便于定位。
function chooseImages(count) {
  return new Promise((resolve) => {
    const done = (paths) => resolve(paths || [])
    const onFail = (err) => {
      const msg = (err && err.errMsg) || ""
      if (!/cancel/i.test(msg)) toast(msg ? "选择图片失败：" + msg.replace(/^choose\w+:fail\s*/i, "") : "选择图片失败")
      done([])
    }
    ensurePrivacyAuthorized().then((ok) => {
      if (!ok) {
        toast("需要同意隐私授权后才能选择图片")
        return done([])
      }
      if (typeof wx.chooseMedia === "function") {
        wx.chooseMedia({
          count: count || 1,
          mediaType: ["image"],
          sourceType: ["album", "camera"],
          sizeType: ["compressed"],
          success: (result) => done((result.tempFiles || []).map((file) => file.tempFilePath)),
          fail: onFail,
        })
        return
      }
      wx.chooseImage({
        count: count || 1,
        sizeType: ["compressed"],
        sourceType: ["album", "camera"],
        success: (result) => done(result.tempFilePaths),
        fail: onFail,
      })
    })
  })
}

function preview(urls, current) {
  const list = (urls || []).filter(Boolean)
  if (!list.length) return
  wx.previewImage({ urls: list, current: current || list[0] })
}

function haptic(type) {
  if (typeof wx.vibrateShort !== "function") return
  wx.vibrateShort({ type: type || "light", fail: () => {} })
}

module.exports = { toast, confirm, actionSheet, chooseImages, preview, haptic }
