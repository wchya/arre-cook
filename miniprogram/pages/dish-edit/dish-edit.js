const api = require("../../utils/api")
const ui = require("../../utils/ui")
const media = require("../../utils/media")
const session = require("../../utils/session")
const dishUtil = require("../../utils/dish")

const CATEGORIES = ["川菜", "湘菜", "贵州菜", "云南菜", "粤菜"]
const TASTES = ["辣", "麻辣", "香辣", "酸辣", "鲜辣", "酸", "甜", "鲜", "清淡", "咸鲜", "蒜香", "葱香", "酱香", "豉香"]
const MEALS = [{ value: "lunch", label: "🍳 午餐" }, { value: "dinner", label: "🍲 晚餐" }, { value: "all", label: "通用" }]
const DIFFS = [{ value: "easy", label: "简单" }, { value: "medium", label: "中等" }, { value: "hard", label: "困难" }]

// 分段控件滑块位置（3 段等宽，容器左右各 8rpx 内边距）。
function segStyle(index, total) {
  return `width: calc((100% - 16rpx) / ${total}); transform: translateX(${index * 100}%);`
}

// 每行「名称 数量」，最后一段作为数量，与 Web 端解析保持一致。
function parseIngredients(text) {
  return String(text || "").split("\n").map((line) => line.trim()).filter(Boolean).map((line) => {
    const parts = line.split(/\s+/)
    if (parts.length === 1) return { name: parts[0], amount: "" }
    return { name: parts.slice(0, -1).join(" "), amount: parts[parts.length - 1] }
  })
}

// 每行一步，可选 (n分钟) 标注时间。
function parseSteps(text) {
  return String(text || "").split("\n").map((line) => line.trim()).filter(Boolean).map((line) => {
    const cleaned = line.replace(/^\d+\.\s*/, "")
    const match = cleaned.match(/\((\d+)\s*分钟\)/)
    return { text: cleaned.replace(/\(\d+\s*分钟\)/, "").trim(), time: match ? Number(match[1]) : 0 }
  })
}

