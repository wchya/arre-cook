// 图片地址处理：接口返回的多为站内相对路径（/uploads/...），小程序 <image> 需要完整 https 地址。
// 展示时用 assetUrl 转成绝对地址；提交给接口（头像、照片）时仍使用原始地址。

const FALLBACK_ORIGIN = "https://cook.arrebyte.top"
const IMAGE_EXT = /\.(jpg|jpeg|png|webp|gif|svg)(\?|#|$)/i

function origin() {
  try {
    const app = getApp()
    return (app && app.globalData && app.globalData.origin) || FALLBACK_ORIGIN
  } catch (_) {
    return FALLBACK_ORIGIN
  }
}

function isImageUrl(url) {
  if (!url || typeof url !== "string") return false
  const value = url.trim()
  return value.indexOf("/uploads/") === 0 || /^https?:\/\//i.test(value) || value.indexOf("//") === 0 || IMAGE_EXT.test(value)
}

function assetUrl(url) {
  if (!isImageUrl(url)) return ""
  const value = url.trim()
  if (/^https?:\/\//i.test(value)) return value
  if (value.indexOf("//") === 0) return `https:${value}`
  if (value.charAt(0) === "/") return `${origin()}${value}`
  return `${origin()}/${value.replace(/^\.?\//, "")}`
}

function asArray(value) {
  if (Array.isArray(value)) return value
  if (typeof value === "string" && value) {
    try {
      const parsed = JSON.parse(value)
      return Array.isArray(parsed) ? parsed : []
    } catch (_) {
      return []
    }
  }
  return []
}

// 菜品封面：image_url 优先，其次 images 第一张（种子菜谱的图片只在 images 里）。
function dishCoverRaw(dish) {
  if (!dish) return ""
  if (isImageUrl(dish.image_url)) return dish.image_url
  if (isImageUrl(dish.image)) return dish.image
  const images = asArray(dish.images)
  for (let i = 0; i < images.length; i++) {
    if (typeof images[i] === "string" && isImageUrl(images[i])) return images[i]
  }
  return ""
}

function dishCover(dish) {
  return assetUrl(dishCoverRaw(dish))
}

// 菜品所有图片（封面 + 额外图片，去重），用于详情页图集。
function dishGallery(dish) {
  if (!dish) return []
  const list = []
  const push = (url) => {
    const resolved = assetUrl(url)
    if (resolved && list.indexOf(resolved) < 0) list.push(resolved)
  }
  if (isImageUrl(dish.image_url)) push(dish.image_url)
  asArray(dish.images).forEach((item) => { if (typeof item === "string") push(item) })
  return list
}

// 用户上传照片列表（day rating photos 等是 JSON 字符串）。
function photoList(value) {
  return asArray(value).filter((item) => typeof item === "string" && isImageUrl(item))
}

module.exports = { origin, isImageUrl, assetUrl, asArray, dishCoverRaw, dishCover, dishGallery, photoList }
