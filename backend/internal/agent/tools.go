package agent

import (
	"encoding/json"
	"errors"
	"ninimenu/internal/auth"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"ninimenu/internal/services"
	"strings"
	"time"
)

var weekdayNames = []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}

func init() {
	register(&Tool{
		Name:        "get_context",
		Title:       "当前上下文",
		Description: "获取当前用户与时间上下文：昵称、今天日期、星期、当前时段建议的餐次（lunch/dinner）、今天已记录的餐。开始对话或做推荐前先调用。",
		Scope:       auth.ScopeRecordsRead,
		Handler: func(ctx *Ctx, _ json.RawMessage) (any, error) {
			now := time.Now()
			meal := "dinner"
			if now.Hour() < 14 {
				meal = "lunch"
			}
			var today []models.MealRecord
			database.DB.Scopes(database.OwnedBy(ctx.UID())).Where("meal_date = ?", services.Today()).Find(&today)
			eaten := make([]map[string]any, 0, len(today))
			for _, r := range today {
				eaten = append(eaten, map[string]any{"record_id": r.ID, "dish_id": r.DishID, "dish_name": r.DishName, "meal_type": r.MealType})
			}
			return map[string]any{
				"nickname":          ctx.Principal.User.DisplayName(),
				"today":             services.Today(),
				"tomorrow":          now.AddDate(0, 0, 1).Format("2006-01-02"),
				"weekday":           weekdayNames[now.Weekday()],
				"time":              now.Format("15:04"),
				"suggested_meal":    meal,
				"today_meals":       eaten,
				"granted_scopes":    ctx.Principal.Scopes.List(),
				"repeat_avoid_days": services.UserRepeatDays(ctx.UID()),
			}, nil
		},
	})

	register(&Tool{
		Name:        "get_taste_profile",
		Title:       "口味画像",
		Description: "获取用户口味画像：由用餐记录（时间衰减加权）、评价、收藏、心情、行为事件与显式偏好聚合，含口味/菜系权重、辣味占比、常吃食材、喜欢/踩雷的菜、近期已吃、采纳率与一句话总结。",
		Scope:       auth.ScopeProfileRead,
		Schema:      object(map[string]any{"days": integer("统计窗口天数，默认 90", 7, 365)}),
		Handler: func(ctx *Ctx, args json.RawMessage) (any, error) {
			in, err := decode[struct {
				Days int `json:"days"`
			}](args)
			if err != nil {
				return nil, err
			}
			p := services.BuildTasteProfile(ctx.UID(), in.Days)
			return p, nil
		},
	})

	register(&Tool{
		Name:        "get_preferences",
		Title:       "饮食偏好",
		Description: "获取用户显式设置的饮食偏好：忌口、过敏原、喜欢的口味、辣度(-1未设置/0不吃辣/1微辣/2中辣/3特辣)、就餐人数、做饭时长上限、饮食目标与备注。推荐时必须遵守过敏原与忌口。",
		Scope:       auth.ScopeProfileRead,
		Handler: func(ctx *Ctx, _ json.RawMessage) (any, error) {
			return services.GetPreferences(ctx.UID()), nil
		},
	})

	register(&Tool{
		Name:        "update_preferences",
		Title:       "更新饮食偏好",
		Description: "更新用户的饮食偏好，只传需要修改的字段。数组字段为整体替换（如需追加忌口，先 get_preferences 再合并）。仅在用户明确表达偏好时调用。",
		Scope:       auth.ScopePreferencesWrite,
		Write:       true,
		Schema: object(map[string]any{
			"avoid_ingredients": strArray("忌口食材，如 香菜、内脏"),
			"allergies":         strArray("过敏原，如 花生、虾"),
			"favorite_tastes":   strArray("喜欢的口味，如 麻辣、酸辣"),
			"spice_level":       integer("辣度：-1 未设置，0 不吃辣，1 微辣，2 中辣，3 特辣", -1, 3),
			"household_size":    integer("通常几个人吃饭", 0, 20),
			"max_cook_time":     integer("单道菜愿意花的最长时间（分钟），0 不限", 0, 240),
			"goals":             str("饮食目标，如 减脂、增肌、控糖"),
			"notes":             str("其他补充说明"),
		}),
		Handler: func(ctx *Ctx, args json.RawMessage) (any, error) {
			patch, err := decode[services.PreferencesPatch](args)
			if err != nil {
				return nil, err
			}
			return services.SavePreferences(ctx.UID(), patch)
		},
	})

	register(&Tool{
		Name:        "search_dishes",
		Title:       "搜索菜品",
		Description: "在用户可见的菜品库（公共菜谱 + 用户私房菜）中检索，返回菜品摘要。支持关键词（菜名/食材/标签，空格分隔表示同时满足）、菜系、口味、必含/排除食材、最长耗时、难度、餐次、只看收藏。",
		Scope:       auth.ScopeDishesRead,
		Schema: object(map[string]any{
			"keyword":             str("关键词，如 “牛肉 下饭”"),
			"categories":          strArray("菜系，如 川菜、湘菜、贵州菜、云南菜、粤菜"),
			"tastes":              strArray("口味，如 麻辣、酸辣、清淡"),
			"ingredients":         strArray("必须包含的食材（如冰箱里有的）"),
			"exclude_ingredients": strArray("排除的食材"),
			"max_cook_time":       integer("最长烹饪分钟数", 0, 600),
			"difficulty":          str("难度", "easy", "medium", "hard"),
			"meal_type":           str("餐次", "lunch", "dinner"),
			"only_favorites":      boolean("只看收藏"),
			"exclude_recent":      boolean("排除最近几天已经吃过的菜"),
			"limit":               integer("返回条数，默认 10", 1, 50),
		}),
		Handler: func(ctx *Ctx, args json.RawMessage) (any, error) {
			q, err := decode[services.DishQuery](args)
			if err != nil {
				return nil, err
			}
			if q.Limit <= 0 {
				q.Limit = 10
			}
			if q.Limit > 50 {
				q.Limit = 50
			}
			q.IDs, q.Offset, q.IncludeDisabled = nil, 0, false
			list, total := services.SearchDishes(ctx.UID(), q)
			return map[string]any{"total": total, "dishes": services.ToDishCards(list)}, nil
		},
	})

	register(&Tool{
		Name:        "get_dish",
		Title:       "菜品详情",
		Description: "获取一道菜的完整信息（食材用量、调料、步骤、耗时）以及该用户吃这道菜的历史统计。用户问做法时调用。",
		Scope:       auth.ScopeDishesRead,
		Schema:      object(map[string]any{"dish_id": integer("菜品 ID", 1, 1<<31-1)}, "dish_id"),
		Handler: func(ctx *Ctx, args json.RawMessage) (any, error) {
			in, err := decode[struct {
				DishID uint `json:"dish_id"`
			}](args)
			if err != nil {
				return nil, err
			}
			dish, err := services.FindVisibleDish(ctx.UID(), in.DishID)
			if err != nil {
				return nil, services.ErrDishNotFound
			}
			stats, _ := services.DishStatsForUser(ctx.UID(), dish.ID)
			dish.Favorite = stats.Favorite
			return map[string]any{"dish": dish, "my_stats": stats}, nil
		},
	})

	register(&Tool{
		Name:        "recommend_dishes",
		Title:       "智能推荐",
		Description: "调用站内推荐引擎：按用户口味画像 + 显式偏好（过敏原/忌口硬过滤）+ 你给的约束打分，自动避开最近吃过的菜并保证菜系多样，返回带推荐理由的菜品。给用户推荐菜时优先使用它，而不是自己编菜名。",
		Scope:       auth.ScopeDishesRead,
		Scopes:      []string{auth.ScopeDishesRead, auth.ScopeProfileRead},
		Schema: object(map[string]any{
			"meal_type":           str("餐次", "lunch", "dinner"),
			"mood":                str("心情：happy 开心/tired 累/lazy 想偷懒/spicy 想吃辣/healthy 想吃清淡", "happy", "tired", "lazy", "spicy", "healthy"),
			"count":               integer("推荐几道，默认 3", 1, 10),
			"keyword":             str("关键词"),
			"tastes":              strArray("想要的口味"),
			"categories":          strArray("限定菜系"),
			"max_cook_time":       integer("最长烹饪分钟数", 0, 600),
			"difficulty":          str("难度", "easy", "medium", "hard"),
			"include_ingredients": strArray("必须用到的食材"),
			"exclude_ingredients": strArray("本次额外排除的食材"),
			"exclude_dish_ids":    intArray("排除的菜品 ID（如用户刚拒绝的）"),
		}),
		Handler: func(ctx *Ctx, args json.RawMessage) (any, error) {
			req, err := decode[services.RecommendRequest](args)
			if err != nil {
				return nil, err
			}
			req.Source, req.Actor, req.Mode = ctx.Source(), actorName(ctx), "agent_tool"
			req.IgnorePreferences, req.ProfileDays = false, 0
			res, err := services.RecommendDishes(ctx.UID(), req)
			if err != nil {
				return nil, err
			}
			items := make([]map[string]any, 0, len(res.Items))
			for _, it := range res.Items {
				items = append(items, map[string]any{"dish": services.ToDishCard(it.Dish), "score": it.Score, "reasons": it.Reasons})
			}
			return map[string]any{"items": items, "candidate_count": res.CandidateCount, "profile_summary": res.ProfileSummary, "applied": res.Applied}, nil
		},
	})

	register(&Tool{
		Name:        "list_meal_records",
		Title:       "用餐记录",
		Description: "查询用户的用餐记录（默认最近 30 天），含每餐的评价（mood: yum 好吃/ok 一般/no 不想再吃）与备注。",
		Scope:       auth.ScopeRecordsRead,
		Schema: object(map[string]any{
			"date_from": str("开始日期 YYYY-MM-DD"),
			"date_to":   str("结束日期 YYYY-MM-DD"),
			"meal_type": str("餐次", "lunch", "dinner"),
			"dish_id":   integer("只看某道菜", 0, 1<<31-1),
			"limit":     integer("条数，默认 50", 1, 500),
		}),
		Handler: func(ctx *Ctx, args json.RawMessage) (any, error) {
			in, err := decode[struct {
				DateFrom string `json:"date_from"`
				DateTo   string `json:"date_to"`
				MealType string `json:"meal_type"`
				DishID   uint   `json:"dish_id"`
				Limit    int    `json:"limit"`
			}](args)
			if err != nil {
				return nil, err
			}
			q := database.DB.Scopes(database.OwnedBy(ctx.UID()))
			if in.DateFrom == "" && in.DateTo == "" {
				in.DateFrom = time.Now().AddDate(0, 0, -30).Format("2006-01-02")
			}
			if in.DateFrom != "" {
				q = q.Where("meal_date >= ?", in.DateFrom)
			}
			if in.DateTo != "" {
				q = q.Where("meal_date <= ?", in.DateTo)
			}
			if in.MealType != "" {
				q = q.Where("meal_type = ?", in.MealType)
			}
			if in.DishID > 0 {
				q = q.Where("dish_id = ?", in.DishID)
			}
			if in.Limit <= 0 || in.Limit > 500 {
				in.Limit = 50
			}
			var records []models.MealRecord
			q.Order("meal_date DESC, created_at DESC").Limit(in.Limit).Find(&records)
			out := make([]map[string]any, 0, len(records))
			for _, r := range records {
				out = append(out, map[string]any{
					"record_id": r.ID, "dish_id": r.DishID, "dish_name": r.DishName, "meal_type": r.MealType,
					"meal_date": r.MealDate, "mood": r.Mood, "rating": r.Rating, "remark": r.Remark,
				})
			}
			return map[string]any{"records": out, "count": len(out)}, nil
		},
	})

	register(&Tool{
		Name:        "log_meal",
		Title:       "记一餐",
		Description: "把一道菜记入用户某天的午餐或晚餐（同时生成买菜清单）。仅在用户明确说“就吃这个/帮我记上/记一下”时调用。meal_date 可用 YYYY-MM-DD、today、tomorrow。",
		Scope:       auth.ScopeRecordsWrite,
		Write:       true,
		Schema: object(map[string]any{
			"dish_id":   integer("菜品 ID", 1, 1<<31-1),
			"meal_type": str("餐次", "lunch", "dinner"),
			"meal_date": str("日期，默认 today"),
			"mood":      str("评价（吃完后才填）", "yum", "ok", "no"),
			"remark":    str("备注"),
		}, "dish_id", "meal_type"),
		Handler: func(ctx *Ctx, args json.RawMessage) (any, error) {
			in, err := decode[services.MealInput](args)
			if err != nil {
				return nil, err
			}
			rec, err := services.CreateMealRecord(ctx.UID(), in, ctx.Source(), actorName(ctx))
			if err != nil {
				return nil, err
			}
			return map[string]any{"ok": true, "record_id": rec.ID, "dish_name": rec.DishName, "meal_type": rec.MealType, "meal_date": rec.MealDate}, nil
		},
	})

	register(&Tool{
		Name:        "rate_meal",
		Title:       "评价一餐",
		Description: "给已记录的一餐打评价或写备注（mood: yum 好吃 / ok 一般 / no 不想再吃）。",
		Scope:       auth.ScopeRecordsWrite,
		Write:       true,
		Schema: object(map[string]any{
			"record_id": integer("用餐记录 ID", 1, 1<<31-1),
			"mood":      str("评价", "yum", "ok", "no"),
			"rating":    integer("1-5 分", 0, 5),
			"remark":    str("备注"),
		}, "record_id"),
		Handler: func(ctx *Ctx, args json.RawMessage) (any, error) {
			in, err := decode[struct {
				RecordID uint    `json:"record_id"`
				Mood     *string `json:"mood"`
				Rating   *int    `json:"rating"`
				Remark   *string `json:"remark"`
			}](args)
			if err != nil {
				return nil, err
			}
			rec, err := services.UpdateMealRecord(ctx.UID(), in.RecordID, services.MealPatch{Mood: in.Mood, Rating: in.Rating, Remark: in.Remark})
			if err != nil {
				return nil, err
			}
			return map[string]any{"ok": true, "record_id": rec.ID, "mood": rec.Mood, "rating": rec.Rating}, nil
		},
	})

	register(&Tool{
		Name:        "delete_meal_record",
		Title:       "删除一餐",
		Description: "删除一条用餐记录（例如记错了）。先用 list_meal_records 或 get_context 找到 record_id，并向用户确认后再删。",
		Scope:       auth.ScopeRecordsWrite,
		Write:       true,
		Schema:      object(map[string]any{"record_id": integer("用餐记录 ID", 1, 1<<31-1)}, "record_id"),
		Handler: func(ctx *Ctx, args json.RawMessage) (any, error) {
			in, err := decode[struct {
				RecordID uint `json:"record_id"`
			}](args)
			if err != nil {
				return nil, err
			}
			rec, err := services.DeleteMealRecord(ctx.UID(), in.RecordID)
			if err != nil {
				return nil, err
			}
			return map[string]any{"ok": true, "deleted": rec.DishName + " · " + rec.MealDate}, nil
		},
	})

	register(&Tool{
		Name:        "list_favorites",
		Title:       "收藏列表",
		Description: "获取用户收藏的菜品。",
		Scope:       auth.ScopeRecordsRead,
		Handler: func(ctx *Ctx, _ json.RawMessage) (any, error) {
			list, total := services.SearchDishes(ctx.UID(), services.DishQuery{OnlyFavorites: true, Limit: 100})
			return map[string]any{"total": total, "dishes": services.ToDishCards(list)}, nil
		},
	})

	register(&Tool{
		Name:        "set_favorite",
		Title:       "收藏/取消收藏",
		Description: "收藏或取消收藏一道菜。",
		Scope:       auth.ScopeFavoritesWrite,
		Write:       true,
		Schema: object(map[string]any{
			"dish_id":  integer("菜品 ID", 1, 1<<31-1),
			"favorite": boolean("true 收藏，false 取消"),
		}, "dish_id", "favorite"),
		Handler: func(ctx *Ctx, args json.RawMessage) (any, error) {
			in, err := decode[struct {
				DishID   uint `json:"dish_id"`
				Favorite bool `json:"favorite"`
			}](args)
			if err != nil {
				return nil, err
			}
			if err := services.SetFavorite(ctx.UID(), in.DishID, in.Favorite); err != nil {
				return nil, err
			}
			return map[string]any{"ok": true, "dish_id": in.DishID, "favorite": in.Favorite}, nil
		},
	})

	register(&Tool{
		Name:        "get_week_plan",
		Title:       "本周菜单",
		Description: "获取用户本周（周一到周日）的午/晚餐菜单。",
		Scope:       auth.ScopeRecordsRead,
		Handler: func(ctx *Ctx, _ json.RawMessage) (any, error) {
			return compactPlan(services.GetCachedWeekPlan(ctx.UID())), nil
		},
	})

	register(&Tool{
		Name:        "regenerate_week_plan",
		Title:       "重排本周菜单",
		Description: "重新随机生成本周菜单（会避开过敏原与忌口）。",
		Scope:       auth.ScopePlanWrite,
		Write:       true,
		Handler: func(ctx *Ctx, _ json.RawMessage) (any, error) {
			plan := services.RegenerateWeekPlan(ctx.UID())
			services.RecordAchievementEvent(ctx.UID(), "week_plan", "")
			return compactPlan(plan), nil
		},
	})

	register(&Tool{
		Name:        "get_shopping_list",
		Title:       "买菜清单",
		Description: "获取今明两天已记录菜品汇总出的买菜清单（按蔬菜/肉类/配料/其他分组，含已买、家中有库存标记）。",
		Scope:       auth.ScopeRecordsRead,
		Handler: func(ctx *Ctx, _ json.RawMessage) (any, error) {
			today := services.Today()
			tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
			return services.BuildShoppingList(ctx.UID(), []string{today, tomorrow}), nil
		},
	})

	register(&Tool{
		Name:        "get_stats",
		Title:       "饮食统计",
		Description: "获取用户的饮食统计：总记录、午/晚餐次数、收藏数、吃过多少道不同的菜、连续记录天数、本月餐数、最常吃的菜、菜系分布、近 7 天趋势。",
		Scope:       auth.ScopeProfileRead,
		Handler: func(ctx *Ctx, _ json.RawMessage) (any, error) {
			return services.BuildUserStats(ctx.UID()), nil
		},
	})

	register(&Tool{
		Name:        "list_behavior_events",
		Title:       "行为事件",
		Description: "查询用户的行为事件流（view 浏览/recommend 推荐/accept 采纳/reject 拒绝/feedback 反馈等），用于分析推荐效果。",
		Scope:       auth.ScopeRecordsRead,
		Schema: object(map[string]any{
			"types": strArray("事件类型过滤"),
			"since": str("起始日期 YYYY-MM-DD，默认 30 天前"),
			"limit": integer("条数，默认 100", 1, 1000),
		}),
		Handler: func(ctx *Ctx, args json.RawMessage) (any, error) {
			in, err := decode[struct {
				Types []string `json:"types"`
				Since string   `json:"since"`
				Limit int      `json:"limit"`
			}](args)
			if err != nil {
				return nil, err
			}
			q := database.DB.Scopes(database.OwnedBy(ctx.UID()))
			if len(in.Types) > 0 {
				q = q.Where("event_type IN ?", in.Types)
			}
			since := time.Now().AddDate(0, 0, -30)
			if t, err := time.ParseInLocation("2006-01-02", in.Since, time.Local); err == nil {
				since = t
			}
			q = q.Where("created_at >= ?", since)
			if in.Limit <= 0 || in.Limit > 1000 {
				in.Limit = 100
			}
			var events []models.BehaviorEvent
			q.Order("created_at DESC").Limit(in.Limit).Find(&events)
			out := make([]map[string]any, 0, len(events))
			for _, e := range events {
				var meta any
				_ = json.Unmarshal([]byte(e.Meta), &meta)
				out = append(out, map[string]any{
					"event_type": e.EventType, "dish_id": e.DishID, "dish_name": e.DishName,
					"source": e.Source, "meta": meta, "created_at": e.CreatedAt.Format(time.RFC3339),
				})
			}
			return map[string]any{"events": out, "count": len(out)}, nil
		},
	})

	register(&Tool{
		Name:        "log_feedback",
		Title:       "记录反馈",
		Description: "记录用户对某道菜的反馈，用于改进推荐：reject=不想吃/被拒绝，feedback=一般反馈，view=看过。例如用户说“最近不想吃鱼香肉丝”。",
		Scope:       auth.ScopeBehaviorWrite,
		Write:       true,
		Schema: object(map[string]any{
			"event_type": str("事件类型", "reject", "feedback", "view", "search", "chat", "custom"),
			"dish_id":    integer("菜品 ID（可选）", 0, 1<<31-1),
			"note":       str("用户原话或说明"),
		}, "event_type"),
		Handler: func(ctx *Ctx, args json.RawMessage) (any, error) {
			in, err := decode[struct {
				EventType string `json:"event_type"`
				DishID    uint   `json:"dish_id"`
				Note      string `json:"note"`
			}](args)
			if err != nil {
				return nil, err
			}
			if !services.IsValidEventType(in.EventType) {
				return nil, errors.New("event_type 无效")
			}
			services.LogBehavior(ctx.UID(), in.EventType, in.DishID, "", ctx.Source(), actorName(ctx), map[string]any{"note": in.Note})
			return map[string]any{"ok": true}, nil
		},
	})

	register(&Tool{
		Name:  "create_suggestion",
		Title: "推送推荐建议",
		Description: "向用户的「AI 建议」收件箱推送一条推荐（例如“明晚试试这两道”），用户在 App 里一键采纳（自动记入对应餐次）或忽略，结果会回流为行为数据。" +
			"外部智能体做推荐管理时优先用它，而不是直接改用户的记录。",
		Scope: auth.ScopeSuggestionsWrite,
		Write: true,
		Schema: object(map[string]any{
			"title":            str("标题，≤60 字"),
			"reason":           str("推荐理由，会展示给用户"),
			"dish_ids":         intArray("推荐的菜品 ID（1-6 道），必须来自 search_dishes / recommend_dishes 的结果"),
			"meal_type":        str("建议的餐次", "lunch", "dinner"),
			"meal_date":        str("建议的日期 YYYY-MM-DD / today / tomorrow"),
			"expires_in_hours": integer("有效期（小时），默认 72", 1, 336),
		}, "dish_ids"),
		Handler: func(ctx *Ctx, args json.RawMessage) (any, error) {
			in, err := decode[services.SuggestionInput](args)
			if err != nil {
				return nil, err
			}
			v, err := services.CreateSuggestion(ctx.UID(), ctx.Source(), in)
			if err != nil {
				return nil, err
			}
			return map[string]any{"ok": true, "suggestion_id": v.ID, "status": v.Status, "dishes": services.ToDishCards(v.Dishes)}, nil
		},
	})

	register(&Tool{
		Name:        "list_suggestions",
		Title:       "建议列表",
		Description: "查看已推送给用户的建议及处理状态（pending 待处理/accepted 已采纳/dismissed 已忽略/expired 已过期），用于评估推荐效果。",
		Scope:       auth.ScopeRecordsRead,
		Schema:      object(map[string]any{"status": str("状态过滤，默认 all", "all", "pending", "accepted", "dismissed", "expired")}),
		Handler: func(ctx *Ctx, args json.RawMessage) (any, error) {
			in, err := decode[struct {
				Status string `json:"status"`
			}](args)
			if err != nil {
				return nil, err
			}
			list := services.ListSuggestions(ctx.UID(), in.Status, 50)
			out := make([]map[string]any, 0, len(list))
			for _, s := range list {
				names := make([]string, 0, len(s.Dishes))
				for _, d := range s.Dishes {
					names = append(names, d.Name)
				}
				out = append(out, map[string]any{
					"id": s.ID, "title": s.Title, "source": s.Source, "status": s.Status, "dishes": strings.Join(names, "、"),
					"meal_type": s.MealType, "meal_date": s.MealDate, "created_at": s.CreatedAt.Format(time.RFC3339),
				})
			}
			return out, nil
		},
	})

	register(&Tool{
		Name:        "get_day_ratings",
		Title:       "每日评价",
		Description: "获取用户每天的整体评价与首页心情（默认最近 30 天）。",
		Scope:       auth.ScopeRecordsRead,
		Schema: object(map[string]any{
			"date_from": str("开始日期 YYYY-MM-DD"),
			"date_to":   str("结束日期 YYYY-MM-DD"),
		}),
		Handler: func(ctx *Ctx, args json.RawMessage) (any, error) {
			in, err := decode[struct {
				DateFrom string `json:"date_from"`
				DateTo   string `json:"date_to"`
			}](args)
			if err != nil {
				return nil, err
			}
			if in.DateFrom == "" {
				in.DateFrom = time.Now().AddDate(0, 0, -30).Format("2006-01-02")
			}
			q := database.DB.Scopes(database.OwnedBy(ctx.UID())).Where("meal_date >= ?", in.DateFrom)
			if in.DateTo != "" {
				q = q.Where("meal_date <= ?", in.DateTo)
			}
			var ratings []models.DayRating
			q.Order("meal_date DESC").Limit(100).Find(&ratings)
			out := make([]map[string]any, 0, len(ratings))
			for _, r := range ratings {
				out = append(out, map[string]any{"date": r.MealDate, "home_mood": r.HomeMood, "day_mood": r.Mood, "remark": r.Remark})
			}
			return out, nil
		},
	})
}

func compactPlan(plan *services.WeekPlan) any {
	days := make([]map[string]any, 0, len(plan.Days))
	names := func(list []models.Dish) []map[string]any {
		out := make([]map[string]any, 0, len(list))
		for _, d := range list {
			out = append(out, map[string]any{"id": d.ID, "name": d.Name, "category": d.Category, "cook_time": d.CookTime})
		}
		return out
	}
	for _, d := range plan.Days {
		days = append(days, map[string]any{"date": d.Date, "day": d.DayName, "lunch": names(d.Lunch), "dinner": names(d.Dinner)})
	}
	return map[string]any{"days": days}
}
