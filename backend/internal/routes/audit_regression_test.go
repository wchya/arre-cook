package routes_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"net/http/httptest"
	"ninimenu/internal/auth"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"ninimenu/internal/services"
	"ninimenu/internal/testutil"
	"strings"
	"sync"
	"testing"
	"time"
)

func auditUser(t *testing.T, label string) (models.User, string) {
	t.Helper()
	u, token, err := testutil.NewUser("fixed-" + label + "@example.test")
	if err != nil {
		t.Fatal(err)
	}
	return u, token
}
func auditDish(t *testing.T, uid uint) models.Dish {
	t.Helper()
	d := models.Dish{OwnerID: uid, Name: "regression dish", Enabled: true, MealType: "all", Ingredients: `[{"name":"egg","amount":"1"}]`, Images: "[]", Tags: "[]"}
	if err := database.DB.Create(&d).Error; err != nil {
		t.Fatal(err)
	}
	return d
}
func injectWriteFailure(t *testing.T, kind, table string) {
	t.Helper()
	name := "audit_failure_" + table
	hook := func(tx *gorm.DB) {
		if tx.Statement.Table == table {
			tx.AddError(errors.New("injected storage failure"))
		}
	}
	switch kind {
	case "create":
		database.DB.Callback().Create().Before("gorm:create").Register(name, hook)
		t.Cleanup(func() { database.DB.Callback().Create().Remove(name) })
	case "update":
		database.DB.Callback().Update().Before("gorm:update").Register(name, hook)
		t.Cleanup(func() { database.DB.Callback().Update().Remove(name) })
	case "delete":
		database.DB.Callback().Delete().Before("gorm:delete").Register(name, hook)
		t.Cleanup(func() { database.DB.Callback().Delete().Remove(name) })
	}
}

func TestAuditAllRecipeInsertionsRespectQuota(t *testing.T) {
	u, tok := auditUser(t, "quota")
	rows := make([]models.Dish, 500)
	for i := range rows {
		rows[i] = models.Dish{OwnerID: u.ID, Name: fmt.Sprint("quota-", i), Enabled: true}
	}
	if err := database.DB.CreateInBatches(&rows, 50).Error; err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/dishes", fmt.Sprintf("/api/dishes/%d/clone", rows[0].ID)} {
		status, r := call(t, "POST", path, tok, map[string]string{"name": "overflow"})
		must(t, status, r, 400)
	}
	name := "agent overflow"
	if _, err := services.CreatePrivateRecipe(u.ID, services.PrivateRecipePatch{Name: &name}); !errors.Is(err, services.ErrDishQuota) {
		t.Fatalf("agent quota error=%v", err)
	}
	var count int64
	database.DB.Model(&models.Dish{}).Where("owner_id = ?", u.ID).Count(&count)
	if count != 500 {
		t.Fatalf("count=%d", count)
	}
	// Two concurrent inserts competing for the final slot cannot both commit.
	database.DB.Delete(&rows[0])
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d := models.Dish{OwnerID: u.ID, Name: "last slot", Enabled: true}
			_ = services.InsertDish(database.DB, &d)
		}()
	}
	wg.Wait()
	database.DB.Model(&models.Dish{}).Where("owner_id = ?", u.ID).Count(&count)
	if count != 500 {
		t.Fatalf("concurrent count=%d", count)
	}
}

