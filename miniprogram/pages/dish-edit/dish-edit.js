const api = require("../../utils/api")
const ui = require("../../utils/ui")
const media = require("../../utils/media")
const video = require("../../utils/video")
const session = require("../../utils/session")
const dishUtil = require("../../utils/dish")
const drafts = require("../../utils/recipe-draft")
const recipeText = require("../../utils/recipe-text")

const CATEGORIES = ["川菜", "湘菜", "贵州菜", "云南菜", "粤菜"]
const TASTES = ["辣", "麻辣", "香辣", "酸辣", "鲜辣", "酸", "甜", "鲜", "清淡", "咸鲜", "蒜香", "葱香", "酱香", "豉香"]
const MEALS = [{ value: "lunch", label: "午餐" }, { value: "dinner", label: "晚餐" }, { value: "all", label: "通用" }]
const DIFFS = [{ value: "easy", label: "简单" }, { value: "medium", label: "中等" }, { value: "hard", label: "困难" }]

// 分段控件滑块位置（3 段等宽，容器左右各 8rpx 内边距）。
function segStyle(index, total) {
  return `width: calc((100% - 16rpx) / ${total}); transform: translateX(${index * 100}%);`
}

Page({
  data: {
    topPad: 0,
    id: 0,
    isNew: true,
    loading: true,
    error: "",
    recovery: null,
    draftStatus: "草稿会自动保存在本设备",
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
    videoMeta: null,
    videoChecking: false,
    tags: [],
    imageUrl: "",
    cover: "",
    images: [],
    imageThumbs: [],
    focusKey: "",
    uploadingCover: false,
    uploadingExtra: false,
    canShareFamily: false,
    shareFamily: false,
    familyName: "",
    alreadyFamily: false,
  },

  async onLoad(options) {
    this.setData({ topPad: getApp().navMetrics().navHeight })
    const id = Number(options.id || 0)
    this._invalidId = Boolean(options.id && (!/^\d+$/.test(options.id) || !Number.isSafeInteger(id) || id <= 0))
    if (!session.requireLogin(`/pages/dish-edit/dish-edit${id ? '?id=' + id : ''}`)) return
    this.setData({ id, isNew: !id })
    await this.initialize()
  },

  async initialize() {
    if (this._invalidId) { this.setData({ loading: false, error: "菜谱链接无效，请返回后重新打开" }); return }
    this.setData({ loading: true, error: "" })
    try {
      const user = await session.currentUser()
      this._draftUserId = user.id
      if (this.data.id) {
        if (!await this.loadDish(this.data.id)) return
      } else await this.checkFamily()
      this._baseline = JSON.stringify(drafts.snapshot(this.data))
      const recovery = drafts.read(user.id, this.data.id)
      this.setData({ recovery: recovery ? { ...recovery, time: new Date(recovery.savedAt).toLocaleString() } : null })
      this._draftReady = true
    } catch (error) {
      this.setData({ error: error.message || "菜谱加载失败，请重试" })
    } finally { this.setData({ loading: false }) }
  },

  onHide() { this.flushDraft() },
  onUnload() {
    this.flushDraft()
    this._disposed = true
    clearTimeout(this._draftTimer)
    clearTimeout(this._videoTimer)
    clearTimeout(this._backTimer)
  },
  updateForm(patch, callback) {
    if (this._disposed || this.data.recovery) return
    this.setData(patch, () => {
      clearTimeout(this._draftTimer)
      this._draftTimer = setTimeout(() => this.flushDraft(), 450)
      if (callback) callback()
    })
  },
  flushDraft() {
    clearTimeout(this._draftTimer)
    if (!this._draftReady || this._saved || this.data.recovery) return
    const value = drafts.snapshot(this.data)
    const serialized = JSON.stringify(value)
    if (serialized === this._baseline) {
      if (this._lastDraft && drafts.remove(this._draftUserId, this.data.id)) {
        this._lastDraft = ""
        if (!this._disposed) this.setData({ draftStatus: "修改已还原，没有待保存的草稿" })
      }
      return
    }
    if (serialized === this._lastDraft) return
    const saved = drafts.write(this._draftUserId, this.data.id, value)
    if (saved) this._lastDraft = serialized
    if (!this._disposed) this.setData({ draftStatus: saved ? "草稿已保存 · 仅本设备、当前账号可恢复" : "草稿未能保存，请完成保存后再离开" })
  },
  resumeDraft() {
    if (!this.data.recovery) return
    const value = this.data.recovery.value
    this.setData({
      ...value,
      shareFamily: this.data.canShareFamily && value.shareFamily,
      recovery: null,
      cover: media.assetUrl(value.imageUrl),
      imageThumbs: value.images.map((url) => media.assetUrl(url)),
      videoMeta: null,
      draftStatus: "已恢复草稿，确认后保存菜谱",
    }, () => { this.syncSeg(); this.fetchVideoPreview() })
  },
  discardDraft() {
    if (!drafts.remove(this._draftUserId, this.data.id)) { ui.toast("暂时无法清除草稿，请检查存储空间"); return }
    this.setData({ recovery: null, draftStatus: "已丢弃旧草稿，新的修改会自动保存" })
  },

  // 新建私房菜时，若用户已加入家庭则可选择直接共享给全家。
  async checkFamily() {
    try {
      const info = await api.get("/family")
      if (info && info.family) {
        this.setData({ canShareFamily: true, familyName: info.family.name || "家庭" })
      }
    } catch (_) { /* 无家庭或加载失败时不显示共享开关 */ }
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
      if (!dish.access || !dish.access.can_edit) throw new Error("你没有这道菜谱的编辑权限，请返回查看或新建自己的私房菜")
      this._sourceDish = dish
      const categories = dish.category && CATEGORIES.indexOf(dish.category) < 0 ? [dish.category, ...CATEGORIES] : CATEGORIES
      const ingredients = recipeText.formatIngredients(media.asArray(dish.ingredients))
      const seasonings = recipeText.formatIngredients(media.asArray(dish.seasonings))
      const steps = recipeText.formatSteps(media.asArray(dish.steps))
      const images = media.asArray(dish.images).filter((x) => typeof x === "string")
      this.setData({
        categories,
        name: dish.name || "",
        category: dish.category || "川菜",
        mealType: dish.meal_type || "all",
        difficulty: dish.difficulty || "easy",
        tasteList: dishUtil.tasteTags(dish.taste),
        cookTime: dish.cook_time == null ? 15 : dish.cook_time,
        sortOrder: dish.sort_order || 0,
        ingredientsText: ingredients,
        seasoningsText: seasonings,
        stepsText: steps,
        remark: dish.remark || "",
        videoUrl: dish.video_url || "",
        videoMeta: this.decorateMeta(dish.video_meta),
        alreadyFamily: Boolean(dish.family_id),
        tags: media.asArray(dish.tags).filter((x) => typeof x === "string"),
        imageUrl: dish.image_url || "",
        cover: media.assetUrl(dish.image_url),
        images,
        imageThumbs: images.map((x) => media.assetUrl(x)),
      })
      this._lastPreviewUrl = dish.video_url || ""
      this.syncSeg()
      return true
    } catch (error) {
      this.setData({ error: error.message || "菜谱加载失败，请重试" })
      return false
    } finally {
      this.setData({ loading: false })
    }
  },
  onName(e) { this.updateForm({ name: e.detail.value }) },
  onRemark(e) { this.updateForm({ remark: e.detail.value }) },
  onVideo(e) {
    const value = e.detail.value
    this.updateForm({ videoUrl: value })
    if (this._videoTimer) clearTimeout(this._videoTimer)
    if (!value.trim()) {
      this._lastPreviewUrl = ""
      this.setData({ videoMeta: null, videoChecking: false })
      return
    }
    this._videoTimer = setTimeout(() => this.fetchVideoPreview(), 700)
  },

  // 把服务端 link-preview / video_meta 转成视图模型；封面转成可加载的绝对地址。
  decorateMeta(meta) {
    if (!meta || typeof meta !== "object") return null
    const knownPlatform = meta.platform === "bilibili" || meta.platform === "douyin"
    return {
      url: meta.url || "",
      platform: meta.platform || "web",
      platformName: meta.platform_name || "链接",
      supported: Boolean(meta.supported || knownPlatform),
      title: meta.title || "",
      cover: media.assetUrl(meta.cover),
      author: meta.author || "",
      duration: meta.duration || "",
      playable: Boolean(meta.playable && (meta.supported || knownPlatform)),
    }
  },

  // 拉取链接预览：仅当输入框仍是同一链接时才回填，避免快速改动时旧结果覆盖。
  async fetchVideoPreview() {
    const url = this.data.videoUrl.trim()
    if (!url || !/^https?:\/\//i.test(url)) {
      this.setData({ videoMeta: null, videoChecking: false })
      return
    }
    if (url === this._lastPreviewUrl && this.data.videoMeta) return
    this._lastPreviewUrl = url
    this.setData({ videoChecking: true })
    try {
      const meta = await api.get("/link-preview", { url })
      if (this.data.videoUrl.trim() === url) this.setData({ videoMeta: this.decorateMeta(meta) })
    } catch (_) {
      if (this.data.videoUrl.trim() === url) this.setData({ videoMeta: null })
    } finally {
      if (!this._disposed && this.data.videoUrl.trim() === url) this.setData({ videoChecking: false })
    }
  },

  onCoverError() {
    if (this.data.videoMeta) this.setData({ "videoMeta.cover": "" })
  },

  openVideoLink() {
    const url = this.data.videoUrl.trim()
    if (!url) return
    if (!video.open(url, () => ui.toast("链接已复制，请在浏览器中打开"))) {
      ui.toast("目前仅支持抖音和哔哩哔哩视频")
    }
  },

  toggleShareFamily() {
    ui.haptic()
    this.updateForm({ shareFamily: !this.data.shareFamily })
  },

  onIngredients(e) { this.updateForm({ ingredientsText: e.detail.value }) },
  onSeasonings(e) { this.updateForm({ seasoningsText: e.detail.value }) },
  onSteps(e) { this.updateForm({ stepsText: e.detail.value }) },
  onFocus(e) { this.setData({ focusKey: e.currentTarget.dataset.key }) },
  onBlur(e) {
    const key = e && e.currentTarget && e.currentTarget.dataset.key
    this.setData({ focusKey: "" })
    if (key === "video") {
      if (this._videoTimer) clearTimeout(this._videoTimer)
      this.fetchVideoPreview()
    }
  },

  chooseCategory(e) { ui.haptic(); this.updateForm({ category: e.currentTarget.dataset.value }) },
  chooseMeal(e) { ui.haptic(); this.updateForm({ mealType: e.currentTarget.dataset.value }, () => this.syncSeg()) },
  chooseDiff(e) { ui.haptic(); this.updateForm({ difficulty: e.currentTarget.dataset.value }, () => this.syncSeg()) },
  onTasteChange(e) { this.updateForm({ tasteList: e.detail.value }) },
  onTagsChange(e) { this.updateForm({ tags: e.detail.value }) },

  stepCook(e) { this.updateForm({ cookTime: Math.min(600, Math.max(0, this.data.cookTime + Number(e.currentTarget.dataset.delta))) }) },
  stepSort(e) { this.updateForm({ sortOrder: Math.max(0, this.data.sortOrder + Number(e.currentTarget.dataset.delta)) }) },

  async pickCover() {
    if (this.data.uploadingCover) return
    const paths = await ui.chooseImages(1)
    if (!paths.length) return
    this.setData({ uploadingCover: true })
    try {
      const raw = await api.upload(paths[0])
      this.updateForm({ imageUrl: raw, cover: media.assetUrl(raw) })
    } catch (error) {
      ui.toast(error.message || "图片上传失败")
    } finally {
      this.setData({ uploadingCover: false })
    }
  },

  async pickExtra() {
    if (this.data.uploadingExtra) return
    if (this.data.images.length >= 18) { ui.toast("最多保存 18 张图片"); return }
    const paths = await ui.chooseImages(Math.min(9, 18 - this.data.images.length))
    if (!paths.length) return
    this.setData({ uploadingExtra: true })
    try {
      for (const path of paths) {
        const raw = await api.upload(path)
        this.updateForm({ images: [...this.data.images, raw], imageThumbs: [...this.data.imageThumbs, media.assetUrl(raw)] })
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
    this.updateForm({
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
    if (this.data.saving || this.data.loading || this.data.error || this.data.recovery || this.data.uploadingCover || this.data.uploadingExtra) return
    this.setData({ saving: true })
    const payload = {
      name,
      category: this.data.category,
      meal_type: this.data.mealType,
      difficulty: this.data.difficulty,
      taste: this.data.tasteList.join(","),
      cook_time: this.data.cookTime,
      ingredients: JSON.stringify(recipeText.parseIngredients(this.data.ingredientsText, media.asArray(this._sourceDish && this._sourceDish.ingredients))),
      seasonings: JSON.stringify(recipeText.parseIngredients(this.data.seasoningsText, media.asArray(this._sourceDish && this._sourceDish.seasonings))),
      steps: JSON.stringify(recipeText.parseSteps(this.data.stepsText, media.asArray(this._sourceDish && this._sourceDish.steps))),
      remark: this.data.remark.trim(),
      image_url: this.data.imageUrl,
      images: JSON.stringify(this.data.images),
      video_url: this.data.videoUrl.trim(),
      tags: JSON.stringify(this.data.tags),
      sort_order: this.data.sortOrder,
    }
    if (this.data.isNew && this.data.canShareFamily && this.data.shareFamily) payload.family = true
    try {
      if (this.data.isNew) await api.post("/dishes", payload)
      else await api.put(`/dishes/${this.data.id}`, payload)
      this._saved = true
      clearTimeout(this._draftTimer)
      drafts.remove(this._draftUserId, this.data.id)
      ui.toast("已保存", "success")
      this._backTimer = setTimeout(() => {
        if (getCurrentPages().length > 1) wx.navigateBack()
        else { wx.setStorageSync("ninimenu_dishes_scope", "mine"); wx.switchTab({ url: "/pages/dishes/dishes" }) }
      }, 500)
    } catch (error) {
      ui.toast(error.message || "保存失败")
      this.setData({ saving: false })
    }
  },
})
