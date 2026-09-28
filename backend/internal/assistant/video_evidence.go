package assistant

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

type videoSourceLine struct {
	ID    int    `json:"id"`
	Text  string `json:"text"`
	start int
	end   int
}

// Retain offsets into the original text so citations never depend on the model
// reproducing subtitle whitespace or punctuation. Long manual text is bounded too.
func videoSourceLines(text string) []videoSourceLine {
	var lines []videoSourceLine
	start, runes := 0, 0
	flush := func(end int) {
		raw := text[start:end]
		left := len(raw) - len(strings.TrimLeftFunc(raw, unicode.IsSpace))
		trimmed := strings.TrimSpace(raw)
		if trimmed != "" {
			lines = append(lines, videoSourceLine{ID: len(lines) + 1, Text: trimmed,
				start: start + left, end: start + left + len(trimmed)})
		}
		start, runes = end, 0
	}
	for offset, r := range text {
		runes++
		if r == '\n' || strings.ContainsRune("。！？；!?;", r) || runes >= 120 {
			flush(offset + utf8.RuneLen(r))
		}
	}
	flush(len(text))
	return lines
}

type videoCitation struct {
	First int `json:"first"`
	Last  int `json:"last"`
}

func (c videoCitation) quote(text string, lines []videoSourceLine) (string, error) {
	if c.First < 1 || c.Last < c.First || c.Last > len(lines) {
		return "", errors.New("invalid citation range")
	}
	quote := text[lines[c.First-1].start:lines[c.Last-1].end]
	if n := utf8.RuneCountInString(quote); n < 4 || n > 240 {
		return "", errors.New("invalid citation length")
	}
	return quote, nil
}

type citedVideoIngredient struct {
	Name     string        `json:"name"`
	Amount   string        `json:"amount"`
	Evidence videoCitation `json:"evidence"`
}

type citedVideoRecipe struct {
	Name        string                 `json:"name"`
	Ingredients []citedVideoIngredient `json:"ingredients"`
	Seasonings  []citedVideoIngredient `json:"seasonings"`
	Steps       []struct {
		Text     string        `json:"text"`
		Time     int           `json:"time"`
		Evidence videoCitation `json:"evidence"`
	} `json:"steps"`
	CookTime         int            `json:"cook_time"`
	CookTimeEvidence *videoCitation `json:"cook_time_evidence"`
	Remark           string         `json:"remark"`
}

func decodeVideoRecipeCitations(raw, transcript string, lines []videoSourceLine, recipe *VideoRecipe) error {
	var cited citedVideoRecipe
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	if len(raw) > 16<<10 || decoder.Decode(&cited) != nil || decoder.Decode(new(any)) != io.EOF {
		return invalidVideoRecipe("invalid_citation_json")
	}
	if len(cited.Ingredients) == 0 || len(cited.Ingredients) > 25 || len(cited.Seasonings) > 25 || len(cited.Steps) == 0 || len(cited.Steps) > 30 {
		return invalidVideoRecipe("invalid_recipe_shape")
	}
	resolved := VideoRecipe{Name: cited.Name, CookTime: cited.CookTime, Remark: cited.Remark}
	for i, items := range [][]citedVideoIngredient{cited.Ingredients, cited.Seasonings} {
		for _, item := range items {
			quote, err := item.Evidence.quote(transcript, lines)
			if err != nil {
				return invalidVideoRecipe("invalid_ingredient_citation")
			}
			value := VideoIngredient{Name: item.Name, Amount: item.Amount, Evidence: quote}
			if i == 0 {
				resolved.Ingredients = append(resolved.Ingredients, value)
			} else {
				resolved.Seasonings = append(resolved.Seasonings, value)
			}
		}
	}
	for _, step := range cited.Steps {
		quote, err := step.Evidence.quote(transcript, lines)
		if err != nil {
			return invalidVideoRecipe("invalid_step_citation")
		}
		resolved.Steps = append(resolved.Steps, VideoStep{Text: step.Text, Time: step.Time, Evidence: quote})
	}
	if cited.CookTime > 0 && cited.CookTimeEvidence != nil {
		// A total duration is optional. Unusable evidence clears only the time,
		// just like a valid citation that does not actually state a total time.
		if quote, err := cited.CookTimeEvidence.quote(transcript, lines); err == nil {
			resolved.CookTimeEvidence = quote
		}
	}
	if err := validateVideoRecipe(&resolved, transcript); err != nil {
		return err
	}
	*recipe = resolved
	return nil
}
