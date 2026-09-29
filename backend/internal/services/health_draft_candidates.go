package services

import (
	"errors"
	"sort"
	"strings"

	"gorm.io/gorm"
	"ninimenu/internal/database"
	"ninimenu/internal/models"
)

type HealthDraftCandidates struct {
	Personal []models.NutritionFood        `json:"personal"`
	Catalog  []models.NutritionCatalogFood `json:"catalog"`
}

var ErrHealthDraftCandidateQuery = errors.New("搜索词需为1至100字")

func healthDraftNameRank(name, query string) int {
	name = strings.ToLower(strings.TrimSpace(name))
	query = strings.ToLower(strings.TrimSpace(query))
	switch {
	case name == query:
		return 0
	case strings.HasPrefix(name, query):
		return 1
	case strings.Contains(name, query):
		return 2
	default:
		return 3
	}
}

// Candidates are suggestions only; neither a match nor an adopted food creates intake.
func FindHealthDraftCandidates(uid uint, query string, dbs ...*gorm.DB) (HealthDraftCandidates, error) {
	query = strings.TrimSpace(query)
	if n := len([]rune(query)); n < 1 || n > 100 {
		return HealthDraftCandidates{}, ErrHealthDraftCandidateQuery
	}
	db := database.Handle(dbs...)
	personal, err := ListNutritionFoods(uid, db)
	if err != nil {
		return HealthDraftCandidates{}, err
	}
	out := HealthDraftCandidates{Personal: []models.NutritionFood{}, Catalog: []models.NutritionCatalogFood{}}
	for _, food := range personal {
		if healthDraftNameRank(food.Name, query) < 3 {
			out.Personal = append(out.Personal, food)
		}
	}
	sort.SliceStable(out.Personal, func(i, j int) bool {
		return healthDraftNameRank(out.Personal[i].Name, query) < healthDraftNameRank(out.Personal[j].Name, query)
	})
	if len(out.Personal) > 5 {
		out.Personal = out.Personal[:5]
	}
	catalog, err := SearchNutritionCatalog(query, "", db)
	if err != nil {
		return HealthDraftCandidates{}, err
	}
	for _, food := range catalog {
		out.Catalog = append(out.Catalog, food)
	}
	sort.SliceStable(out.Catalog, func(i, j int) bool {
		rank := func(food models.NutritionCatalogFood) int {
			best := healthDraftNameRank(food.Name, query)
			for _, alias := range food.Aliases {
				best = min(best, healthDraftNameRank(alias, query))
			}
			return best
		}
		return rank(out.Catalog[i]) < rank(out.Catalog[j])
	})
	if len(out.Catalog) > 5 {
		out.Catalog = out.Catalog[:5]
	}
	return out, nil
}
