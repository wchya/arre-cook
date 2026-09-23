package achievements

import "ninimenu/internal/models"

func DefaultAchievements() []models.Achievement {
	return []models.Achievement{
		auto("first_pick", "初入厨房", "记录第一道菜", "🍳"),
		auto("record_3", "三餐起步", "累计记录3道菜", "🥢"),
		auto("record_10", "十全十美", "累计记录10道菜", "🔟"),
		auto("record_20", "饭桌常客", "累计记录20道菜", "🍽️"),
		auto("record_50", "半百饭局", "累计记录50道菜", "🏅"),
		auto("record_100", "百味人生", "累计记录100道菜", "💯"),
		auto("record_200", "厨房长跑", "累计记录200道菜", "🏃"),

		auto("distinct_3", "尝鲜入门", "吃过3道不同菜", "🌱"),
		auto("distinct_10", "十味收藏", "吃过10道不同菜", "✨"),
		auto("distinct_30", "菜单探索者", "吃过30道不同菜", "🧭"),
		auto("distinct_50", "五十味图鉴", "吃过50道不同菜", "📖"),
		auto("hundred_dishes", "百菜斩", "吃过100道不同菜", "💯"),

		auto("streak_3", "三天不断炊", "连续3天有点菜记录", "🔥"),
		auto("seven_days", "连续七天", "连续7天有点菜记录", "🔥"),
		auto("streak_14", "双周饭搭子", "连续14天有点菜记录", "📆"),
		auto("streak_30", "月度厨房王", "连续30天有点菜记录", "🏆"),

		auto("lunch_5", "午餐小队", "记录5次午餐", "🍳"),
		auto("lunch_20", "午间主理人", "记录20次午餐", "🌞"),
		auto("lunch_50", "午餐专家", "记录50次午餐", "🥇"),
		auto("dinner_5", "晚餐小队", "记录5次晚餐", "🍲"),
		auto("dinner_20", "夜饭主理人", "记录20次晚餐", "🌙"),
		auto("dinner_50", "晚餐专家", "记录50次晚餐", "🥇"),
		auto("full_day_once", "一日两餐", "同一天记录午餐和晚餐", "🌓"),
		auto("full_day_3", "规律开饭", "3天同时记录午餐和晚餐", "📋"),
		auto("full_day_10", "饭点稳定器", "10天同时记录午餐和晚餐", "⏱️"),

		auto("home_chef", "居家大厨", "记录20道川菜或湘菜", "🏠"),
		auto("sichuan_rookie", "川味初探", "记录5道川菜", "🌶️"),
		auto("sichuan_master", "川菜大师", "记录10道川菜", "🔥"),
		auto("hunan_rookie", "湘味初探", "记录5道湘菜", "🌶️"),
		auto("hunan_master", "湘菜大师", "记录10道湘菜", "🫕"),
		auto("guizhou_rookie", "黔味初探", "记录5道贵州菜", "🍲"),
		auto("guizhou_master", "酸汤大师", "记录10道贵州菜", "🥘"),
		auto("yunnan_rookie", "滇味初探", "记录5道云南菜", "🍄"),
		auto("cantonese_rookie", "粤味初探", "记录5道粤菜", "🐟"),
		auto("cantonese_master", "粤菜大师", "记录10道粤菜", "🥘"),
		auto("quick_cook", "快手达人", "连续3次记录快手菜", "⚡"),
		auto("quick_master", "十五分钟战神", "记录20道快手菜", "⏲️"),
		auto("south_explorer", "南方菜系探索者", "尝试3种不同菜系", "🗺️"),
		auto("foodie_explorer", "美食探险家", "尝试5种不同菜系", "🧭"),

		auto("easy_10", "轻松下厨", "记录10道简单菜", "🙂"),
		auto("medium_10", "稳扎稳打", "记录10道中等难度菜", "🧑‍🍳"),
		auto("hard_first", "挑战开始", "记录第一道困难菜", "💪"),
		auto("hard_10", "硬菜担当", "记录10道困难菜", "👨‍🍳"),
		auto("weekend_chef", "周末大厨", "周末记录困难菜", "👨‍🍳"),
		auto("sunday_feast", "周日盛宴", "周末累计记录3道菜", "🍽️"),

		auto("lightning_10", "闪电出餐", "记录10道15分钟内菜品", "⚡"),
		auto("slow_cook_5", "慢炖时光", "记录5道45分钟以上菜品", "🕰️"),
		auto("midnight_snack", "夜宵达人", "深夜记录过菜品", "🌙"),
		auto("breakfast_king", "早餐之王", "早晨记录过菜品", "🌅"),

		auto("spicy_life", "火辣人生", "连续3次记录辣味菜", "🌶️"),
		auto("spicy_master_10", "辣味收藏家", "记录10道辣味菜", "🔥"),
		auto("sweet_tooth_5", "甜口拥护者", "记录5道甜味菜", "🍬"),
		auto("sour_explorer_5", "酸味探索家", "记录5道酸味菜", "🍋"),
		auto("umami_10", "鲜味雷达", "记录10道鲜味菜", "🐟"),
		auto("light_diet_10", "清淡生活家", "记录10道清淡菜", "🥗"),
		auto("healthy_life", "养生达人", "记录10道健康取向菜品", "🥗"),
		auto("taste_master", "味觉大师", "尝试6种不同口味", "👅"),

		auto("first_review", "第一次点评", "给菜品写下第一次评价", "💬"),
		auto("review_5", "认真吃饭", "评价5道菜", "⭐"),
		auto("review_20", "点评达人", "评价20道菜", "🌟"),
		auto("meal_remark_10", "饭后有感", "留下10条文字备注", "📝"),
		auto("yummy_10", "真香时刻", "记录10次好吃评价", "😋"),
		auto("not_again_3", "踩雷记录员", "记录3次不想再吃", "🧯"),
		auto("first_day_rating", "整餐复盘", "完成第一次整餐评价", "🍚"),
		auto("day_rating_7", "一周复盘", "完成7天整餐评价", "📔"),

		auto("happy_home_5", "开心开饭", "记录5次开心心情", "😊"),
		auto("tired_care_3", "疲惫也要吃好", "疲惫时记录3次心情", "😫"),
		auto("lazy_easy_3", "偷懒有理", "想偷懒心情累计3次", "😌"),
		auto("spicy_mood_5", "今天就想吃辣", "想吃辣心情累计5次", "🤤"),
		auto("healthy_mood_5", "养生信号", "想养生心情累计5次", "🌿"),
		auto("seasonal_eater", "时令美食家", "四个季节都有记录", "🌸"),

		auto("first_photo", "第一张饭照", "上传第一张用餐照片", "📸"),
		auto("photo_wall_3_days", "回忆上墙", "3天留下用餐照片", "🖼️"),
		auto("photo_collector_10", "照片收藏家", "累计上传10张用餐照片", "📷"),
		auto("photo_album_30", "美食相册", "累计上传30张用餐照片", "📚"),

		auto("first_favorite", "心头好", "收藏第一道菜", "❤"),
		auto("favorite_5", "私房菜单", "收藏5道菜", "💖"),
		auto("favorite_10", "收藏夹满员", "收藏10道菜", "💎"),

		auto("first_shopping_check", "买菜开张", "勾选第一项买菜清单", "🛒"),
		auto("shopping_10", "采购熟手", "勾选10项买菜清单", "✅"),
		auto("shopping_50", "采购大师", "勾选50项买菜清单", "🧺"),
		auto("pantry_first", "家中有粮", "标记第一项家中库存", "🏡"),
		auto("pantry_10", "库存管家", "标记10项家中库存", "📦"),

		auto("first_recommend", "选择困难终结者", "触发第一次推荐", "🎯"),
		auto("recommend_10", "推荐常客", "触发10次推荐", "🎲"),
		auto("recommend_50", "灵感不断", "触发50次推荐", "🪄"),
		auto("lunch_recommend_10", "午餐灵感王", "触发10次午餐推荐", "🍳"),
		auto("dinner_recommend_10", "晚餐灵感王", "触发10次晚餐推荐", "🍲"),
		auto("mood_recommend_5", "心情点菜师", "触发5次心情推荐", "🎭"),
		auto("blind_box_first", "盲盒初体验", "打开第一次惊喜盲盒", "🎁"),
		auto("blind_box_10", "惊喜收藏家", "打开10次惊喜盲盒", "🎪"),
		auto("week_plan_first", "本周安排上", "生成一周菜单", "📅"),
		auto("week_plan_5", "计划型吃货", "生成5次一周菜单", "🗓️"),
		auto("agent_recommend_first", "AI 点菜初体验", "第一次让 AI 推荐官帮你点菜", "🤖"),
		auto("agent_recommend_10", "AI 饭搭子", "AI 推荐官帮你点了10次菜", "🧠"),
		auto("agent_accept_5", "从善如流", "采纳5次 AI 推荐", "🤝"),

		auto("dish_library_10", "菜单起步", "菜品库达到10道菜", "📒"),
		auto("dish_library_30", "菜单扩容", "菜品库达到30道菜", "📚"),
		auto("dish_library_50", "菜单博物馆", "菜品库达到50道菜", "🏛️"),
		auto("image_library_10", "有图有真相", "10道菜拥有图片", "🖼️"),
		auto("video_recipe_1", "视频课开张", "录入第一条视频教程", "▶️"),
		auto("ingredient_ready_10", "备料清楚", "10道菜录入配料", "🥬"),
		auto("rich_recipe_10", "步骤控", "10道菜录入制作步骤", "📝"),
		auto("category_builder_5", "菜单架构师", "菜品库覆盖5种菜系", "🗂️"),

		auto("top_dish_5", "有个老朋友", "同一道菜记录5次", "🤝"),
		auto("top_dish_10", "本命菜出现", "同一道菜记录10次", "👑"),
		auto("month_regular_10", "月度常客", "同一个月记录10道菜", "📆"),
		auto("season_regular_30", "季度食客", "同一季节记录30道菜", "🍂"),
	}
}

func auto(code, name, description, icon string) models.Achievement {
	return models.Achievement{
		Code:        code,
		Name:        name,
		Description: description,
		Icon:        icon,
		Condition:   "auto",
	}
}

// RetiredCodes 随“只保留南方菜系”裁剪一起下线的旧成就编码。
func RetiredCodes() []string {
	return []string{
		"home_food_5", "soup_lover", "soup_master", "staple_runner", "snack_collector", "category_collector",
	}
}
