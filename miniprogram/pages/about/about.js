const content = {
  "name": "arre食谱推荐小助手",
  "tagline": "让每天吃什么，变得轻松一点。",
  "intro": "一个面向个人与家庭的食谱站。把喜欢的菜、自己的拿手做法和日常饮食记录放在一起，从选菜到买菜，再到下厨，都更有头绪。",
  "features": [
    {
      "icon": "book-open",
      "title": "收好每一道拿手菜",
      "text": "浏览食谱、收藏喜欢的做法，也可以建立自己的私房菜。文字、照片与视频链接一起保存，做菜时随手查。"
    },
    {
      "icon": "sparkles",
      "title": "少一点选菜纠结",
      "text": "按口味、手边食材和烹饪时间挑菜；AI 饮食助手专注食谱与日常饮食，检查提问和回复后，结合你的偏好寻找灵感。"
    },
    {
      "icon": "shopping-basket",
      "title": "把一周安排明白",
      "text": "生成一周午晚餐菜单，合并买菜清单；和家人共享菜谱与计划，让准备一餐更轻松。"
    }
  ],
  "steps": [
    {
      "number": "01",
      "title": "先填饮食偏好",
      "text": "设好忌口、辣度和能接受的做饭时间。",
      "web": "/me/preferences",
      "mini": "/pages/preferences/preferences"
    },
    {
      "number": "02",
      "title": "挑一道今天想做的菜",
      "text": "浏览菜谱，打开详情查看食材和步骤。",
      "web": "/dishes",
      "mini": "/pages/dishes/dishes"
    },
    {
      "number": "03",
      "title": "记下自己的拿手做法",
      "text": "从“我的私房菜”进入，新建或继续编辑。",
      "web": "/dishes?scope=mine",
      "mini": "/pages/dishes/dishes?scope=mine"
    },
    {
      "number": "04",
      "title": "安排菜单，一次买齐",
      "text": "在一周菜单里挑选计划，按清单准备食材。",
      "web": "/plan",
      "mini": "/pages/plan/plan"
    }
  ],
  "questions": [
    {
      "title": "我的私房菜，别人能看到吗？",
      "text": "新建的私房菜默认仅自己可见，在“我的 → 我的私房菜”查看。主动共享给家庭后，家庭成员才能看到相应菜谱。"
    },
    {
      "title": "食谱写到一半，可以下次继续吗？",
      "text": "可以。未提交的内容会自动保存为当前设备的草稿，再次编辑时可恢复。草稿不会跨设备同步；清理浏览器或微信本地存储会清除草稿。"
    },
    {
      "title": "AI 助手每天可以用几次？",
      "text": "每个账号每日最多 20 次，Web 与小程序共用，北京时间 00:00 重置。管理员可在 0–20 次之间调整，当前次数以对话页为准，也受全站总额度限制。每条已受理的提问计 1 次，停止生成也会计入；新建或删除对话不会恢复次数。"
    },
    {
      "title": "图片和视频链接怎么用？",
      "text": "编辑菜谱时可以添加照片。只有你主动选择图片或拍照时才会上传对应照片。视频链接支持抖音和哔哩哔哩，按菜谱详情页的提示打开对应平台播放。"
    }
  ]
}

require("../../utils/theme").page({
  data: { content, taglineLines: content.tagline.split("，").map((line, index) => line + (index === 0 ? "，" : "")) },
  go(event) {
    const url = event.currentTarget.dataset.url
    if (!url) return
    if (url.indexOf("/pages/dishes/dishes") === 0) {
      wx.setStorageSync("ninimenu_dishes_scope", url.indexOf("scope=mine") >= 0 ? "mine" : "all")
      wx.switchTab({ url: "/pages/dishes/dishes" })
    } else wx.navigateTo({ url })
  },
  onShareAppMessage() {
    return { title: "认识 arre食谱推荐小助手", path: "/pages/about/about" }
  },
})
