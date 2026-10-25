package services

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	mathrand "math/rand"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
)

// RecommendationContext is request-local and reusable when planning many meals.
type RecommendationContext struct {
	uid       uint
	db        *gorm.DB
	profile   *TasteProfile
	dishes    []models.Dish
	blocked   []string
	inventory map[string]bool
	plans     []models.HealthPlanItem
	quality   map[uint]float64
	exposures map[uint]int
	events    []models.BehaviorEvent
	persisted bool
}

func NewRecommendationContext(uid uint, profileDays int, dbs ...*gorm.DB) (*RecommendationContext, error) {
	db := database.Handle(dbs...)
	blocked, err := loadDietaryExclusions(uid, db)
	if err != nil {
		return nil, err
	}
	ctx := &RecommendationContext{uid: uid, db: db, profile: BuildTasteProfile(uid, profileDays, db), dishes: VisibleEnabledDishes(uid, db), blocked: blocked, inventory: map[string]bool{}, quality: map[uint]float64{}, exposures: map[uint]int{}}
	if ctx.profile.loadErr != nil {
		return nil, ctx.profile.loadErr
	}
	var inventory []models.HomeInventory
	if err = db.Scopes(database.OwnedBy(uid)).Where("in_stock = ?", true).Find(&inventory).Error; err != nil {
		return nil, err
	}
	for _, i := range inventory {
		ctx.inventory[canonicalIngredient(i.ItemName)] = true
	}
	if err = db.Scopes(database.OwnedBy(uid)).Where("meal_date >= ?", recommendationNow().AddDate(0, 0, -7).Format("2006-01-02")).Find(&ctx.plans).Error; err != nil {
		return nil, err
	}
	if err = db.Scopes(database.OwnedBy(uid)).Where("created_at >= ?", recommendationDay(recommendationNow())).Order("created_at ASC, id ASC").Find(&ctx.events).Error; err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, e := range ctx.events {
		if e.EventType != "impression" {
			continue
		}
		var meta struct {
			RecommendationID string `json:"recommendation_id"`
		}
		_ = json.Unmarshal([]byte(e.Meta), &meta)
		key := fmt.Sprintf("%s:%d", meta.RecommendationID, e.DishID)
		if meta.RecommendationID != "" && !seen[key] {
			ctx.exposures[e.DishID]++
			seen[key] = true
		}
	}
	var rows []struct {
		ID         uint
		StepLength int
	}
	if err = db.Model(&models.Dish{}).Scopes(database.VisibleDishes(uid)).Select("id, COALESCE(LENGTH(steps),0) AS step_length").Where("enabled = ?", true).Find(&rows).Error; err != nil {
		return nil, err
	}
	stepLengths := map[uint]int{}
	for _, r := range rows {
		stepLengths[r.ID] = r.StepLength
	}
	for i, d := range ctx.dishes {
		ctx.dishes[i].Favorite = ctx.profile.favoriteSet[d.ID]
		q := 40.0
		if len(ingredientNames(d.Ingredients)) > 0 {
			q += 20
		}
		if d.Seasonings != "" {
			q += 10
		}
		if stepLengths[d.ID] > 2 {
			q += 20
		}
		if d.CookTime > 0 {
			q += 10
		}
		ctx.quality[d.ID] = q
	}
	return ctx, nil
}

