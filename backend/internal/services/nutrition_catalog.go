package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"math"
	"net/url"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
	"sort"
	"strings"
	"time"
)

const CatalogCalculationVersion = "catalog-per100-v1"

type CatalogFoodInput struct {
	Portions    []models.CatalogPortion  `json:"portions,omitempty"`
	Name        string                   `json:"name"`
	Aliases     []string                 `json:"aliases"`
	BasisUnit   string                   `json:"basis_unit"`
	FoodState   string                   `json:"food_state"`
	EdibleBasis string                   `json:"edible_basis"`
	Nutrients   models.NutrientValues    `json:"nutrients"`
	Provenance  models.CatalogProvenance `json:"provenance"`
}

func ValidateCatalogFoods(inputs []CatalogFoodInput) ([]models.NutritionCatalogFood, error) {
	if len(inputs) < 1 || len(inputs) > 1000 {
		return nil, errors.New("每批需包含1至1000项食物")
	}
	out := make([]models.NutritionCatalogFood, 0, len(inputs))
	seen := map[string]bool{}
	for row, in := range inputs {
		fail := func(message string) ([]models.NutritionCatalogFood, error) {
			return nil, fmt.Errorf("第%d项：%s", row+1, message)
		}
		p := in.Provenance
		for _, value := range []string{p.Dataset, p.Version, p.RecordID} {
			if len([]rune(value)) < 1 || len([]rune(value)) > 80 || strings.TrimSpace(value) != value {
				return fail("来源标识、版本和记录编号需为1至80字，不能带首尾空白")
			}
		}
		if len([]rune(p.License)) < 1 || len([]rune(p.License)) > 500 || strings.TrimSpace(p.License) == "" || len([]rune(p.ReviewedBy)) < 1 || len([]rune(p.ReviewedBy)) > 100 || strings.TrimSpace(p.ReviewedBy) == "" {
			return fail("请提供许可说明与实际核验人")
		}
		if _, err := time.Parse("2006-01-02", p.ReviewedAt); err != nil || p.ReviewedAt > healthToday() {
			return fail("核验日期无效或在未来")
		}
		u, err := url.Parse(p.URL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || len(p.URL) > 500 {
			return fail("来源链接必须为不含凭证的HTTPS地址，最多500字节")
		}
		if in.EdibleBasis != "edible_portion" {
			return fail("必须明确营养数据按可食部分计量，不能推断可食比例")
		}
		if err := validateNutritionFood(NutritionFoodInput{Name: in.Name, BasisUnit: in.BasisUnit, FoodState: in.FoodState, SourceReference: p.URL, Nutrients: in.Nutrients}); err != nil {
			return fail(err.Error())
		}
		if len(in.Portions) > 20 {
			return fail("标准份量最多20项")
		}
		portionKeys := map[string]bool{}
		for _, portion := range in.Portions {
			if portion.Key == "" || len(portion.Key) > 40 || strings.TrimSpace(portion.Key) != portion.Key || portionKeys[portion.Key] {
				return fail("份量标识需唯一、非空且最多40字节，不能带首尾空白")
			}
			portionKeys[portion.Key] = true
			if strings.TrimSpace(portion.Label) == "" || len([]rune(portion.Label)) > 60 || strings.TrimSpace(portion.Reference) == "" || len([]rune(portion.Reference)) > 500 {
				return fail("请提供份量名称（最多60字）和换算来源依据（最多500字）")
			}
			if math.IsNaN(portion.Amount) || math.IsInf(portion.Amount, 0) || portion.Amount < 0.01 || portion.Amount > 10000 {
				return fail("标准份量需为0.01至10000，按同状态可食部分的基准单位计量")
			}
		}
		if len(in.Aliases) > 20 {
			return fail("别名最多20项")
		}
		aliases := []string{}
		aliasSet := map[string]bool{}
		for _, alias := range in.Aliases {
			alias = strings.TrimSpace(alias)
			if alias == "" || len([]rune(alias)) > 100 {
				return fail("别名需为1至100字")
			}
			if !aliasSet[alias] {
				aliases = append(aliases, alias)
				aliasSet[alias] = true
			}
		}
		sort.Strings(aliases)
		in.Aliases = aliases
		in.Name = strings.TrimSpace(in.Name)
		identity, _ := json.Marshal([]string{p.Dataset, p.Version, p.RecordID})
		key := string(identity)
		if seen[key] {
			return fail("同批来源版本和记录编号重复")
		}
		seen[key] = true
		raw, _ := json.Marshal(in)
		sum := sha256.Sum256(raw)
		out = append(out, models.NutritionCatalogFood{Portions: in.Portions, Dataset: p.Dataset, DatasetVersion: p.Version, RecordID: p.RecordID, Name: in.Name, Aliases: aliases, SearchText: strings.ToLower(in.Name + "\n" + strings.Join(aliases, "\n")), BasisUnit: in.BasisUnit, FoodState: in.FoodState, EdibleBasis: in.EdibleBasis, Nutrients: in.Nutrients, Provenance: p, ContentHash: hex.EncodeToString(sum[:]), Enabled: true})
	}
	return out, nil
}

// An existing source revision is immutable; corrections require a new dataset version.
func ImportNutritionCatalog(inputs []CatalogFoodInput, db *gorm.DB) error {
	rows, err := ValidateCatalogFoods(inputs)
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for _, row := range rows {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
				return err
			}
			var existing models.NutritionCatalogFood
			if err := tx.Where("dataset = ? AND dataset_version = ? AND record_id = ?", row.Dataset, row.DatasetVersion, row.RecordID).First(&existing).Error; err != nil {
				return err
			}
			if existing.ContentHash != row.ContentHash {
				return errors.New("来源版本内容冲突，请使用新版本；本批未写入")
			}
		}
		return nil
	})
}

