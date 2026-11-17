package services

import (
	"math"

	"ninimenu/internal/models"
)

func nearestSelectedSimilarity(model *recommendationHistoryModel, id uint, selected []models.Dish) float64 {
	nearest := 0.0
	for _, dish := range selected {
		nearest = math.Max(nearest, model.similarity(id, dish.ID))
	}
	return nearest
}

// historyDiscovery trades a small amount of relevance for a meaningful change
// from liked recipes. It never widens the already-filtered candidate pool.
func historyDiscovery(scored []RecommendItem, selected []models.Dish, p *TasteProfile) (RecommendItem, bool) {
	if p.historyModel == nil || len(scored) == 0 {
		return RecommendItem{}, false
	}
	var best RecommendItem
	bestValue := math.Inf(-1)
	for _, item := range scored {
		id := item.Dish.ID
		signal := p.historyModel.signals[id]
		if p.eatenCount[id] > 0 || p.favoriteSet[id] || p.dislikedSet[id] ||
			item.Score < scored[0].Score-6 || signal.Discovery < .08 ||
			signal.Negative >= signal.Positive*.5 || signal.Novelty < .12 || signal.Similarity >= .92 {
			continue
		}
		nearest := nearestSelectedSimilarity(p.historyModel, id, selected)
		if nearest >= .85 {
			continue
		}
		value := item.Score + 6*signal.Novelty*signal.Discovery - 8*nearest
		if value > bestValue {
			best, bestValue = item, value
		}
	}
	if math.IsInf(bestValue, -1) {
		return RecommendItem{}, false
	}
	best.Reasons = append([]string{"和喜欢的菜有相似食材，也有新变化"}, best.Reasons...)
	if len(best.Reasons) > 2 {
		best.Reasons = best.Reasons[:2]
	}
	return best, true
}
