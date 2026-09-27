package handlers

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Validate before applying a replacement: malformed JSON must never silently
// erase previously saved ingredients, steps or photos.
func (r *CreateDishRequest) validate() error {
	if n := utf8.RuneCountInString(strings.TrimSpace(r.Name)); n < 1 || n > 100 {
		return fmt.Errorf("菜名请填写 1-100 个字")
	}
	if r.MealType != "" && r.MealType != "all" && r.MealType != "lunch" && r.MealType != "dinner" {
		return fmt.Errorf("请选择有效的餐次")
	}
	if r.Difficulty != "" && r.Difficulty != "easy" && r.Difficulty != "medium" && r.Difficulty != "hard" {
		return fmt.Errorf("请选择有效的烹饪难度")
	}
	if r.CookTime < 0 || r.CookTime > 600 {
		return fmt.Errorf("烹饪时间应在 0-600 分钟之间")
	}
	for _, field := range []struct {
		name, value string
		max         int
	}{
		{"分类", r.Category, 40}, {"口味", r.Taste, 200}, {"备注", r.Remark, 500}, {"视频链接", r.VideoURL, 2048},
	} {
		if utf8.RuneCountInString(field.value) > field.max {
			return fmt.Errorf("%s最多填写 %d 个字", field.name, field.max)
		}
	}
	for _, field := range []struct {
		name, value string
		max         int
	}{
		{"图片", r.Images, 18}, {"标签", r.Tags, 30}, {"食材", r.Ingredients, 100}, {"调料", r.Seasonings, 100}, {"步骤", r.Steps, 100},
	} {
		if strings.TrimSpace(field.value) == "" {
			continue
		}
		var items []json.RawMessage
		if len(field.value) > 100000 || json.Unmarshal([]byte(field.value), &items) != nil || items == nil {
			return fmt.Errorf("%s格式无效，请检查后重试", field.name)
		}
		if len(items) > field.max {
			return fmt.Errorf("%s最多保存 %d 项", field.name, field.max)
		}
		for _, item := range items {
			var value any
			_ = json.Unmarshal(item, &value)
			valid := false
			switch v := value.(type) {
			case string:
				valid = strings.TrimSpace(v) != ""
			case map[string]any:
				key := "name"
				if field.name == "步骤" {
					key = "text"
				}
				text, ok := v[key].(string)
				valid = field.name != "图片" && field.name != "标签" && ok && strings.TrimSpace(text) != ""
				if amount, exists := v["amount"]; exists && field.name != "步骤" {
					_, isString := amount.(string)
					valid = valid && isString
				}
				if field.name == "步骤" {
					if duration, exists := v["time"]; exists {
						minutes, isNumber := duration.(float64)
						valid = valid && isNumber && minutes >= 0 && minutes <= 600
					}
					if image, exists := v["image"]; exists {
						_, isString := image.(string)
						valid = valid && isString
					}
				}
			}
			if !valid {
				return fmt.Errorf("%s中有无效内容，请检查后重试", field.name)
			}
		}
	}
	return nil
}