func SearchNutritionCatalog(query, state string, dbs ...*gorm.DB) ([]models.NutritionCatalogFood, error) {
	query = strings.TrimSpace(query)
	if len([]rune(query)) > 100 {
		return nil, errors.New("搜索词最多100字")
	}
	if state != "" && !validFoodState(state) {
		return nil, errors.New("食物状态无效")
	}
	db := database.Handle(dbs...).Where("enabled = ?", true)
	if state != "" {
		db = db.Where("food_state = ?", state)
	}
	if query != "" {
		// Use an explicit escape character for identical SQLite/MySQL LIKE semantics.
		query = strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(strings.ToLower(query))
		db = db.Where("search_text LIKE ? ESCAPE '!'", "%"+query+"%")
	}
	out := []models.NutritionCatalogFood{}
	err := db.Order("name, id").Limit(50).Find(&out).Error
	return out, err
}

// Lock the same source row as adoption. Repeated withdrawal is safe, but an
// unknown ID must not be reported as a successful withdrawal.
func WithdrawNutritionCatalog(id uint, db *gorm.DB) error {
	if id == 0 {
		return ErrFoodEntryNotFound
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var food models.NutritionCatalogFood
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&food, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrFoodEntryNotFound
			}
			return err
		}
		return tx.Model(&food).Update("enabled", false).Error
	})
}

func AdoptNutritionCatalog(uid, id uint, dbs ...*gorm.DB) (*models.NutritionFood, error) {
	var out models.NutritionFood
	err := database.Handle(dbs...).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.User{}).Where("id = ?", uid).UpdateColumn("id", gorm.Expr("id")).Error; err != nil {
			return err
		}
		var source models.NutritionCatalogFood
		if id == 0 {
			return ErrFoodEntryNotFound
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("enabled = ?", true).First(&source, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrFoodEntryNotFound
			}
			return err
		}
		err := tx.Where("user_id = ? AND catalog_id = ?", uid, id).First(&out).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var count int64
		if err := tx.Model(&models.NutritionFood{}).Where("user_id = ?", uid).Count(&count).Error; err != nil {
			return err
		}
		if count >= 300 {
			return errors.New("个人标签和配方最多共300项")
		}
		out = models.NutritionFood{Portions: source.Portions, UserID: uid, CatalogID: &source.ID, Catalog: &source.Provenance, Name: source.Name, BasisUnit: source.BasisUnit, FoodState: source.FoodState, Source: "standard_food", SourceReference: source.Provenance.URL, Nutrients: source.Nutrients, Version: 1, Enabled: true}
		return tx.Create(&out).Error
	})
	return &out, err
}
