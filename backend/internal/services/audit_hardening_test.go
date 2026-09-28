package services

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"ninimenu/internal/testutil"
)

func TestAchievementQueueBoundsAndCoalescesUsers(t *testing.T) {
	achievementSyncMu.Lock()
	oldQueue, oldPending := achievementQueue, achievementPending
	achievementQueue, achievementPending = make(chan uint, 128), map[uint]bool{}
	achievementSyncMu.Unlock()
	t.Cleanup(func() {
		achievementSyncMu.Lock()
		achievementQueue, achievementPending = oldQueue, oldPending
		achievementSyncMu.Unlock()
	})
	for uid := uint(1); uid <= 10000; uid++ {
		QueueAutoAchievementSync(uid)
		QueueAutoAchievementSync(uid)
	}
	if len(achievementQueue) != 128 || len(achievementPending) != 128 {
		t.Fatalf("unbounded or duplicate work: queued=%d pending=%d", len(achievementQueue), len(achievementPending))
	}
	if !achievementPending[1] {
		t.Fatal("repeated request did not mark a running/queued user dirty")
	}
	uid := <-achievementQueue
	delete(achievementPending, uid)
	QueueAutoAchievementSync(10000)
	if _, exists := achievementPending[10000]; !exists {
		t.Fatal("overflow permanently prevented later retry")
	}
}

func TestConcurrentDeploymentAndRollbackDoNotRepeatNotices(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "notices.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, _ := db.DB()
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { pool.Close() })
	if err := db.AutoMigrate(&models.User{}, &models.Notification{}, &models.Setting{}, &models.TaskClaim{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.User{Nickname: "recipient"}).Error; err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := AnnounceDeployment("1.0.0", db.Session(&gorm.Session{})); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	for _, version := range []string{"1.0.1", "1.0.0"} {
		if err := AnnounceDeployment(version, db); err != nil {
			t.Fatal(err)
		}
	}
	var count int64
	if err := db.Model(&models.Notification{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("notices=%d, want one per version", count)
	}
}

func TestDishSearchPagesKeepDetailsAndSeasoningFilters(t *testing.T) {
	u, _, err := testutil.NewUser("bounded-search@example.test")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		d := models.Dish{OwnerID: u.ID, Name: "page", SortOrder: 3 - i, Enabled: true,
			Ingredients: "[]", Seasonings: `[{"name":"sesame"}]`, Steps: `[{"text":"full instructions"}]`}
		if err := database.DB.Create(&d).Error; err != nil {
			t.Fatal(err)
		}
	}
	got, total := SearchDishes(u.ID, DishQuery{OnlyMine: true, Keyword: "sesame", Offset: 1, Limit: 1})
	if total != 3 || len(got) != 1 || got[0].SortOrder != 2 || !strings.Contains(got[0].Steps, "full instructions") {
		t.Fatalf("page=%+v total=%d", got, total)
	}
}

func TestMenuChecksSeasoningAllergiesAfterRecipeChanges(t *testing.T) {
	u, _, err := testutil.NewUser("menu-seasoning@example.test")
	if err != nil {
		t.Fatal(err)
	}
	d := models.Dish{OwnerID: u.ID, Name: "safe before edit", Enabled: true, MealType: "all", Ingredients: "[]", Seasonings: "[]", Tags: "[]", Images: "[]"}
	if err := database.DB.Create(&d).Error; err != nil {
		t.Fatal(err)
	}
	allergies := []string{"sesame"}
	if _, err := SavePreferences(u.ID, PreferencesPatch{Allergies: &allergies}); err != nil {
		t.Fatal(err)
	}
	plan := &WeekPlan{Days: []WeekDayPlan{{Date: getCurrentWeekKey(), Lunch: []models.Dish{d}}}}
	saveWeekPlanCache(u.ID, plan)
	raw := database.GetUserSetting(u.ID, "week_plan_cache", "")
	if strings.Contains(raw, "safe before edit") || len(raw) > 512 {
		t.Fatal("cache contains recipe payload")
	}
	if err := database.DB.Model(&d).Update("seasonings", `[{"name":"sesame"}]`).Error; err != nil {
		t.Fatal(err)
	}
	for _, day := range GetCachedWeekPlan(u.ID).Days {
		for _, meal := range [][]models.Dish{day.Lunch, day.Dinner} {
			for _, dish := range meal {
				if dish.ID == d.ID {
					t.Fatal("allergen added after caching survived hydration")
				}
			}
		}
	}
}
