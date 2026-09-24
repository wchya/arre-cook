// Package assistant 站内 AI 助手：大模型 + 工具调用循环，工具即 agent 包的同一套能力，
// 只能读写当前登录用户的数据。对话历史按用户隔离保存。
package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"ninimenu/internal/agent"
	"ninimenu/internal/auth"
	"ninimenu/internal/database"
	"ninimenu/internal/llm"
	"ninimenu/internal/models"
	"ninimenu/internal/services"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxRounds       = 6
	historyMessages = 16
	maxToolResult   = 6000
)

// Emit 向前端推送一个 SSE 事件。
type Emit func(event string, data any)

// Card 助手回复里附带的结构化内容，前端渲染成菜品卡片/操作回执。
type Card struct {
	Type  string     `json:"type"` // dishes / action
	Title string     `json:"title,omitempty"`
	Items []CardItem `json:"items,omitempty"`
	Text  string     `json:"text,omitempty"`
}

type CardItem struct {
	Dish    services.DishCard `json:"dish"`
	Reasons []string          `json:"reasons,omitempty"`
	Score   float64           `json:"score,omitempty"`
}

var toolLabels = map[string]string{
	"get_context": "看看今天的情况", "get_taste_profile": "分析你的口味画像", "get_preferences": "读取你的饮食偏好",
	"update_preferences": "更新你的饮食偏好", "search_dishes": "在菜谱里搜索", "get_dish": "查看做法",
	"recommend_dishes": "按口味挑菜", "list_meal_records": "翻看用餐记录", "log_meal": "帮你记一餐",
	"rate_meal": "记录评价", "delete_meal_record": "删除记录", "list_favorites": "查看收藏", "set_favorite": "更新收藏",
	"get_week_plan": "查看本周菜单", "regenerate_week_plan": "重排本周菜单", "get_shopping_list": "整理买菜清单",
	"get_stats": "统计饮食数据", "list_behavior_events": "分析推荐反馈", "log_feedback": "记下你的反馈",
	"create_suggestion": "放进建议收件箱", "list_suggestions": "查看建议", "get_day_ratings": "查看每日评价",
}

// Run 处理一轮用户输入。
func Run(ctx context.Context, p *auth.Principal, sessionID uint, text string, emit Emit) error {
	uid := p.UserID()
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("说点什么吧")
	}
	if utf8.RuneCountInString(text) > 1000 {
		text = string([]rune(text)[:1000])
	}

	session, err := loadOrCreateSession(uid, sessionID, text)
	if err != nil {
		return err
	}
	emit("session", map[string]any{"session_id": session.ID, "title": session.Title})

	history := loadHistory(uid, session.ID)
	userMsg := models.ChatMessage{SessionID: session.ID, UserID: uid, Role: "user", Content: text, Cards: "[]"}
	database.DB.Create(&userMsg)
	services.LogBehavior(uid, "chat", 0, "", "assistant", "", map[string]any{"text": truncate(text, 200)})

	settings := llm.Resolve()
	var reply string
	var cards []Card
	if settings.Enabled() {
		reply, cards, err = runLLM(ctx, p, settings, history, text, emit)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			emit("error", map[string]any{"message": err.Error()})
			// 模型异常时退化为本地推荐，保证有结果
			fbReply, fbCards := fallback(p, text, emit)
			reply = strings.TrimSpace(reply + "\n\n" + fbReply)
			cards = append(cards, fbCards...)
		}
	} else {
		reply, cards = fallback(p, text, emit)
	}

	cardsJSON, _ := json.Marshal(cards)
	msg := models.ChatMessage{SessionID: session.ID, UserID: uid, Role: "assistant", Content: reply, Cards: string(cardsJSON)}
	database.DB.Create(&msg)
	database.DB.Model(&models.ChatSession{}).Where("id = ?", session.ID).Update("updated_at", time.Now())
	emit("done", map[string]any{"session_id": session.ID, "message_id": msg.ID})
	return nil
}

