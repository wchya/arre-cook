package assistant

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestVideoCitationsFromRealCookingVideo(t *testing.T) {
	// Recorded public subtitles and an approved real-model draft. Keep this
	// deterministic: normal regression tests must not call a platform or model.
	data, err := os.ReadFile("testdata/video_chicken_citations.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Transcript string          `json:"transcript"`
		Response   json.RawMessage `json:"response"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	lines := videoSourceLines(fixture.Transcript)
	var recipe VideoRecipe
	if err := decodeVideoRecipeCitations(string(fixture.Response), fixture.Transcript, lines, &recipe); err != nil {
		t.Fatal(err)
	}
	if recipe.Name != "黄焖辣子鸡" || recipe.CookTime != 0 || len(recipe.Steps) != 11 {
		t.Fatalf("real draft lost its structure: name=%q time=%d steps=%d", recipe.Name, recipe.CookTime, len(recipe.Steps))
	}
	chicken, cinnamon := false, false
	for _, group := range [][]VideoIngredient{recipe.Ingredients, recipe.Seasonings} {
		for _, item := range group {
			if !strings.Contains(fixture.Transcript, item.Evidence) {
				t.Fatal("citation changed original subtitle characters")
			}
			if strings.Contains(item.Name, "鸡") && item.Amount == "三斤" {
				chicken = true
			}
			if strings.Contains(item.Name, "桂皮") && item.Amount == "一点点 不要超过一克" && strings.Contains(item.Evidence, "\n不要超过一克") {
				cinnamon = true
			}
		}
	}
	if !chicken || !cinnamon {
		t.Fatalf("lost a quantity or its qualifier: chicken=%t cinnamon=%t", chicken, cinnamon)
	}
	for _, step := range recipe.Steps {
		if !strings.Contains(fixture.Transcript, step.Evidence) {
			t.Fatal("step evidence was rewritten or stitched together")
		}
	}
}

func TestVideoCitationsPreserveContiguousSourceIncludingQualifiers(t *testing.T) {
	const source = "  桂皮一点点\n不要超过一克。\n\n鸡块三斤炒香。"
	lines := videoSourceLines(source)
	if len(lines) != 3 {
		t.Fatalf("unexpected line count: %d", len(lines))
	}
	quote, err := (videoCitation{First: 1, Last: 2}).quote(source, lines)
	if err != nil || quote != "桂皮一点点\n不要超过一克。" {
		t.Fatalf("citation changed the source: %q %v", quote, err)
	}
	for _, citation := range []videoCitation{{}, {First: -1, Last: 1}, {First: 2, Last: 1}, {First: 1, Last: 4}} {
		if _, err := citation.quote(source, lines); err == nil {
			t.Fatalf("accepted invalid range: %+v", citation)
		}
	}
	long := strings.Repeat("鸡", 500)
	for _, line := range videoSourceLines(long) {
		if !utf8.ValidString(line.Text) || utf8.RuneCountInString(line.Text) > 120 || long[line.start:line.end] != line.Text {
			t.Fatal("long input split changed source boundaries")
		}
	}
	if _, err := (videoCitation{First: 1, Last: 3}).quote(long, videoSourceLines(long)); err == nil {
		t.Fatal("accepted oversized evidence")
	}
}

func TestVideoCitationDecoderRetainsMultilineQuantityQualifier(t *testing.T) {
	const source = "今天做黄焖辣子鸡。\n首先我们准备三斤鸡块。\n桂皮一点点\n不要超过一克。\n鸡块下锅炒香。"
	const raw = `{"name":"黄焖辣子鸡","ingredients":[{"name":"鸡块","amount":"三斤","evidence":{"first":2,"last":2}}],"seasonings":[{"name":"桂皮","amount":"一点点 不要超过一克","evidence":{"first":3,"last":4}}],"steps":[{"text":"鸡块下锅炒香。","time":0,"evidence":{"first":5,"last":5}}],"cook_time":0,"cook_time_evidence":null,"remark":""}`
	var recipe VideoRecipe
	if err := decodeVideoRecipeCitations(raw, source, videoSourceLines(source), &recipe); err != nil {
		t.Fatal(err)
	}
	if got := recipe.Seasonings[0]; got.Amount != "一点点 不要超过一克" || got.Evidence != "桂皮一点点\n不要超过一克。" {
		t.Fatalf("multiline qualifier or exact quote changed: %+v", got)
	}
}

func TestVideoCitationDecoderSupportsNumericSubtitleLines(t *testing.T) {
	const source = "今天做番茄炖牛腩。\n准备牛腩\n500\n克切块。\n小火炖\n30\n分钟后出锅。"
	const raw = `{"name":"番茄炖牛腩","ingredients":[{"name":"牛腩","amount":"500 克","evidence":{"first":2,"last":4}}],"seasonings":[],"steps":[{"text":"小火炖牛腩。","time":30,"evidence":{"first":5,"last":7}}],"cook_time":0,"cook_time_evidence":null,"remark":""}`
	var recipe VideoRecipe
	if err := decodeVideoRecipeCitations(raw, source, videoSourceLines(source), &recipe); err != nil {
		t.Fatal(err)
	}
	if recipe.Ingredients[0].Amount != "500 克" || recipe.Ingredients[0].Evidence != "准备牛腩\n500\n克切块。" || recipe.Steps[0].Time != 30 || recipe.Steps[0].Evidence != "小火炖\n30\n分钟后出锅。" {
		t.Fatalf("numeric subtitle lines were not preserved: %+v", recipe)
	}
}

func TestVideoCitationDecoderRejectsForgedRangesAndUnsupportedQuantities(t *testing.T) {
	lines := videoSourceLines(recipeTranscript)
	var recipe VideoRecipe
	if err := decodeVideoRecipeCitations(citedRecipeJSON, recipeTranscript, lines, &recipe); err != nil {
		t.Fatal(err)
	}
	if recipe.Ingredients[0].Evidence != "番茄两个切块，鸡蛋三个打散。" {
		t.Fatal("exact original evidence was not restored")
	}
	for _, raw := range []string{
		strings.Replace(citedRecipeJSON, `"first":2`, `"first":0`, 1),
		strings.Replace(citedRecipeJSON, `"last":2`, `"last":999`, 1),
		strings.Replace(citedRecipeJSON, `"first":2`, `"first":2.5`, 1),
		strings.Replace(citedRecipeJSON, `"amount":"两个"`, `"amount":"十个"`, 1),
		strings.Replace(citedRecipeJSON, `"first":2,"last":2`, `"first":2,"last":2,"quote":"invented"`, 1),
		citedRecipeJSON + `{}`, recipeJSON,
	} {
		if err := decodeVideoRecipeCitations(raw, recipeTranscript, lines, &VideoRecipe{}); err == nil {
			t.Fatal("unreliable model citation accepted")
		}
	}
}

func TestVideoCitationsAllowExpandedEvidenceWithoutRaisingModelOutputLimit(t *testing.T) {
	// A long tutorial can legitimately repeat the source in each step. The
	// model returns short references; expanding them must not hit its byte cap.
	var transcript strings.Builder
	steps := make([]map[string]any, 0, 30)
	for i := 1; i <= 30; i++ {
		line := fmt.Sprintf("第%d步，鸡块下锅翻炒，", i) + strings.Repeat("保持中小火翻炒鸡块，观察锅内水分变化，", 10)
		transcript.WriteString(line + "。\n")
		steps = append(steps, map[string]any{"text": "保持中小火翻炒鸡块。", "time": 0, "evidence": videoCitation{First: 2*i - 1, Last: 2 * i}})
	}
	source := transcript.String()
	if utf8.RuneCountInString(source) > 8000 {
		t.Fatal("fixture exceeds the supported transcript length")
	}
	generated := map[string]any{
		"name": "炒鸡块", "ingredients": []map[string]any{{"name": "鸡块", "amount": "", "evidence": videoCitation{First: 1, Last: 2}}},
		"seasonings": []any{}, "steps": steps, "cook_time": 0, "cook_time_evidence": nil, "remark": "",
	}
	raw, err := json.Marshal(generated)
	if err != nil || len(raw) >= 16<<10 {
		t.Fatal("fixture must fit the model response limit")
	}
	var recipe VideoRecipe
	if err := decodeVideoRecipeCitations(string(raw), source, videoSourceLines(source), &recipe); err != nil {
		t.Fatalf("valid long tutorial rejected after resolving citations: %v", err)
	}
	resolved, _ := json.Marshal(recipe)
	if len(resolved) <= 16<<10 || len(recipe.Steps) != len(steps) {
		t.Fatal("expanded evidence was truncated or did not exercise the old limit")
	}
	for _, step := range recipe.Steps {
		if !strings.Contains(source, step.Evidence) {
			t.Fatal("expanded citation no longer matches the original text")
		}
	}
	oversized := string(raw) + strings.Repeat(" ", 16<<10)
	if err := decodeVideoRecipeCitations(oversized, source, videoSourceLines(source), &VideoRecipe{}); err == nil {
		t.Fatal("oversized model output accepted")
	}
}

func TestVideoCitationsDiscardUnsupportedOptionalTotalTime(t *testing.T) {
	for _, citation := range []string{`null`, `{"first":0,"last":0}`, `{"first":1,"last":999}`, `{"first":3,"last":2}`} {
		t.Run(citation, func(t *testing.T) {
			raw := strings.Replace(citedRecipeJSON, `"cook_time":0`, `"cook_time":30`, 1)
			raw = strings.Replace(raw, `"cook_time_evidence":null`, `"cook_time_evidence":`+citation, 1)
			var recipe VideoRecipe
			if err := decodeVideoRecipeCitations(raw, recipeTranscript, videoSourceLines(recipeTranscript), &recipe); err != nil {
				t.Fatalf("unsupported total time should be cleared without losing the recipe: %v", err)
			}
			if recipe.CookTime != 0 || recipe.CookTimeEvidence != "" || len(recipe.Steps) != 3 {
				t.Fatal("unsupported time was retained or unrelated steps were lost")
			}
		})
	}
}