func TestAuditBoundedMenuSettingsAndLegacyReads(t *testing.T) {
	u, tok := auditUser(t, "settings")
	for _, value := range []string{"0", "-1", "10000", "999999999999999999999999"} {
		status, r := call(t, "PUT", "/api/settings", tok, map[string]any{"settings": map[string]string{"lunch_dishes_per_day": value}})
		must(t, status, r, 400)
	}
	status, r := call(t, "PUT", "/api/settings", tok, map[string]any{"settings": map[string]string{"repeat_days": "3"}, "ignored": strings.Repeat("x", 2<<20)})
	must(t, status, r, 413)
	if err := database.SetUserSetting(u.ID, "lunch_dishes_per_day", "10000"); err != nil {
		t.Fatal(err)
	}
	plan, err := services.GenerateWeekPlan(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, day := range plan.Days {
		if len(day.Lunch) > 10 || len(day.Dinner) > 10 {
			t.Fatal("unbounded legacy setting")
		}
	}
	encoded, err := json.Marshal(plan)
	if err != nil || len(encoded) > 8<<20 {
		t.Fatalf("menu size=%d err=%v", len(encoded), err)
	}
}

func TestAuditFailedSecurityUpdatesDoNotReportSuccess(t *testing.T) {
	u, tok := auditUser(t, "security-failure")
	injectWriteFailure(t, "update", "users")
	status, r := call(t, "PUT", "/api/me/password", tok, map[string]string{"new_password": "ValidNewPass123!"})
	must(t, status, r, 500)
	status, r = call(t, "POST", "/api/me/logout-all", tok, nil)
	must(t, status, r, 500)
	var fresh models.User
	if err := database.DB.First(&fresh, u.ID).Error; err != nil {
		t.Fatal(err)
	}
	if fresh.TokenVersion != u.TokenVersion || fresh.PasswordHash != "" {
		t.Fatal("failed writes changed credentials")
	}
}

func TestAuditPasswordCannotRestoreRevokedGeneration(t *testing.T) {
	u, tok := auditUser(t, "generation")
	previous := u
	previous.TokenVersion = 2
	revoked, _, err := auth.IssueUserToken(&previous)
	if err != nil {
		t.Fatal(err)
	}
	ran := false
	name := "audit_version_interleaving"
	database.DB.Callback().Update().Before("gorm:begin_transaction").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "users" && !ran {
			ran = true
			tx.AddError(database.DB.Model(&models.User{}).Where("id = ?", u.ID).UpdateColumn("token_version", gorm.Expr("token_version + 2")).Error)
		}
	})
	defer database.DB.Callback().Update().Remove(name)
	status, r := call(t, "PUT", "/api/me/password", tok, map[string]string{"new_password": "ValidNewPass123!"})
	must(t, status, r, 409)
	var fresh models.User
	database.DB.First(&fresh, u.ID)
	if !ran || fresh.TokenVersion != 3 || fresh.PasswordHash != "" {
		t.Fatalf("ran=%v generation=%d", ran, fresh.TokenVersion)
	}
	status, r = call(t, "GET", "/api/me", revoked, nil)
	must(t, status, r, 401)
}

func TestAuditMenuResolvesCurrentContentAndVisibility(t *testing.T) {
	u, tok := auditUser(t, "cache")
	d := auditDish(t, u.ID)
	now := time.Now()
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	monday := now.AddDate(0, 0, 1-weekday).Format("2006-01-02")
	b, _ := json.Marshal(services.WeekPlan{Days: []services.WeekDayPlan{{Date: monday, Lunch: []models.Dish{d}}}})
	if err := database.SetUserSetting(u.ID, "week_plan_cache", string(b)); err != nil {
		t.Fatal(err)
	}
	status, r := call(t, "PUT", fmt.Sprintf("/api/dishes/%d", d.ID), tok, map[string]string{"name": "new name", "ingredients": `[{"name":"new ingredient","amount":"2g"}]`})
	must(t, status, r, 200)
	got := services.GetCachedWeekPlan(u.ID)
	if len(got.Days) == 0 || len(got.Days[0].Lunch) == 0 || got.Days[0].Lunch[0].Name != "new name" || !strings.Contains(got.Days[0].Lunch[0].Ingredients, "new ingredient") {
		t.Fatal("stale menu content")
	}
	status, r = call(t, "PUT", fmt.Sprintf("/api/dishes/%d/toggle", d.ID), tok, nil)
	must(t, status, r, 200)
	got = services.GetCachedWeekPlan(u.ID)
	for _, day := range got.Days {
		for _, meal := range [][]models.Dish{day.Lunch, day.Dinner} {
			for _, item := range meal {
				if item.ID == d.ID {
					t.Fatal("disabled recipe survived cache")
				}
			}
		}
	}
}

