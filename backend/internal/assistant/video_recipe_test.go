package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"ninimenu/internal/llm"
	"ninimenu/internal/video"
	"strings"
	"sync/atomic"
	"testing"
)

const recipeTranscript = "今天做番茄炒蛋。番茄两个切块，鸡蛋三个打散。鸡蛋下锅炒熟后盛出。炒软番茄，加盐半勺，倒入鸡蛋炒匀即可。"
const recipeJSON = `{"name":"番茄炒蛋","ingredients":[{"name":"番茄","amount":"两个","evidence":"番茄两个切块"},{"name":"鸡蛋","amount":"三个","evidence":"鸡蛋三个打散"}],"seasonings":[{"name":"盐","amount":"半勺","evidence":"加盐半勺"}],"steps":[{"text":"番茄切块，鸡蛋打散。","time":0,"evidence":"番茄两个切块，鸡蛋三个打散。"},{"text":"炒熟鸡蛋并盛出。","time":0,"evidence":"鸡蛋下锅炒熟后盛出。"},{"text":"炒软番茄，加盐和鸡蛋炒匀。","time":0,"evidence":"炒软番茄，加盐半勺，倒入鸡蛋炒匀即可。"}],"cook_time":0,"remark":""}`

func TestVideoRecipeRejectsInventedEvidenceUnknownFieldsAndPartialJSON(t *testing.T) {
	var recipe VideoRecipe
	if err := decodeVideoRecipe(recipeJSON, recipeTranscript, &recipe); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		recipeJSON[:len(recipeJSON)-1], "```json\n" + recipeJSON + "\n```", recipeJSON + `{}`,
		strings.Replace(recipeJSON, `"amount":"半勺"`, `"amount":"两公斤"`, 1),
		strings.Replace(recipeJSON, `"evidence":"加盐半勺"`, `"evidence":"字幕里没有这句"`, 1),
		strings.Replace(recipeJSON, `"cook_time":0`, `"cook_time":601`, 1),
		strings.Replace(recipeJSON, `"remark":""`, `"remark":"","tool":"save_recipe"`, 1),
		strings.Replace(recipeJSON, `"remark":""`, `"remark":"<script>attack</script>"`, 1),
	} {
		if err := decodeVideoRecipe(raw, recipeTranscript, &recipe); err == nil {
			t.Errorf("accepted invalid model result %q", raw)
		}
	}
}

func TestVideoRecipeRequiresCompleteAndApprovedToolFreeResult(t *testing.T) {
	for _, scenario := range []string{"ok", "input_block", "output_block", "truncated", "tool_call"} {
		t.Run(scenario, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				index := int(calls.Add(1))
				var request map[string]any
				if json.NewDecoder(r.Body).Decode(&request) != nil {
					t.Error("invalid request")
				}
				if _, exists := request["tools"]; exists {
					t.Error("video extractor received tools")
				}
				text, finish := "ALLOW", "stop"
				if index == 2 {
					text = recipeJSON
				}
				if scenario == "input_block" && index == 1 || scenario == "output_block" && index == 3 {
					text = "BLOCK"
				}
				if scenario == "truncated" && index == 2 {
					finish = "length"
				}
				delta := map[string]any{"content": text}
				if scenario == "tool_call" && index == 2 {
					delta["tool_calls"] = []any{map[string]any{"index": 0, "id": "forbidden", "function": map[string]string{"name": "create_private_recipe", "arguments": "{}"}}}
				}
				body, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": delta, "finish_reason": finish}}})
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", body)
			}))
			defer server.Close()
			settings := llm.Settings{BaseURL: server.URL, APIKey: "fixture", Model: "fixture"}
			result, err := ExtractVideoRecipe(context.Background(), settings, video.Source{Method: "manual", Text: recipeTranscript}, func(string) {})
			if scenario == "ok" {
				if err != nil || result.Recipe.Name != "番茄炒蛋" || calls.Load() != 3 {
					t.Fatalf("unexpected result %v %d", err, calls.Load())
				}
			} else if err == nil {
				t.Fatal("unapproved output accepted")
			}
			if scenario == "input_block" && calls.Load() != 1 {
				t.Fatal("blocked input reached generator")
			}
		})
	}
}