func runLLM(ctx context.Context, p *auth.Principal, s llm.Settings, history []llm.Message, text string, emit Emit) (string, []Card, error) {
	toolCtx := &agent.Ctx{Context: ctx, Principal: p, Channel: "chat"}
	messages := []llm.Message{{Role: "system", Content: systemPrompt(p)}}
	messages = append(messages, history...)
	messages = append(messages, llm.Message{Role: "user", Content: text})
	tools := agent.OpenAITools(p)

	var reply strings.Builder
	var cards []Card
	for round := 0; round < maxRounds; round++ {
		res, err := llm.Stream(ctx, s, llm.Request{
			Messages: messages, Tools: tools, ToolChoice: "auto", Temperature: 0.7, MaxTokens: 1200,
		}, func(delta string) {
			reply.WriteString(delta)
			emit("delta", map[string]any{"text": delta})
		})
		if err != nil {
			return reply.String(), cards, err
		}
		if len(res.ToolCalls) == 0 {
			break
		}
		messages = append(messages, llm.Message{Role: "assistant", Content: res.Content, ToolCalls: res.ToolCalls})
		for _, tc := range res.ToolCalls {
			name := tc.Function.Name
			emit("tool_start", map[string]any{"id": tc.ID, "name": name, "label": labelOf(name)})
			result, err := agent.Invoke(toolCtx, name, json.RawMessage(orEmptyObject(tc.Function.Arguments)))
			var content string
			if err != nil {
				content = fmt.Sprintf(`{"error":%q}`, err.Error())
				emit("tool_end", map[string]any{"id": tc.ID, "name": name, "ok": false, "error": err.Error()})
			} else {
				b, _ := json.Marshal(result)
				content = truncate(string(b), maxToolResult)
				card := cardFromResult(name, b)
				if card != nil {
					cards = append(cards, *card)
				}
				emit("tool_end", map[string]any{"id": tc.ID, "name": name, "ok": true, "card": card})
			}
			messages = append(messages, llm.Message{Role: "tool", ToolCallID: tc.ID, Name: name, Content: content})
		}
		if round == maxRounds-1 {
			reply.WriteString("\n（这次查得有点多，先说到这里～）")
		}
	}
	return strings.TrimSpace(reply.String()), dedupeCards(cards), nil
}

func systemPrompt(p *auth.Principal) string {
	uid := p.UserID()
	now := time.Now()
	meal := "晚餐"
	if now.Hour() < 14 {
		meal = "午餐"
	}
	profile := services.BuildTasteProfile(uid, 90)
	return fmt.Sprintf(`你是「%s」里的私人食谱助手，说话温暖、简洁、口语化，用中文回答。
当前用户：%s；现在是 %s %s %s，这个时间通常在考虑%s。
用户画像：%s

工作守则：
1. 推荐菜只能来自工具结果（优先 recommend_dishes，其次 search_dishes），绝不编造菜名或菜品 ID；推荐时用一两句话说明理由。
2. 严格遵守用户的过敏原与忌口；用户临时提出的新限制，作为本次的 exclude_ingredients 等参数传入。
3. 用户问做法时调用 get_dish，按步骤清晰列出，标出关键火候与用量。
4. 只有当用户明确表示“就吃这个/帮我记上/收藏/改偏好”时才调用写入类工具（log_meal、set_favorite、update_preferences 等）；删除记录前先确认。
5. 用户说“不想吃某道菜”时，用 log_feedback 记录 reject，并换一批（exclude_dish_ids）。
6. 回答控制在 200 字以内，菜品列表不必重复卡片里已有的细节（界面会自动展示菜品卡片）。
7. 与饮食无关的问题，礼貌地拉回到吃饭这件事上。`,
		database.GetSetting("app_name", "NiniMenu"), p.User.DisplayName(),
		now.Format("2006-01-02"), []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}[now.Weekday()], now.Format("15:04"),
		meal, profile.Summary)
}

// fallback 未配置大模型或模型出错时，用关键词解析意图后直接调用推荐引擎。
func fallback(p *auth.Principal, text string, emit Emit) (string, []Card) {
	req := map[string]any{"count": 4}
	switch {
	case strings.Contains(text, "午"):
		req["meal_type"] = "lunch"
	case strings.Contains(text, "晚"):
		req["meal_type"] = "dinner"
	}
	switch {
	case strings.Contains(text, "辣"):
		req["mood"] = "spicy"
	case strings.Contains(text, "累"), strings.Contains(text, "懒"), strings.Contains(text, "快"), strings.Contains(text, "简单"):
		req["mood"] = "tired"
	case strings.Contains(text, "清淡"), strings.Contains(text, "健康"), strings.Contains(text, "减"):
		req["mood"] = "healthy"
	}
	args, _ := json.Marshal(req)
	emit("tool_start", map[string]any{"id": "fallback", "name": "recommend_dishes", "label": labelOf("recommend_dishes")})
	res, err := agent.Invoke(&agent.Ctx{Context: context.Background(), Principal: p, Channel: "chat"}, "recommend_dishes", args)
	if err != nil {
		emit("tool_end", map[string]any{"id": "fallback", "name": "recommend_dishes", "ok": false, "error": err.Error()})
		msg := "暂时没找到合适的菜，换个说法试试？"
		emit("delta", map[string]any{"text": msg})
		return msg, nil
	}
	b, _ := json.Marshal(res)
	card := cardFromResult("recommend_dishes", b)
	emit("tool_end", map[string]any{"id": "fallback", "name": "recommend_dishes", "ok": true, "card": card})
	msg := "按你的口味挑了这几道，点卡片看做法，或者直接记到今天的菜单里～"
	if card == nil || len(card.Items) == 0 {
		msg = "暂时没找到合适的菜，放宽点条件试试？"
	}
	emit("delta", map[string]any{"text": msg})
	if card == nil {
		return msg, nil
	}
	return msg, []Card{*card}
}