func TestAuditMealAndShoppingRollbackTogether(t *testing.T) {
	u, _ := auditUser(t, "shopping")
	d := auditDish(t, u.ID)
	injectWriteFailure(t, "create", "shopping_checks")
	rec, err := services.CreateMealRecord(u.ID, services.MealInput{DishID: d.ID, MealType: "lunch"}, "app", "")
	if err == nil || rec != nil {
		t.Fatal("partial commit reported success")
	}
	var meals, shopping int64
	database.DB.Model(&models.MealRecord{}).Where("user_id = ?", u.ID).Count(&meals)
	database.DB.Model(&models.ShoppingCheck{}).Where("user_id = ?", u.ID).Count(&shopping)
	if meals != 0 || shopping != 0 {
		t.Fatalf("meals=%d shopping=%d", meals, shopping)
	}
}
func TestAuditMealDeletionRollback(t *testing.T) {
	u, _ := auditUser(t, "delete-shopping")
	d := auditDish(t, u.ID)
	rec, err := services.CreateMealRecord(u.ID, services.MealInput{DishID: d.ID, MealType: "lunch"}, "app", "")
	if err != nil {
		t.Fatal(err)
	}
	injectWriteFailure(t, "delete", "shopping_checks")
	if _, err := services.DeleteMealRecord(u.ID, rec.ID); err == nil {
		t.Fatal("failed cleanup ignored")
	}
	var count int64
	database.DB.Model(&models.MealRecord{}).Where("id = ?", rec.ID).Count(&count)
	if count != 1 {
		t.Fatal("meal deletion did not roll back")
	}
}
func TestAuditCanceledRequestDoesNotReadAuthenticatedUser(t *testing.T) {
	_, tok := auditUser(t, "context")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest("GET", "/api/me", nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code == 200 {
		t.Fatal("cancelled database request returned authenticated data")
	}
}
func TestAuditTransferAndLeavePreserveAdministrator(t *testing.T) {
	owner, _ := auditUser(t, "owner")
	member, _ := auditUser(t, "member")
	family, err := services.CreateFamily(owner.ID, "audit family")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.DB.Create(&models.FamilyMember{FamilyID: family.ID, UserID: member.ID, JoinedAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	ran := false
	name := "audit_family_interleaving"
	database.DB.Callback().Update().Before("gorm:begin_transaction").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "families" && !ran {
			ran = true
			tx.AddError(services.LeaveFamily(member.ID))
		}
	})
	err = services.TransferFamily(owner.ID, member.ID)
	database.DB.Callback().Update().Remove(name)
	if !ran || err == nil {
		t.Fatalf("racing transfer should reject departed member: ran=%v err=%v", ran, err)
	}
	var fresh models.Family
	database.DB.First(&fresh, family.ID)
	var count int64
	database.DB.Model(&models.FamilyMember{}).Where("family_id = ? AND user_id = ?", family.ID, fresh.OwnerID).Count(&count)
	if count != 1 {
		t.Fatal("family lost administrator")
	}
	if err := services.RenameFamily(owner.ID, "still managed"); err != nil {
		t.Fatal(err)
	}
}
func TestAuditSuggestionFailureDoesNotConsumePendingState(t *testing.T) {
	u, _ := auditUser(t, "suggestion")
	d := auditDish(t, u.ID)
	s := models.AgentSuggestion{UserID: u.ID, DishIDs: fmt.Sprintf("[%d]", d.ID), MealType: "lunch", Status: "pending"}
	if err := database.DB.Create(&s).Error; err != nil {
		t.Fatal(err)
	}
	injectWriteFailure(t, "create", "meal_records")
	rows, err := services.ResolveSuggestion(u.ID, s.ID, services.ResolveInput{Accept: true})
	if err == nil || len(rows) != 0 {
		t.Fatal("failed acceptance reported success")
	}
	var fresh models.AgentSuggestion
	database.DB.First(&fresh, s.ID)
	if fresh.Status != "pending" {
		t.Fatalf("status=%s", fresh.Status)
	}
}
func TestAuditSuggestionRetryAndFamilyPlanRollback(t *testing.T) {
	u, _ := auditUser(t, "suggestion-retry")
	d := auditDish(t, u.ID)
	s := models.AgentSuggestion{UserID: u.ID, DishIDs: fmt.Sprintf("[%d]", d.ID), MealType: "lunch", Status: "pending"}
	if err := database.DB.Create(&s).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := services.ResolveSuggestion(u.ID, s.ID, services.ResolveInput{Accept: true}); err != nil {
			t.Fatal(err)
		}
	}
	var count int64
	database.DB.Model(&models.MealRecord{}).Where("user_id = ?", u.ID).Count(&count)
	if count != 1 {
		t.Fatalf("replayed meals=%d", count)
	}
	family, err := services.CreateFamily(u.ID, "plan rollback")
	if err != nil {
		t.Fatal(err)
	}
	d.FamilyID = family.ID
	if err := database.DB.Save(&d).Error; err != nil {
		t.Fatal(err)
	}
	injectWriteFailure(t, "create", "family_shopping_checks")
	if err := services.SetFamilyPlan(u.ID, services.Today(), "dinner", d.ID); err == nil {
		t.Fatal("shopping failure ignored")
	}
	database.DB.Model(&models.FamilyPlanItem{}).Where("family_id = ?", family.ID).Count(&count)
	if count != 0 {
		t.Fatal("family plan partially committed")
	}
}
