package routes_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"ninimenu/internal/auth"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"ninimenu/internal/services"
	"ninimenu/internal/testutil"
	"strings"
	"testing"
)

func chatRequest(t *testing.T, token, client string, body any) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/api/assistant/chat", bytes.NewReader(encoded))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", client)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}

func TestPageAssistantQuotaSharedAcrossClientsAndSessionDeletion(t *testing.T) {
	user, token, err := testutil.NewUser("quota-page@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	old := database.GetSetting(services.AssistantDailyLimitKey, "20")
	t.Cleanup(func() { _ = database.SetSetting(services.AssistantDailyLimitKey, old) })
	if err := database.SetSetting(services.AssistantDailyLimitKey, "2"); err != nil {
		t.Fatal(err)
	}
	for _, client := range []string{"Web", "MicroMessenger"} {
		res := chatRequest(t, token, client, map[string]any{"message": "推荐一道晚餐"})
		if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "event: quota") || !strings.Contains(res.Body.String(), "event: done") {
			t.Fatalf("chat status=%d body=%s", res.Code, res.Body.String())
		}
	}
	st, response := call(t, "GET", "/api/assistant/status", token, nil)
	must(t, st, response, http.StatusOK)
	status := decode[struct {
		Quota services.AssistantQuota `json:"quota"`
	}](t, response.Data)
	if status.Quota.Used != 2 || status.Quota.Remaining != 0 {
		t.Fatalf("quota=%+v", status.Quota)
	}
	var sessions []models.ChatSession
	if err := database.DB.Where("user_id = ?", user.ID).Find(&sessions).Error; err != nil {
		t.Fatal(err)
	}
	for _, session := range sessions {
		st, response = call(t, "DELETE", fmt.Sprintf("/api/assistant/sessions/%d", session.ID), token, nil)
		must(t, st, response, http.StatusOK)
	}
	res := chatRequest(t, token, "Web", map[string]any{"message": "新建对话不能重置次数"})
	if res.Code != http.StatusTooManyRequests || strings.Contains(res.Header().Get("Content-Type"), "event-stream") || res.Header().Get("Retry-After") == "" {
		t.Fatalf("quota must reject before SSE: %d %s", res.Code, res.Body.String())
	}
	var rejected apiResp
	if err := json.Unmarshal(res.Body.Bytes(), &rejected); err != nil {
		t.Fatal(err)
	}
	if rejected.Code != 42901 || !strings.Contains(rejected.Message, "次数已用完") {
		t.Fatalf("unfriendly rejection: %+v", rejected)
	}
	_, otherToken, err := testutil.NewUser("quota-page-other@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	st, response = call(t, "GET", "/api/assistant/status", otherToken, nil)
	must(t, st, response, http.StatusOK)
	other := decode[struct {
		Quota services.AssistantQuota `json:"quota"`
	}](t, response.Data)
	if other.Quota.Remaining != 2 {
		t.Fatal("another account lost quota")
	}
}

func TestAssistantQuotaInvalidRequestsAndAdminPermissions(t *testing.T) {
	user, token, err := testutil.NewUser("quota-validation@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	owner, _, err := testutil.NewUser("quota-session-owner@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	foreign := models.ChatSession{UserID: owner.ID, Title: "private"}
	if err := database.DB.Create(&foreign).Error; err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		body any
		want int
	}{
		{map[string]any{"message": "   "}, http.StatusBadRequest},
		{map[string]any{"message": strings.Repeat("字", 1001)}, http.StatusBadRequest},
		{map[string]any{"message": "继续", "session_id": foreign.ID}, http.StatusNotFound},
	} {
		res := chatRequest(t, token, "Web", row.body)
		if res.Code != row.want {
			t.Fatalf("invalid request: %d %s", res.Code, res.Body.String())
		}
	}
	var count int64
	if err := database.DB.Model(&models.AssistantUsage{}).Where("user_id = ?", user.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("invalid requests consumed quota: %d %v", count, err)
	}
	st, response := call(t, "PUT", "/api/admin/settings", token, map[string]any{"settings": map[string]string{services.AssistantDailyLimitKey: "1"}})
	must(t, st, response, http.StatusForbidden)
	admin, _, err := testutil.NewUser("quota-admin@qq.com")
	if err != nil {
		t.Fatal(err)
	}
	admin.Role = models.RoleAdmin
	if err := database.DB.Model(&admin).Update("role", admin.Role).Error; err != nil {
		t.Fatal(err)
	}
	adminToken, _, err := auth.IssueUserToken(&admin)
	if err != nil {
		t.Fatal(err)
	}
	old := database.GetSetting(services.AssistantDailyLimitKey, "20")
	t.Cleanup(func() { _ = database.SetSetting(services.AssistantDailyLimitKey, old) })
	for _, value := range []string{"21", "-1", "2.5", ""} {
		st, response = call(t, "PUT", "/api/admin/settings", adminToken, map[string]any{"settings": map[string]string{services.AssistantDailyLimitKey: value}})
		must(t, st, response, http.StatusBadRequest)
	}
	st, response = call(t, "PUT", "/api/admin/settings", adminToken, map[string]any{"settings": map[string]string{services.AssistantDailyLimitKey: "0"}})
	must(t, st, response, http.StatusOK)
	res := chatRequest(t, token, "MicroMessenger", map[string]any{"message": "你好"})
	if res.Code != http.StatusTooManyRequests || !strings.Contains(res.Body.String(), "暂停") {
		t.Fatalf("pause not applied: %d %s", res.Code, res.Body.String())
	}
	st, response = call(t, "PUT", "/api/settings", token, map[string]any{"settings": map[string]string{services.AssistantDailyLimitKey: "20"}})
	must(t, st, response, http.StatusOK)
	if database.GetSetting(services.AssistantDailyLimitKey, "20") != "0" {
		t.Fatal("personal settings bypassed admin quota")
	}
}