Page({
  data: {
    topPad: 0,
    id: 0,
    isNew: true,
    loading: false,
    saving: false,
    categories: CATEGORIES,
    tastes: TASTES,
    meals: MEALS,
    diffs: DIFFS,
    mealStyle: segStyle(2, 3),
    diffStyle: segStyle(0, 3),
    name: "",
    category: "川菜",
    mealType: "all",
    difficulty: "easy",
    tasteList: [],
    cookTime: 15,
    sortOrder: 0,
    ingredientsText: "",
    seasoningsText: "",
    stepsText: "",
    remark: "",
    videoUrl: "",
    tags: [],
    imageUrl: "",
    cover: "",
    images: [],
    imageThumbs: [],
    focusKey: "",
    uploadingCover: false,
    uploadingExtra: false,
  },

  onLoad(options) {
    const nav = getApp().navMetrics()
    this.setData({ topPad: nav.navHeight })
    if (!session.requireLogin("/pages/dish-edit/dish-edit")) return
    const id = Number(options.id || 0)
    if (id) {
      this.setData({ id, isNew: false })
      this.loadDish(id)
    }
  },

  syncSeg() {
    const mealIdx = Math.max(0, this.data.meals.findIndex((m) => m.value === this.data.mealType))
    const diffIdx = Math.max(0, this.data.diffs.findIndex((d) => d.value === this.data.difficulty))
    this.setData({ mealStyle: segStyle(mealIdx, 3), diffStyle: segStyle(diffIdx, 3) })
  },

  async loadDish(id) {
    this.setData({ loading: true })
    try {
      const dish = await api.get(`/dishes/${id}`)
      const categories = dish.category && CATEGORIES.indexOf(dish.category) < 0 ? [dish.category, ...CATEGORIES] : CATEGORIES
      const ingredients = dishUtil.normalizeIngredients(dish.ingredients).map((it) => `${it.name} ${it.amount}`.trim()).join("\n")
      const seasonings = dishUtil.normalizeIngredients(dish.seasonings).map((it) => `${it.name} ${it.amount}`.trim()).join("\n")
      const steps = dishUtil.normalizeSteps(dish.steps).map((it, i) => `${i + 1}. ${it.text}${it.time ? ` (${it.time}分钟)` : ""}`).join("\n")
      const images = media.asArray(dish.images).filter((x) => typeof x === "string")
      this.setData({
        categories,
        name: dish.name || "",
        category: dish.category || "川菜",
        mealType: dish.meal_type || "all",
        difficulty: dish.difficulty || "easy",
        tasteList: dishUtil.tasteTags(dish.taste),
        cookTime: dish.cook_time || 15,
        sortOrder: dish.sort_order || 0,
        ingredientsText: ingredients,
        seasoningsText: seasonings,
        stepsText: steps,
        remark: dish.remark || "",
        videoUrl: dish.video_url || "",
        tags: media.asArray(dish.tags).filter((x) => typeof x === "string"),
        imageUrl: dish.image_url || "",
        cover: media.assetUrl(dish.image_url),
        images,
        imageThumbs: images.map((x) => media.assetUrl(x)),
      })
      this.syncSeg()
    } catch (error) {
      ui.toast(error.message || "菜品加载失败")
    } finally {
      this.setData({ loading: false })
    }
  },
  onName(e) { this.setData({ name: e.detail.value }) },
  onRemark(e) { this.setData({ remark: e.detail.value }) },
  onVideo(e) { this.setData({ videoUrl: e.detail.value }) },
  onIngredients(e) { this.setData({ ingredientsText: e.detail.value }) },
  onSeasonings(e) { this.setData({ seasoningsText: e.detail.value }) },
  onSteps(e) { this.setData({ stepsText: e.detail.value }) },
  onFocus(e) { this.setData({ focusKey: e.currentTarget.dataset.key }) },
  onBlur() { this.setData({ focusKey: "" }) },

  chooseCategory(e) { ui.haptic(); this.setData({ category: e.currentTarget.dataset.value }) },
  chooseMeal(e) { ui.haptic(); this.setData({ mealType: e.currentTarget.dataset.value }, () => this.syncSeg()) },
  chooseDiff(e) { ui.haptic(); this.setData({ difficulty: e.currentTarget.dataset.value }, () => this.syncSeg()) },
  onTasteChange(e) { this.setData({ tasteList: e.detail.value }) },
  onTagsChange(e) { this.setData({ tags: e.detail.value }) },

  stepCook(e) { this.setData({ cookTime: Math.max(1, this.data.cookTime + Number(e.currentTarget.dataset.delta)) }) },
  stepSort(e) { this.setData({ sortOrder: Math.max(0, this.data.sortOrder + Number(e.currentTarget.dataset.delta)) }) },

  async pickCover() {
    if (this.data.uploadingCover) return
    const paths = await ui.chooseImages(1)
    if (!paths.length) return
    this.setData({ uploadingCover: true })
    try {
      const raw = await api.upload(paths[0])
      this.setData({ imageUrl: raw, cover: media.assetUrl(raw) })
    } catch (error) {
      ui.toast(error.message || "图片上传失败")
    } finally {
      this.setData({ uploadingCover: false })
    }
  },

  async pickExtra() {
    if (this.data.uploadingExtra) return
    const paths = await ui.chooseImages(9)
    if (!paths.length) return
    this.setData({ uploadingExtra: true })
    try {
      for (const path of paths) {
        const raw = await api.upload(path)
        this.setData({ images: [...this.data.images, raw], imageThumbs: [...this.data.imageThumbs, media.assetUrl(raw)] })
      }
      ui.toast("图片已上传", "success")
    } catch (error) {
      ui.toast(error.message || "图片上传失败")
    } finally {
      this.setData({ uploadingExtra: false })
    }
  },
  removeImage(e) {
    const index = Number(e.currentTarget.dataset.index)
    this.setData({
      images: this.data.images.filter((_, i) => i !== index),
      imageThumbs: this.data.imageThumbs.filter((_, i) => i !== index),
    })
  },

  previewCover() {
    if (this.data.cover) ui.preview([this.data.cover])
  },

  async save() {
    const name = this.data.name.trim()
    if (!name) { ui.toast("请填写菜品名称"); return }
    if (this.data.saving) return
    this.setData({ saving: true })
    const payload = {
      name,
      category: this.data.category,
      meal_type: this.data.mealType,
      difficulty: this.data.difficulty,
      taste: this.data.tasteList.join(","),
      cook_time: this.data.cookTime,
      ingredients: JSON.stringify(parseIngredients(this.data.ingredientsText)),
      seasonings: JSON.stringify(parseIngredients(this.data.seasoningsText)),
      steps: JSON.stringify(parseSteps(this.data.stepsText)),
      remark: this.data.remark.trim(),
      image_url: this.data.imageUrl,
      images: JSON.stringify(this.data.images),
      video_url: this.data.videoUrl.trim(),
      tags: JSON.stringify(this.data.tags),
      sort_order: this.data.sortOrder,
    }
    try {
      if (this.data.isNew) await api.post("/dishes", payload)
      else await api.put(`/dishes/${this.data.id}`, payload)
      ui.toast("已保存", "success")
      setTimeout(() => wx.navigateBack(), 600)
    } catch (error) {
      ui.toast(error.message || "保存失败")
      this.setData({ saving: false })
    }
  },
})
