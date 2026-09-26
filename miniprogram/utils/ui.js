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

// 选择一张或多张图片，返回本地临时路径数组；用户取消返回 []。
function chooseImages(count) {
  return new Promise((resolve) => {
    const done = (paths) => resolve(paths || [])
    if (typeof wx.chooseMedia === "function") {
      wx.chooseMedia({
        count: count || 1,
        mediaType: ["image"],
        sourceType: ["album", "camera"],
        sizeType: ["compressed"],
        success: (result) => done((result.tempFiles || []).map((file) => file.tempFilePath)),
        fail: () => done([]),
      })
      return
    }
    wx.chooseImage({
      count: count || 1,
      sizeType: ["compressed"],
      sourceType: ["album", "camera"],
      success: (result) => done(result.tempFilePaths),
      fail: () => done([]),
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