func RecommendWithContext(ctx *RecommendationContext, req RecommendRequest) (*RecommendResponse, error) {
	if ctx == nil {
		return nil, errors.New("推荐上下文不存在")
	}
	count := req.Count
	if count < 1 {
		count = 3
	}
	if count > 10 {
		count = 10
	}
	req.Count = count
	meal := strings.TrimSpace(req.MealType)
	if meal != "lunch" && meal != "dinner" && meal != "breakfast" {
		meal = ""
	}
	req.MealType = meal
	date := recommendationDay(recommendationNow())
	if strings.TrimSpace(req.MealDate) != "" {
		var err error
		date, err = time.ParseInLocation("2006-01-02", strings.TrimSpace(req.MealDate), recommendationLocation)
		if err != nil {
			return nil, ErrInvalidDate
		}
	}
	if date.Before(recommendationDay(recommendationNow()).AddDate(0, 0, -7)) || date.After(recommendationDay(recommendationNow()).AddDate(0, 0, 30)) {
		return nil, ErrInvalidDate
	}
	req.MealDate = date.Format("2006-01-02")
	recentDays := ctx.profile.RepeatDays
	if req.ExcludeRecentDays != nil {
		recentDays = max(0, min(365, *req.ExcludeRecentDays))
	}
	diversity := req.Diversity == nil || *req.Diversity
	applied := map[string]any{"meal_type": meal, "meal_date": req.MealDate, "mood": strings.TrimSpace(req.Mood), "count": count, "exclude_recent_days": recentDays, "diversity": diversity, "profile_days": ctx.profile.WindowDays, "preferences_applied": true, "relaxed": []string{}, "score_scale": "internal_0_100", "algorithm_version": RecommendationAlgorithmVersion}
	response := &RecommendResponse{Items: []RecommendItem{}, ServedForDate: req.MealDate, MealType: meal, AlgorithmVersion: RecommendationAlgorithmVersion, ProfileSummary: ctx.profile.Summary, Applied: applied}
	excluded := map[uint]bool{}
	for _, id := range req.ExcludeDishIDs {
		excluded[id] = true
	}
	for _, id := range req.SelectedDishIDs {
		excluded[id] = true
	}
	selected := []models.Dish{}
	byID := map[uint]models.Dish{}
	for _, d := range ctx.dishes {
		byID[d.ID] = d
	}
	for _, id := range req.SelectedDishIDs {
		if d, ok := byID[id]; ok {
			selected = append(selected, d)
		}
	}
	for _, p := range ctx.plans {
		if p.MealDate == req.MealDate {
			excluded[p.DishID] = true
			if p.MealType == meal {
				if d, ok := byID[p.DishID]; ok {
					selected = append(selected, d)
				}
			}
		}
	}
	req.confirmedDishIDs = make(map[uint]bool, len(selected))
	for _, dish := range selected {
		req.confirmedDishIDs[dish.ID] = true
	}
	for id, last := range ctx.profile.LastEaten {
		if last == req.MealDate {
			excluded[id] = true
		}
	}
	sessionID := ""
	if req.SessionID == "" && req.Mode == "home_auto" {
		var latest models.RecommendationSnapshot
		err := ctx.db.Where("user_id = ? AND meal_date = ? AND meal_type = ? AND algorithm_version = ?", ctx.uid, req.MealDate, meal, RecommendationAlgorithmVersion).Order("created_at DESC, id DESC").First(&latest).Error
		if err == nil {
			sessionID = latest.SessionID
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	if req.SessionID != "" {
		var err error
		sessionID, err = RecommendationSessionID(ctx.uid, req.SessionID, ctx.db)
		if err != nil {
			return nil, err
		}
		var session models.RecommendationSnapshot
		if err = ctx.db.Where("id = ? AND user_id = ?", sessionID, ctx.uid).First(&session).Error; err != nil {
			return nil, err
		}
		if session.MealDate != req.MealDate || session.MealType != meal {
			return nil, errors.New("推荐会话餐次或日期已变化，请重新推荐")
		}
	}
	// Skip is a session action, while not_today is a current-day action. Cached
	// refreshes do not count prefetched cards as actual impressions.
	for _, e := range ctx.events {
		if e.EventType != "skip" && e.EventType != "not_today" && !(e.EventType == "impression" && (req.Refresh || len(req.ExcludeDishIDs) > 0)) {
			continue
		}
		var meta struct {
			SessionID        string `json:"session_id"`
			RecommendationID string `json:"recommendation_id"`
		}
		_ = json.Unmarshal([]byte(e.Meta), &meta)
		if sessionID != "" && (meta.SessionID == sessionID || meta.RecommendationID == sessionID) {
			excluded[e.DishID] = true
		}
	}
	for id := range ctx.profile.notToday {
		if req.MealDate == recommendationNow().Format("2006-01-02") {
			excluded[id] = true
		}
	}
	recent := map[uint]bool{}
	for id, last := range ctx.profile.LastEaten {
		t, err := time.ParseInLocation("2006-01-02", last, recommendationLocation)
		if err == nil {
			age := date.Sub(t).Hours() / 24
			if recentDays > 0 && age >= 0 && age <= float64(recentDays) {
				recent[id] = true
			}
		}
	}
	filter := candidateFilter{mealType: meal, categories: stringSet(req.Categories), tastes: cleanList(req.Tastes), include: cleanList(req.IncludeIngredients), hardExclude: append(append([]string{}, ctx.blocked...), cleanList(req.ExcludeIngredients)...), keyword: strings.ToLower(strings.TrimSpace(req.Keyword)), difficulty: strings.TrimSpace(req.Difficulty), maxCookTime: req.MaxCookTime, excluded: excluded, recent: recent, applyTastes: true, applyRecent: true, applyAvoid: true}
	candidates := filterCandidates(ctx.dishes, filter)
	relaxed := []string{}
	if len(candidates) == 0 && len(filter.tastes) > 0 {
		filter.applyTastes = false
		relaxed = append(relaxed, "tastes")
		candidates = filterCandidates(ctx.dishes, filter)
	}
	if len(candidates) == 0 && len(recent) > 0 {
		filter.applyRecent = false
		relaxed = append(relaxed, "recent")
		candidates = filterCandidates(ctx.dishes, filter)
	}
	if req.Mode == "tomorrow_favorite" {
		favorites := []models.Dish{}
		for _, d := range candidates {
			if ctx.profile.favoriteSet[d.ID] {
				favorites = append(favorites, d)
			}
		}
		if len(favorites) > 0 {
			candidates = favorites
		} else {
			relaxed = append(relaxed, "favorites")
		}
	}
	applied["relaxed"] = relaxed
	response.CandidateCount = len(candidates)
	if len(candidates) == 0 {
		applied["empty_reason"] = "没有满足当前餐次、排除条件和忌口的菜品"
		return response, nil
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
	contextKey := recommendationContextKey(ctx, req, candidates, selected)
	persist := req.Mode != "week_plan"
	if persist && !req.Refresh {
		var snapshot models.RecommendationSnapshot
		q := ctx.db.Where("user_id = ? AND meal_date = ? AND meal_type = ? AND context_key = ? AND algorithm_version = ?", ctx.uid, req.MealDate, meal, contextKey, RecommendationAlgorithmVersion)
		if sessionID != "" {
			q = q.Where("session_id = ?", sessionID)
		}
		if err := q.Order("created_at DESC, id DESC").First(&snapshot).Error; err == nil {
			if items, ok := rehydrateRecommendation(snapshot, candidates); ok {
				response.Items = items
				response.RecommendationID = snapshot.ID
				response.SessionID = snapshot.SessionID
				return response, nil
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	id := newRecommendationID()
	if sessionID == "" {
		sessionID = id
	}
	seed := contextKey
	if req.Refresh {
		seed += id
	}
	sum := sha256.Sum256([]byte(seed))
	var n int64
	for _, v := range sum[:8] {
		n = n<<8 | int64(v)
	}
	rng := mathrand.New(mathrand.NewSource(n))
	picked := []RecommendItem{}
	pool := append([]models.Dish{}, candidates...)
	for len(pool) > 0 && len(picked) < count {
		scored := make([]RecommendItem, 0, len(pool))
		for _, d := range pool {
			score, reasons, parts := scoreDishV2(d, req, ctx.profile, date, selected, ctx.inventory, ctx.quality[d.ID], ctx.exposures)
			d.Favorite = ctx.profile.favoriteSet[d.ID]
			scored = append(scored, RecommendItem{Dish: d, Score: score, Reasons: reasons, parts: parts})
		}
		ties := map[uint]float64{}
		for _, item := range scored {
			ties[item.Dish.ID] = rng.Float64()
		}
		sort.SliceStable(scored, func(i, j int) bool {
			if scored[i].Score != scored[j].Score {
				return scored[i].Score > scored[j].Score
			}
			return ties[scored[i].Dish.ID] > ties[scored[j].Dish.ID]
		})
		chosen := scored[0]
		if diversity && ctx.profile.WindowRecords > 0 && rng.Float64() < .2 {
			explorers := []RecommendItem{}
			for _, it := range scored {
				if ctx.profile.eatenCount[it.Dish.ID] == 0 && !ctx.profile.dislikedSet[it.Dish.ID] && it.Score >= scored[0].Score-12 {
					explorers = append(explorers, it)
				}
			}
			if len(explorers) > 0 {
				chosen = explorers[rng.Intn(len(explorers))]
			}
		}
		chosen.ItemID = fmt.Sprintf("%s:%d", id, chosen.Dish.ID)
		picked = append(picked, chosen)
		selected = append(selected, chosen.Dish)
		for i, d := range pool {
			if d.ID == chosen.Dish.ID {
				pool = append(pool[:i], pool[i+1:]...)
				break
			}
		}
	}
	response.Items = picked
	if persist {
		created := false
		err := ctx.db.Transaction(func(tx *gorm.DB) error {
			// Serialize the check/create boundary per user so simultaneous Web
			// and Agent reads converge on the same daily snapshot.
			if err := tx.Model(&models.User{}).Where("id = ?", ctx.uid).UpdateColumn("id", gorm.Expr("id")).Error; err != nil {
				return err
			}
			if !req.Refresh {
				var existing models.RecommendationSnapshot
				q := tx.Where("user_id = ? AND meal_date = ? AND meal_type = ? AND context_key = ? AND algorithm_version = ?", ctx.uid, req.MealDate, meal, contextKey, RecommendationAlgorithmVersion)
				if req.SessionID != "" {
					q = q.Where("session_id = ?", sessionID)
				}
				err := q.Order("created_at DESC, id DESC").First(&existing).Error
				if err == nil {
					if items, ok := rehydrateRecommendation(existing, candidates); ok {
						response.Items = items
						response.RecommendationID = existing.ID
						response.SessionID = existing.SessionID
						return nil
					}
				} else if !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
			}
			if !ctx.persisted {
				projection := *ctx
				projection.db = tx
				if err := persistAffinities(&projection); err != nil {
					return err
				}
			}
			payload, _ := json.Marshal(snapshotItems(picked))
			snapshot := models.RecommendationSnapshot{ID: id, UserID: ctx.uid, MealDate: req.MealDate, MealType: meal, ContextKey: contextKey, SessionID: sessionID, Payload: string(payload), AlgorithmVersion: RecommendationAlgorithmVersion}
			if err := tx.Create(&snapshot).Error; err != nil {
				return err
			}
			if err := pruneRecommendationSnapshots(ctx.uid, tx); err != nil {
				return err
			}
			response.RecommendationID = id
			response.SessionID = sessionID
			created = true
			return nil
		})
		if err != nil {
			return nil, err
		}
		if created {
			ctx.persisted = true
			req.SessionID = sessionID
			logRecommendEvents(ctx.uid, picked, req, meal, ctx.db)
		}
	}
	return response, nil
}

func persistAffinities(ctx *RecommendationContext) error {
	rows := make([]models.UserDishAffinity, 0, len(ctx.dishes))
	now := recommendationNow()
	for _, d := range ctx.dishes {
		affinity, confidence, samples := affinityForDish(d, ctx.profile)
		rows = append(rows, models.UserDishAffinity{UserID: ctx.uid, DishID: d.ID, Affinity: affinity, Confidence: confidence, EffectiveSamples: samples, LastEaten: ctx.profile.LastEaten[d.ID], AlgorithmVersion: RecommendationAlgorithmVersion, UpdatedAt: now})
	}
	if len(rows) == 0 {
		return nil
	}
	return ctx.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}, {Name: "dish_id"}}, DoUpdates: clause.AssignmentColumns([]string{"affinity", "confidence", "effective_samples", "last_eaten", "algorithm_version", "updated_at"})}).CreateInBatches(rows, 100).Error
}
func newRecommendationID() string {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		panic("cryptographic randomness unavailable")
	}
	return hex.EncodeToString(b)
}

func recommendationContextKey(ctx *RecommendationContext, req RecommendRequest, candidates, selected []models.Dish) string {
	req.Source = ""
	req.Actor = ""
	req.Mode = ""
	req.SessionID = ""
	req.Refresh = false
	req.Jitter = false
	payload := struct {
		Version              string
		Request              RecommendRequest
		Preferences          Preferences
		Blocked              []string
		Favorites            map[uint]bool
		Evidence             map[uint]*affinityEvidence
		LastEaten            map[uint]string
		Candidates, Selected []models.Dish
		Inventory            map[string]bool
		Quality              map[uint]float64
	}{RecommendationAlgorithmVersion + ":reasons-v2", req, ctx.profile.Preferences, ctx.blocked, ctx.profile.favoriteSet, ctx.profile.dishEvidence, ctx.profile.LastEaten, candidates, selected, ctx.inventory, ctx.quality}
	raw, _ := json.Marshal(payload)
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}

type recommendationStoredItem struct {
	DishID  uint                     `json:"dish_id"`
	Score   float64                  `json:"score"`
	Reasons []string                 `json:"reasons"`
	Parts   recommendationScoreParts `json:"parts"`
}

func snapshotItems(items []RecommendItem) []recommendationStoredItem {
	rows := make([]recommendationStoredItem, 0, len(items))
	for _, i := range items {
		rows = append(rows, recommendationStoredItem{i.Dish.ID, i.Score, i.Reasons, i.parts})
	}
	return rows
}
func rehydrateRecommendation(snapshot models.RecommendationSnapshot, candidates []models.Dish) ([]RecommendItem, bool) {
	var rows []recommendationStoredItem
	if json.Unmarshal([]byte(snapshot.Payload), &rows) != nil || len(rows) == 0 || len(rows) > 10 {
		return nil, false
	}
	byID := map[uint]models.Dish{}
	for _, d := range candidates {
		byID[d.ID] = d
	}
	result := make([]RecommendItem, 0, len(rows))
	for _, row := range rows {
		d, ok := byID[row.DishID]
		if !ok {
			return nil, false
		}
		result = append(result, RecommendItem{ItemID: fmt.Sprintf("%s:%d", snapshot.ID, d.ID), Dish: d, Score: row.Score, Reasons: row.Reasons, parts: row.Parts})
	}
	return result, true
}

var ErrRecommendationNotFound = errors.New("推荐不存在或不属于当前用户")

func ValidateRecommendationItem(uid uint, recommendationID string, dishID uint, dbs ...*gorm.DB) error {
	if recommendationID == "" || dishID == 0 {
		return ErrRecommendationNotFound
	}
	var snapshot models.RecommendationSnapshot
	if err := database.Handle(dbs...).Where("id = ? AND user_id = ?", recommendationID, uid).First(&snapshot).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrRecommendationNotFound
		}
		return err
	}
	var rows []recommendationStoredItem
	if json.Unmarshal([]byte(snapshot.Payload), &rows) != nil {
		return ErrRecommendationNotFound
	}
	for _, r := range rows {
		if r.DishID == dishID {
			return nil
		}
	}
	return ErrRecommendationNotFound
}
func RecommendationSessionID(uid uint, recommendationID string, dbs ...*gorm.DB) (string, error) {
	var snapshot models.RecommendationSnapshot
	if err := database.Handle(dbs...).Where("id = ? AND user_id = ?", recommendationID, uid).First(&snapshot).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrRecommendationNotFound
		}
		return "", err
	}
	return snapshot.SessionID, nil
}

// Prune a bounded page on writes; no unbounded scan or background goroutine is
// needed. Old session IDs naturally expire with the thirty-day snapshot window.
func pruneRecommendationSnapshots(uid uint, db *gorm.DB) error {
	var ids []string
	if err := db.Model(&models.RecommendationSnapshot{}).Where("user_id = ? AND created_at < ?", uid, recommendationNow().AddDate(0, 0, -30)).Order("created_at ASC").Limit(50).Pluck("id", &ids).Error; err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	return db.Where("user_id = ? AND id IN ?", uid, ids).Delete(&models.RecommendationSnapshot{}).Error
}
