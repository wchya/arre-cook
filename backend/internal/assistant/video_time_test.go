package assistant

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestVideoTimesRequireExplicitSourceDurations(t *testing.T) {
	for _, row := range []struct {
		text    string
		minutes int
		allowed bool
	}{
		{"小火炖二十分钟", 20, true}, {"炒3分钟后盛出", 3, true},
		{"炖两小时", 120, true}, {"炖一个半小时", 90, true}, {"泡发半小时", 30, true},
		{"煮1小时30分钟", 90, true}, {"煮一小时二十分钟", 80, true},
		{"煮120秒后取出", 2, true}, {"煮３０分钟后取出", 30, true},
		{"小火炖到软烂", 20, false}, {"小火炖二十分钟", 30, false},
		{"炒三分钟，再炖二十分钟", 23, false}, {"炒三分钟，再炖二十分钟", 20, false},
		{"炖二十到三十分钟", 30, false}, {"炖20-30分钟", 30, false},
		{"炒两三分钟", 3, false}, {"煮2.5分钟", 5, false}, {"煮1/2分钟", 2, false},
		{"煮一百二分钟", 102, false}, {"不要煮二十分钟", 20, false},
		{"不超过二十分钟", 20, false}, {"煮一小时半", 60, false},
		{"煮1小时30分钟", 30, false}, {"煮90秒", 2, false},
	} {
		if got := supportsVideoMinutes(row.text, row.minutes); got != row.allowed {
			t.Errorf("%q supports %d minutes = %v, want %v", row.text, row.minutes, got, row.allowed)
		}
	}
}

func TestVideoRecipeDoesNotInferTotalFromIndividualStepTimes(t *testing.T) {
	for _, row := range []struct {
		name, source, evidence string
		claimed, want          int
	}{
		{"summed_steps", "炒三分钟，再小火炖二十分钟。", "", 23, 0},
		{"single_step", "小火炖二十分钟。", "小火炖二十分钟。", 20, 0},
		{"step_total", "总共炖二十分钟。", "总共炖二十分钟。", 20, 0},
		{"invented_quote", "小火炖二十分钟。", "总烹饪时间二十分钟。", 20, 0},
		{"explicit_total", "总烹饪时间三十分钟。", "总烹饪时间三十分钟。", 30, 30},
		{"wrong_value", "总烹饪时间三十分钟。", "总烹饪时间三十分钟。", 20, 0},
		{"half_hour", "全程用时半小时。", "全程用时半小时。", 30, 30},
		{"combined_units", "总用时1小时30分钟。", "总用时1小时30分钟。", 90, 90},
		{"video_duration", "视频总时长三十分钟。", "视频总时长三十分钟。", 30, 0},
	} {
		t.Run(row.name, func(t *testing.T) {
			var generated VideoRecipe
			if err := json.Unmarshal([]byte(recipeJSON), &generated); err != nil {
				t.Fatal(err)
			}
			generated.CookTime, generated.CookTimeEvidence = row.claimed, row.evidence
			// A model can also invent a timer for a step whose quote has no time.
			generated.Steps[0].Time = 5
			raw, _ := json.Marshal(generated)
			var recipe VideoRecipe
			if err := decodeVideoRecipe(string(raw), recipeTranscript+row.source, &recipe); err != nil {
				t.Fatal(err)
			}
			if recipe.CookTime != row.want || recipe.Steps[0].Time != 0 {
				t.Fatalf("unsupported time retained: total=%d step=%d", recipe.CookTime, recipe.Steps[0].Time)
			}
			if row.want == 0 && recipe.CookTimeEvidence != "" {
				t.Fatal("unsupported total time retained its quote")
			}
			if !strings.Contains(recipeTranscript, recipe.Steps[0].Evidence) {
				t.Fatal("normalizing an unsupported timer changed the original evidence")
			}
		})
	}
}
