package agent

import (
	"encoding/json"
	"errors"
	"ninimenu/internal/auth"
	"ninimenu/internal/services"
)

func init() {
	fields := map[string]any{
		"name": str("菜名"), "category": str("菜系"),
		"meal_type": str("适用餐次", "all", "lunch", "dinner"),
		"taste":     str("口味"), "ingredients": strArray("食材名称或名称加用量"),
		"seasonings": strArray("调料"), "steps": strArray("按顺序排列的做法步骤"),
		"cook_time":  integer("烹饪时间（分钟）", 0, 600),
		"difficulty": str("难度", "easy", "medium", "hard"), "remark": str("备注"),
	}
	register(&Tool{
		Name: "create_private_recipe", Title: "新建私房菜",
		Description: "用户明确要保存菜谱时，在当前账号下新建私房菜。只保存用户提供或确认的食材和步骤，不编造食谱。不会创建公共或家庭共享菜谱。",
		Scope:       auth.ScopeDishesWrite, Write: true, Schema: object(fields, "name"),
		Handler: func(ctx *Ctx, args json.RawMessage) (any, error) {
			patch, err := decode[services.PrivateRecipePatch](args)
			if err != nil {
				return nil, err
			}
			return services.CreatePrivateRecipe(ctx.UID(), patch)
		},
	})
	updateFields := map[string]any{"dish_id": integer("本人私房菜 ID", 1, 1<<31-1)}
	for name, schema := range fields {
		updateFields[name] = schema
	}
	register(&Tool{
		Name: "update_private_recipe", Title: "修改私房菜",
		Description: "用户明确要求修改本人私房菜时使用；只传需要修改的字段。数组字段为整体替换，修改前应先用 get_dish 确认原内容。不能修改公共或家庭共享菜谱。",
		Scope:       auth.ScopeDishesWrite, Write: true, Schema: object(updateFields, "dish_id"),
		Handler: func(ctx *Ctx, args json.RawMessage) (any, error) {
			in, err := decode[struct {
				DishID uint `json:"dish_id"`
				services.PrivateRecipePatch
			}](args)
			if err != nil {
				return nil, err
			}
			if in.DishID == 0 {
				return nil, errors.New("请提供菜谱 ID")
			}
			return services.UpdatePrivateRecipe(ctx.UID(), in.DishID, in.PrivateRecipePatch)
		},
	})
	register(&Tool{
		Name: "delete_private_recipe", Title: "删除私房菜",
		Description: "用户明确确认删除本人私房菜后使用。不能删除公共或家庭共享菜谱。",
		Scope:       auth.ScopeDishesWrite, Write: true,
		Schema: object(map[string]any{"dish_id": integer("本人私房菜 ID", 1, 1<<31-1)}, "dish_id"),
		Handler: func(ctx *Ctx, args json.RawMessage) (any, error) {
			in, err := decode[struct {
				DishID uint `json:"dish_id"`
			}](args)
			if err != nil {
				return nil, err
			}
			if in.DishID == 0 {
				return nil, errors.New("请提供菜谱 ID")
			}
			if err := services.DeletePrivateRecipe(ctx.UID(), in.DishID); err != nil {
				return nil, err
			}
			return map[string]any{"deleted": true, "dish_id": in.DishID}, nil
		},
	})
}
