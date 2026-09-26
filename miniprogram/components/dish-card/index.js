// 菜品网格卡：对应 Web 端 DishCard（心情推荐、同类菜品、收藏等复用）。
// dish 为 utils/dish.toCard 生成的视图模型；默认点击进入详情页。
Component({
  properties: {
    dish: { type: Object, value: null },
    showFav: { type: Boolean, value: false },
    showDifficulty: { type: Boolean, value: false },
    imageHeight: { type: Number, value: 240 },
    nonav: { type: Boolean, value: false },
  },

  methods: {
    onTap() {
      const dish = this.data.dish
      if (!dish) return
      this.triggerEvent("select", { id: dish.id })
      if (!this.data.nonav) wx.navigateTo({ url: `/pages/dish/dish?id=${dish.id}` })
    },
    onFav() {
      const dish = this.data.dish
      if (dish) this.triggerEvent("fav", { id: dish.id, active: dish.favorite })
    },
  },
})
