package agent

import (
	"encoding/json"
	"errors"
	"ninimenu/internal/auth"
	"ninimenu/internal/services"
)

func init() {
	register(&Tool{
		Name: "list_food_journal", Title: "查看饮食日记",
		Description: "读取当前用户手动记录的早餐、午餐、晚餐和加餐，包含菜名、菜系、食物类别与备注。只代表用户实际填写的内容。",
		Scope:       auth.ScopeRecordsRead,
		Schema:      object(map[string]any{"from": str("开始日期 YYYY-MM-DD"), "to": str("结束日期 YYYY-MM-DD")}),
		Handler: func(ctx *Ctx, args json.RawMessage) (any, error) {
			in, err := decode[struct{ From, To string }](args)
			if err != nil {
				return nil, err
			}
			if in.From == "" && in.To == "" {
				in.From = services.Today()
			}
			items, err := services.ListFoodJournal(ctx.UID(), in.From, in.To)
			if err != nil {
				return nil, err
			}
			total := len(items)
			if len(items) > 50 {
				items = items[:50]
			}
			return map[string]any{"items": items, "total": total, "truncated": total > len(items)}, nil
		},
	})
	register(&Tool{
		Name: "log_food_journal", Title: "记录实际饮食",
		Description: "用户明确说自己实际吃了什么时记录一餐。菜系和食物类别只填写用户提供或可确认的信息，不根据菜名猜测营养或热量。",
		Scope:       auth.ScopeRecordsWrite, Write: true,
		Schema: object(map[string]any{
			"meal_date": str("用餐日期 YYYY-MM-DD，默认今天"),
			"meal_type": str("餐次", "breakfast", "lunch", "dinner", "snack"),
			"dish_name": str("实际吃的食物名称"), "cuisine": str("用户确认的菜系，可留空"),
			"food_groups": strArray("用户确认的类别：vegetable/fruit/protein/whole_grain/dairy"),
			"notes":       str("用户补充的备注"),
		}, "meal_type", "dish_name"),
		Handler: func(ctx *Ctx, args json.RawMessage) (any, error) {
			in, err := decode[services.FoodJournalInput](args)
			if err != nil {
				return nil, err
			}
			return services.CreateFoodJournalEntry(ctx.UID(), in)
		},
	})
	register(&Tool{
		Name: "delete_food_journal", Title: "删除饮食日记",
		Description: "用户明确要求删除本人某条手动饮食记录时使用；先通过 list_food_journal 确认 ID。",
		Scope:       auth.ScopeRecordsWrite, Write: true,
		Schema: object(map[string]any{"id": integer("饮食日记 ID", 1, 1<<31-1)}, "id"),
		Handler: func(ctx *Ctx, args json.RawMessage) (any, error) {
			in, err := decode[struct {
				ID uint `json:"id"`
			}](args)
			if err != nil {
				return nil, err
			}
			if in.ID == 0 {
				return nil, errors.New("请提供记录 ID")
			}
			if err := services.DeleteFoodJournalEntry(ctx.UID(), in.ID); err != nil {
				return nil, err
			}
			return map[string]any{"deleted": true, "id": in.ID}, nil
		},
	})
	register(&Tool{
		Name: "get_health_report", Title: "饮食报告与推荐",
		Description: "根据当前用户实际记录的饮食生成近 7 或 30 天报告，包含记录天数、菜系分布、用户标注的食物类别、温和的规划提示和符合过敏原限制的菜谱推荐。没有记录的信息不能推断为没有吃，也不提供热量或诊断。",
		Scopes:      []string{auth.ScopeRecordsRead, auth.ScopeProfileRead, auth.ScopeDishesRead},
		Schema:      object(map[string]any{"days": integer("报告窗口，只能为 7 或 30，默认 7", 7, 30)}),
		Handler: func(ctx *Ctx, args json.RawMessage) (any, error) {
			in, err := decode[struct {
				Days int `json:"days"`
			}](args)
			if err != nil {
				return nil, err
			}
			if in.Days == 0 {
				in.Days = 7
			}
			if in.Days != 7 && in.Days != 30 {
				return nil, errors.New("只支持最近 7 天或 30 天")
			}
			report, err := services.BuildHealthReport(ctx.UID(), in.Days)
			if err != nil {
				return nil, err
			}
			recommendations := make([]map[string]any, 0, len(report.Recommendations))
			for _, item := range report.Recommendations {
				recommendations = append(recommendations, map[string]any{
					"dish_id": item.Dish.ID, "dish_name": item.Dish.Name, "reason": item.Reason,
				})
			}
			return map[string]any{
				"period_days": report.PeriodDays, "from": report.From, "to": report.To,
				"logged_days": report.LoggedDays, "meal_count": report.MealCount,
				"cuisine_counts": report.CuisineCounts, "food_group_days": report.FoodGroupDays,
				"insights": report.Insights, "plan_actions": report.PlanActions,
				"recommendations": recommendations,
				"note":            "仅根据实际记录整理；未记录不等于未吃。不推断热量或诊断。",
			}, nil
		},
	})
}