func cardFromResult(tool string, raw []byte) *Card {
	switch tool {
	case "recommend_dishes":
		var r struct {
			Items []struct {
				Dish    services.DishCard `json:"dish"`
				Score   float64           `json:"score"`
				Reasons []string          `json:"reasons"`
			} `json:"items"`
		}
		if json.Unmarshal(raw, &r) != nil || len(r.Items) == 0 {
			return nil
		}
		c := &Card{Type: "dishes", Title: "为你推荐"}
		for _, it := range r.Items {
			c.Items = append(c.Items, CardItem{Dish: it.Dish, Reasons: it.Reasons, Score: it.Score})
		}
		return c
	case "search_dishes", "list_favorites":
		var r struct {
			Dishes []services.DishCard `json:"dishes"`
		}
		if json.Unmarshal(raw, &r) != nil || len(r.Dishes) == 0 {
			return nil
		}
		title := "找到这些菜"
		if tool == "list_favorites" {
			title = "你的收藏"
		}
		c := &Card{Type: "dishes", Title: title}
		for i, d := range r.Dishes {
			if i >= 6 {
				break
			}
			c.Items = append(c.Items, CardItem{Dish: d})
		}
		return c
	case "get_dish":
		var r struct {
			Dish models.Dish `json:"dish"`
		}
		if json.Unmarshal(raw, &r) != nil || r.Dish.ID == 0 {
			return nil
		}
		return &Card{Type: "dishes", Title: "做法", Items: []CardItem{{Dish: services.ToDishCard(r.Dish)}}}
	case "log_meal":
		var r struct {
			DishName string `json:"dish_name"`
			MealType string `json:"meal_type"`
			MealDate string `json:"meal_date"`
		}
		if json.Unmarshal(raw, &r) != nil {
			return nil
		}
		meal := map[string]string{"lunch": "午餐", "dinner": "晚餐"}[r.MealType]
		return &Card{Type: "action", Text: fmt.Sprintf("已记入 %s %s：%s", r.MealDate, meal, r.DishName)}
	case "set_favorite":
		var r struct {
			Favorite bool `json:"favorite"`
		}
		if json.Unmarshal(raw, &r) != nil {
			return nil
		}
		if r.Favorite {
			return &Card{Type: "action", Text: "已加入收藏"}
		}
		return &Card{Type: "action", Text: "已取消收藏"}
	case "update_preferences":
		return &Card{Type: "action", Text: "已更新你的饮食偏好"}
	case "regenerate_week_plan":
		return &Card{Type: "action", Text: "本周菜单已重新生成"}
	case "create_suggestion":
		return &Card{Type: "action", Text: "已放进首页的「AI 建议」"}
	case "delete_meal_record":
		return &Card{Type: "action", Text: "已删除这条记录"}
	}
	return nil
}

// dedupeCards 同一道菜只在第一张卡片里出现。
func dedupeCards(cards []Card) []Card {
	seen := map[uint]bool{}
	out := make([]Card, 0, len(cards))
	for _, c := range cards {
		if c.Type != "dishes" {
			out = append(out, c)
			continue
		}
		items := c.Items[:0:0]
		for _, it := range c.Items {
			if !seen[it.Dish.ID] {
				seen[it.Dish.ID] = true
				items = append(items, it)
			}
		}
		if len(items) > 0 {
			c.Items = items
			out = append(out, c)
		}
	}
	return out
}

func loadOrCreateSession(uid, id uint, firstText string) (*models.ChatSession, error) {
	var s models.ChatSession
	if id > 0 {
		if err := database.DB.Scopes(database.OwnedBy(uid)).First(&s, id).Error; err == nil {
			return &s, nil
		}
	}
	s = models.ChatSession{UserID: uid, Title: truncate(firstText, 24)}
	if err := database.DB.Create(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

func loadHistory(uid, sessionID uint) []llm.Message {
	var rows []models.ChatMessage
	database.DB.Scopes(database.OwnedBy(uid)).Where("session_id = ?", sessionID).
		Order("id DESC").Limit(historyMessages).Find(&rows)
	out := make([]llm.Message, 0, len(rows))
	for i := len(rows) - 1; i >= 0; i-- {
		r := rows[i]
		if r.Content == "" {
			continue
		}
		content := r.Content
		// 把上一轮展示过的菜品 ID 告诉模型，方便用户说“就吃第二道”
		if r.Role == "assistant" && r.Cards != "" && r.Cards != "[]" {
			var cards []Card
			if json.Unmarshal([]byte(r.Cards), &cards) == nil {
				var names []string
				for _, c := range cards {
					for _, it := range c.Items {
						names = append(names, fmt.Sprintf("%s(id=%d)", it.Dish.Name, it.Dish.ID))
					}
				}
				if len(names) > 0 {
					content += "\n[已展示菜品卡片：" + strings.Join(names, "、") + "]"
				}
			}
		}
		out = append(out, llm.Message{Role: r.Role, Content: content})
	}
	return out
}

func labelOf(name string) string {
	if l, ok := toolLabels[name]; ok {
		return l
	}
	return name
}

func orEmptyObject(s string) string {
	if strings.TrimSpace(s) == "" {
		return "{}"
	}
	return s
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}
