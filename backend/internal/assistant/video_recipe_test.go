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
			if (scenario == "truncated" || scenario == "tool_call") && calls.Load() != 2 {
				t.Fatal("incomplete generation was retried or reached output review")
			}
		})
	}
}

func TestVideoRecipeNormalizesMultilineAmountsWithoutChangingEvidence(t *testing.T) {
	const evidence = "桂皮一点点\n不要超过一克"
	for _, group := range []string{"ingredients", "seasonings"} {
		t.Run(group, func(t *testing.T) {
			var generated VideoRecipe
			if err := json.Unmarshal([]byte(recipeJSON), &generated); err != nil {
				t.Fatal(err)
			}
			item := VideoIngredient{Name: "桂皮", Amount: " 一点点\r\n\t不要超过一克 ", Evidence: evidence}
			if group == "ingredients" {
				generated.Ingredients = append(generated.Ingredients, item)
			} else {
				generated.Seasonings = append(generated.Seasonings, item)
			}
			raw, _ := json.Marshal(generated)
			var recipe VideoRecipe
			if err := decodeVideoRecipe(string(raw), recipeTranscript+evidence, &recipe); err != nil {
				t.Fatal(err)
			}
			items := recipe.Ingredients
			if group == "seasonings" {
				items = recipe.Seasonings
			}
			got := items[len(items)-1]
			if got.Amount != "一点点 不要超过一克" || got.Evidence != evidence {
				t.Fatalf("quantity or source was changed: %+v", got)
			}
		})
	}
	invalid := strings.Replace(recipeJSON, `"amount":"半勺"`, `"amount":"两\n公斤"`, 1)
	if err := decodeVideoRecipe(invalid, recipeTranscript, &VideoRecipe{}); err == nil {
		t.Fatal("normalizing whitespace accepted an unsupported quantity")
	}
}

func TestVideoRecipeBoundsReasoningForGenerationAndSourceReview(t *testing.T) {
	var calls, outputReviews atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var request llm.Request
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Messages) != 2 {
			t.Error("invalid model request")
			return
		}
		if len(request.Tools) != 0 {
			t.Error("video extraction enabled tools")
		}
		// This provider otherwise spends its entire budget reasoning, without
		// returning any JSON or verdict, as GLM-5.3 does at its default effort.
		if request.ReasoningEffort != "low" {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"length\"}]}\n\ndata: [DONE]\n\n")
			return
		}
		if request.Messages[0].Content == videoRecipePrompt {
			if request.MaxTokens <= 3000 || request.MaxTokens > 4096 {
				t.Error("recipe generation budget must fit source evidence and remain bounded")
			}
			respondText(w, recipeJSON)
			return
		}
		var payload struct {
			Stage string `json:"stage"`
		}
		if json.Unmarshal([]byte(request.Messages[1].Content), &payload) != nil {
			t.Error("invalid review payload")
			return
		}
		if payload.Stage == "video-output" {
			outputReviews.Add(1)
			// This provider fixture needs 768 reasoning tokens before it can emit
			// a verdict. The old 512-token limit ended with no visible answer.
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"checking each field against the transcript\"}}]}\n\n")
			if request.MaxTokens < 768 {
				fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"length\"}]}\n\ndata: [DONE]\n\n")
				return
			}
			if request.MaxTokens > 1536 {
				t.Error("source review exceeded its bounded reasoning budget")
			}
		}
		respondText(w, "ALLOW")
	}))
	defer server.Close()
	result, err := ExtractVideoRecipe(context.Background(), llm.Settings{BaseURL: server.URL, APIKey: "fixture", Model: "glm-5.3"}, video.Source{Method: "manual", Text: recipeTranscript}, func(string) {})
	if err != nil || result.Recipe.Name != "番茄炒蛋" || calls.Load() != 3 || outputReviews.Load() != 1 {
		t.Fatalf("source review did not complete: error=%v calls=%d reviews=%d", err, calls.Load(), outputReviews.Load())
	}
}
