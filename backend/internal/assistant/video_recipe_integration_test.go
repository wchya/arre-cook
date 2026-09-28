package assistant

import (
	"context"
	"encoding/json"
	"ninimenu/internal/config"
	"ninimenu/internal/llm"
	"ninimenu/internal/video"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Explicit opt-in: calls the configured real model, never a database or platform.
func TestVideoRecipeLiveProvider(t *testing.T) {
	fixture := os.Getenv("ARRE_VIDEO_RECIPE_TEST_TRANSCRIPT")
	if fixture == "" {
		t.Skip("set ARRE_VIDEO_RECIPE_TEST_TRANSCRIPT to a public transcript fixture")
	}
	text, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	previous := config.C
	config.C.CPAConfigPath = os.Getenv("LLM_CPA_CONFIG_PATH")
	config.C.CPABaseURL = os.Getenv("LLM_CPA_BASE_URL")
	config.C.CPAModel = os.Getenv("LLM_CPA_MODEL")
	t.Cleanup(func() { config.C = previous })
	if config.C.CPAConfigPath == "" {
		t.Fatal("live test requires a read-only CPA configuration")
	}
	settings := llm.Resolve()
	for _, sample := range []struct{ name, text string }{
		{"video", string(text)},
		{"times", "今天做番茄炖牛腩。准备牛腩五百克切块，番茄两个切块，姜三片，盐半勺。牛腩冷水下锅，煮三分钟，捞出冲净。锅里放油，加入姜片和牛腩，炒两分钟。加入番茄翻炒出汁，加入清水没过食材。小火炖一个半小时，加入盐半勺，翻匀出锅。视频时长三分钟。"},
	} {
		t.Run(sample.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			started := time.Now()
			result, err := ExtractVideoRecipe(ctx, settings, video.Source{Text: sample.text, Method: "manual"}, func(string) {})
			if err != nil {
				t.Fatal(err)
			}
			if result.Recipe.CookTime != 0 || len(result.Recipe.Steps) == 0 || len(result.Recipe.Ingredients) == 0 {
				t.Fatal("invalid recipe or inferred total time")
			}
			for _, item := range append(append([]VideoIngredient{}, result.Recipe.Ingredients...), result.Recipe.Seasonings...) {
				if !strings.Contains(sample.text, item.Evidence) {
					t.Fatal("ingredient evidence absent from original transcript")
				}
			}
			for _, step := range result.Recipe.Steps {
				if !strings.Contains(sample.text, step.Evidence) {
					t.Fatal("evidence absent from original transcript")
				}
			}
			if sample.name == "video" && strings.Contains(sample.text, "首先我们准备三斤鸡块") && strings.Contains(sample.text, "不要超过一克") {
				chicken := false
				cinnamon := false
				for _, item := range append(append([]VideoIngredient{}, result.Recipe.Ingredients...), result.Recipe.Seasonings...) {
					if strings.Contains(item.Name, "鸡") && strings.Contains(item.Amount, "三斤") {
						chicken = true
					}
					if strings.Contains(item.Name, "桂皮") && strings.Contains(item.Amount, "不要超过一克") {
						cinnamon = true
					}
				}
				if !chicken || !cinnamon {
					t.Fatalf("live extraction missed a stated quantity or its limit: chicken=%t cinnamon=%t", chicken, cinnamon)
				}
			}
			if output := os.Getenv("ARRE_VIDEO_RECIPE_TEST_OUTPUT"); output != "" {
				body, _ := json.MarshalIndent(result, "", "  ")
				if err := os.WriteFile(filepath.Join(output, sample.name+".json"), body, 0600); err != nil {
					t.Fatal(err)
				}
			}
			t.Logf("seconds=%.2f ingredients=%d seasonings=%d steps=%d cook_time=0", time.Since(started).Seconds(), len(result.Recipe.Ingredients), len(result.Recipe.Seasonings), len(result.Recipe.Steps))
		})
	}
}
