package agent

import (
	"encoding/json"
	"ninimenu/internal/auth"
	"ninimenu/internal/database"
	"ninimenu/internal/services"
)

func init() {
	register(&Tool{
		Name: "recommendation_feedback", Title: "推荐展示与反馈",
		Description: "记录用户真正看见或明确反馈的推荐项。impression=实际展示（预取不算），skip=换一个（不降低长期喜好），not_today=今天不想吃，dislike=明确长期不喜欢，undo_dislike=撤销不喜欢。使用 recommend_dishes 返回的 recommendation_id 与真实 dish_id；request_key 重试必须复用。不要把换一个当 dislike，也不要推测用户反馈。accept由plan_meal可靠记录，无需另报。",
		Scope:       auth.ScopeBehaviorWrite, Write: true,
		Schema: object(map[string]any{
			"recommendation_id": str("推荐结果ID；仅直接表达长期不喜欢及撤销可省略"),
			"dish_id":           integer("推荐里的菜品 ID", 1, 1<<31-1),
			"event_type":        str("动作", "impression", "skip", "not_today", "dislike", "undo_dislike"),
			"request_key":       str("本次动作的唯一键，重试复用，最多128字节"),
			"note":              str("用户明确表达的原话，可选"),
		}, "dish_id", "event_type"),
		Handler: func(ctx *Ctx, args json.RawMessage) (any, error) {
			in, err := decode[services.RecommendationFeedbackInput](args)
			if err != nil {
				return nil, err
			}
			return services.RecordRecommendationFeedback(ctx.UID(), in, ctx.Source(), actorName(ctx), database.DB.WithContext(ctx.Context))
		},
	})
	register(&Tool{
		Name: "list_meal_plans", Title: "已确认菜单",
		Description: "读取用户明确选好的菜单，与实际吃过的记录分开。含计划日期、餐次、菜品、状态及当前是否可用；生成的随机周菜单不是确认计划。",
		Scope:       auth.ScopeRecordsRead,
		Schema:      object(map[string]any{"date_from": str("开始日期 YYYY-MM-DD，默认今天"), "date_to": str("结束日期 YYYY-MM-DD，默认未来七天末日")}),
		Handler: func(ctx *Ctx, args json.RawMessage) (any, error) {
			in, err := decode[struct {
				From string `json:"date_from"`
				To   string `json:"date_to"`
			}](args)
			if err != nil {
				return nil, err
			}
			return services.ListMealPlans(ctx.UID(), in.From, in.To, database.DB.WithContext(ctx.Context))
		},
	})
	register(&Tool{
		Name: "plan_meal", Title: "加入菜单",
		Description: "用户明确说准备吃、今晚吃、明天吃或加入菜单时，把一道可见且符合当前忌口的菜加入今天起七天内的计划，并同步买菜清单。同菜同日期餐次重试幂等，不会记为已吃或学习为吃后好评。recommendation_id可关联本次推荐；实际吃完请再用log_meal。",
		Scope:       auth.ScopePlanWrite, Write: true,
		Schema: object(map[string]any{
			"dish_id": integer("菜品ID", 1, 1<<31-1), "meal_date": str("日期 YYYY-MM-DD，默认今天"),
			"meal_type": str("餐次", "breakfast", "lunch", "dinner"), "recommendation_id": str("对应推荐结果ID，可选"),
		}, "dish_id", "meal_type"),
		Handler: func(ctx *Ctx, args json.RawMessage) (any, error) {
			in, err := decode[services.MealPlanInput](args)
			if err != nil {
				return nil, err
			}
			in.Source, in.Actor = ctx.Source(), actorName(ctx)
			return services.AcceptMealPlan(ctx.UID(), in, database.DB.WithContext(ctx.Context))
		},
	})
	register(&Tool{
		Name: "replace_meal_plan", Title: "替换整餐计划",
		Description: "用户明确要求替换某一天某一餐菜单时调用。dish_ids是这一餐最终保留的所有菜，整餐原子替换，未列出的旧计划会撤销；空数组表示清空。不会删除真实用餐记录或其他餐次。",
		Scope:       auth.ScopePlanWrite, Write: true,
		Schema: object(map[string]any{
			"meal_date": str("日期 YYYY-MM-DD"), "meal_type": str("餐次", "breakfast", "lunch", "dinner"), "dish_ids": intArray("最终保留的菜品 ID，最多10道"),
			"recommendation_ids": map[string]any{"type": "object", "description": "每道菜对应的推荐ID，键为dish_id；手动选菜可省略", "additionalProperties": map[string]any{"type": "string"}},
		}, "meal_date", "meal_type", "dish_ids"),
		Handler: func(ctx *Ctx, args json.RawMessage) (any, error) {
			in, err := decode[services.MealPlanReplaceInput](args)
			if err != nil {
				return nil, err
			}
			if in.DishIDs == nil {
				return nil, services.ErrInvalidMealPlan
			}
			in.Source, in.Actor = ctx.Source(), actorName(ctx)
			return services.ReplaceMealPlan(ctx.UID(), in, database.DB.WithContext(ctx.Context))
		},
	})
	register(&Tool{
		Name: "cancel_meal_plan", Title: "撤销一道计划菜",
		Description: "用户明确要求撤销一道已确认计划时调用，只撤销计划，不删除实际吃过的记录。plan_id来自list_meal_plans。",
		Scope:       auth.ScopePlanWrite, Write: true,
		Schema: object(map[string]any{"plan_id": integer("计划ID", 1, 1<<31-1)}, "plan_id"),
		Handler: func(ctx *Ctx, args json.RawMessage) (any, error) {
			in, err := decode[struct {
				PlanID uint `json:"plan_id"`
			}](args)
			if err != nil {
				return nil, err
			}
			if err := services.CancelMealPlan(ctx.UID(), in.PlanID, database.DB.WithContext(ctx.Context)); err != nil {
				return nil, err
			}
			return map[string]any{"ok": true}, nil
		},
	})
}
