package services

import (
	"encoding/json"
	"ninimenu/internal/models"
)

// Dish.MarshalJSON writes its JSON columns as arrays/objects for the API, while
// Dish's Go fields are strings. Decode the persisted plan with a local adapter
// so removing the memory cache does not regenerate the menu on every request.
// Keep this compatibility confined to the cache; request binding is unchanged.
type cachedPlanDish models.Dish

func (d *cachedPlanDish) UnmarshalJSON(data []byte) error {
	type plain cachedPlanDish
	wire := struct {
		*plain
		Images      json.RawMessage `json:"images"`
		Ingredients json.RawMessage `json:"ingredients"`
		Seasonings  json.RawMessage `json:"seasonings"`
		Steps       json.RawMessage `json:"steps"`
		Tags        json.RawMessage `json:"tags"`
		VideoMeta   json.RawMessage `json:"video_meta"`
	}{plain: (*plain)(d)}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	for _, field := range []struct {
		raw  json.RawMessage
		dest *string
	}{
		{wire.Images, &d.Images}, {wire.Ingredients, &d.Ingredients},
		{wire.Seasonings, &d.Seasonings}, {wire.Steps, &d.Steps},
		{wire.Tags, &d.Tags}, {wire.VideoMeta, &d.VideoMeta},
	} {
		if len(field.raw) == 0 || string(field.raw) == "null" {
			continue
		}
		if field.raw[0] == '"' {
			// Older persisted plans may contain escaped JSON strings.
			if err := json.Unmarshal(field.raw, field.dest); err != nil {
				return err
			}
		} else {
			*field.dest = string(field.raw)
		}
	}
	return nil
}

func decodeCachedWeekPlan(raw string) (*WeekPlan, error) {
	var wire struct {
		Days []struct {
			Date    string           `json:"date"`
			DayName string           `json:"day_name"`
			Lunch   []cachedPlanDish `json:"lunch"`
			Dinner  []cachedPlanDish `json:"dinner"`
		} `json:"days"`
	}
	if err := json.Unmarshal([]byte(raw), &wire); err != nil {
		return nil, err
	}
	plan := &WeekPlan{Days: make([]WeekDayPlan, len(wire.Days))}
	for i, day := range wire.Days {
		out := &plan.Days[i]
		out.Date, out.DayName = day.Date, day.DayName
		out.Lunch = make([]models.Dish, len(day.Lunch))
		out.Dinner = make([]models.Dish, len(day.Dinner))
		for j, dish := range day.Lunch {
			out.Lunch[j] = models.Dish(dish)
		}
		for j, dish := range day.Dinner {
			out.Dinner[j] = models.Dish(dish)
		}
	}
	return plan, nil
}
