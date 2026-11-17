package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"sort"
	"strings"
	"time"

	"ninimenu/internal/models"
)

type recommendationHistorySignal struct {
	Adjustment float64
	Confidence float64
	Positive   float64
	Negative   float64
	Discovery  float64
	Similarity float64
	Novelty    float64
	SourceID   uint
	SourceName string
	Reason     string
}

type recommendationHistoryToken struct {
	Name   string
	Weight float64
}

type recommendationHistoryVector struct {
	ID     uint
	Name   string
	Method string
	Tokens []recommendationHistoryToken
}

type recommendationHistoryModel struct {
	vectors  map[uint]recommendationHistoryVector
	signals  map[uint]recommendationHistorySignal
	features map[uint]recommendationFeatures
	ids      []uint
	digest   string
}

type recommendationHistoryAnchor struct {
	id                            uint
	positive, negative, discovery float64
}

type recommendationHistoryNeighbor struct {
	id           uint
	similarity   float64
	contribution float64
}

// Evidence already has temporal decay applied by BuildTasteProfile. Keeping the
// model request-local avoids another projection that can outlive edited history.
func newRecommendationHistory(dishes []models.Dish, p *TasteProfile) *recommendationHistoryModel {
	m := &recommendationHistoryModel{
		vectors:  map[uint]recommendationHistoryVector{},
		signals:  map[uint]recommendationHistorySignal{},
		features: map[uint]recommendationFeatures{},
	}
	byID := make(map[uint]models.Dish, len(dishes))
	for _, dish := range dishes {
		if dish.ID != 0 {
			byID[dish.ID] = dish
		}
	}
	for id := range byID {
		m.ids = append(m.ids, id)
	}
	sort.Slice(m.ids, func(i, j int) bool { return m.ids[i] < m.ids[j] })
	documentFrequency := map[string]int{}
	for _, id := range m.ids {
		m.features[id] = dishFeatures(byID[id])
		v := historyRecipeVector(byID[id], m.features[id])
		m.vectors[id] = v
		for _, token := range v.Tokens {
			documentFrequency[token.Name]++
		}
	}
	for _, id := range m.ids {
		v := m.vectors[id]
		norm := 0.0
		for i := range v.Tokens {
			token := &v.Tokens[i]
			idf := 1 + math.Log(float64(len(m.ids)+1)/float64(documentFrequency[token.Name]+1))
			token.Weight *= idf
			norm += token.Weight * token.Weight
		}
		if norm > 0 {
			norm = math.Sqrt(norm)
			for i := range v.Tokens {
				v.Tokens[i].Weight /= norm
			}
		}
		m.vectors[id] = v
	}
	anchors := historyAnchors(m.ids, p)
	for _, id := range m.ids {
		m.signals[id] = m.signalFor(id, anchors)
	}
	m.digest = m.buildFingerprint()
	return m
}

func historyRecipeVector(d models.Dish, f recommendationFeatures) recommendationHistoryVector {
	v := recommendationHistoryVector{ID: d.ID, Name: strings.TrimSpace(d.Name), Method: f.Method}
	weights := map[string]float64{}
	for _, ingredient := range f.Ingredients {
		name := canonicalIngredient(ingredient)
		if name == "" || historyGenericIngredient(name) {
			continue
		}
		weights["ingredient:"+name] = 3
		group := ingredientFoodGroup(name)
		switch group {
		case "猪肉", "牛肉", "鸡肉", "羊肉", "鸭肉", "虾", "鱼", "蟹", "贝类", "鸡蛋", "大豆":
			weights["protein:"+group] = 1
		}
	}
	if f.Method != "" {
		weights["method:"+f.Method] = 1.6
	}
	for _, taste := range tasteTokens(d.Taste) {
		if taste = strings.TrimSpace(taste); taste != "" {
			weights["taste:"+taste] = 1
		}
	}
	if category := strings.TrimSpace(d.Category); category != "" {
		weights["category:"+category] = .45
	}
	if f.Role != "" {
		weights["role:"+f.Role] = .4
	}
	names := make([]string, 0, len(weights))
	channelCounts := map[string]int{}
	for name := range weights {
		names = append(names, name)
		channel, _, _ := strings.Cut(name, ":")
		channelCounts[channel]++
	}
	sort.Strings(names)
	for _, name := range names {
		weight := weights[name]
		channel, _, _ := strings.Cut(name, ":")
		// Normalize each multi-value channel before IDF so long ingredient or
		// taste lists do not erase the contribution of cooking methods.
		switch channel {
		case "ingredient", "protein", "taste":
			weight /= math.Sqrt(float64(channelCounts[channel]))
		}
		v.Tokens = append(v.Tokens, recommendationHistoryToken{Name: name, Weight: weight})
	}
	return v
}

func (m *recommendationHistoryModel) featuresFor(d models.Dish) recommendationFeatures {
	if m != nil {
		if features, ok := m.features[d.ID]; ok {
			return features
		}
	}
	return dishFeatures(d)
}

// Condiments occasionally appear in ingredients instead of seasonings. They
// cannot establish a meal relationship or turn soy sauce into a tofu match.
func historyGenericIngredient(name string) bool {
	switch name {
	case "水", "清水", "开水", "热水", "凉水", "冰水", "盐", "食盐", "细盐", "海盐",
		"糖", "白糖", "白砂糖", "冰糖", "红糖", "油", "食用油", "植物油", "色拉油",
		"花生油", "玉米油", "菜籽油", "大豆油", "橄榄油", "芝麻油", "香油", "麻油",
		"酱油", "生抽", "老抽", "味极鲜", "蚝油", "料酒", "黄酒", "白酒",
		"醋", "白醋", "米醋", "陈醋", "香醋", "胡椒粉", "白胡椒粉", "黑胡椒粉",
		"鸡精", "味精", "淀粉", "玉米淀粉", "水淀粉", "生粉", "豆瓣酱", "豆豉",
		"葱", "小葱", "香葱", "葱花", "姜", "生姜", "姜片", "姜丝", "蒜", "大蒜", "蒜末", "蒜瓣":
		return true
	}
	return false
}

func historyAnchors(ids []uint, p *TasteProfile) []recommendationHistoryAnchor {
	if p == nil {
		return nil
	}
	anchors := []recommendationHistoryAnchor{}
	for _, id := range ids {
		a := recommendationHistoryAnchor{id: id}
		trusted := false
		if e := p.dishEvidence[id]; e != nil && e.Weight > 0 && !math.IsNaN(e.Weight) && !math.IsInf(e.Weight, 0) {
			value := boundedScore(e.Sum / e.Weight)
			if !math.IsNaN(value) {
				trusted = value >= 75
				strength := e.Weight / (e.Weight + 3)
				signed := (value - 50) / 50
				a.positive = math.Max(0, signed) * strength
				a.negative = math.Max(0, -signed) * strength
				if p.dislikedSet[id] {
					a.positive = 0
					a.negative = math.Max(a.negative, .5*strength)
				}
			}
		}
		if p.dislikedSet[id] {
			a.positive = 0
		} else if p.favoriteSet[id] && a.negative == 0 {
			a.positive = math.Max(a.positive, .18)
			trusted = true
		}
		if trusted {
			a.discovery = a.positive
		}
		if a.positive > 0 || a.negative > 0 {
			anchors = append(anchors, a)
		}
	}
	return anchors
}

func (m *recommendationHistoryModel) signalFor(id uint, anchors []recommendationHistoryAnchor) recommendationHistorySignal {
	positive, negative, discovery := []recommendationHistoryNeighbor{}, []recommendationHistoryNeighbor{}, []recommendationHistoryNeighbor{}
	signal := recommendationHistorySignal{}
	for _, anchor := range anchors {
		if anchor.id == id {
			continue
		}
		similarity := m.similarity(id, anchor.id)
		if similarity <= 0 {
			continue
		}
		if anchor.positive > 0 {
			positive = append(positive, recommendationHistoryNeighbor{anchor.id, similarity, similarity * similarity * anchor.positive})
			signal.Similarity = math.Max(signal.Similarity, similarity)
		}
		if anchor.negative > 0 {
			negative = append(negative, recommendationHistoryNeighbor{anchor.id, similarity, similarity * similarity * anchor.negative})
		}
		if anchor.discovery > 0 {
			discovery = append(discovery, recommendationHistoryNeighbor{anchor.id, similarity, similarity * similarity * anchor.discovery})
		}
	}
	positiveSupport := historyNeighborSupport(positive)
	negativeSupport := historyNeighborSupport(negative)
	discoverySupport := historyNeighborSupport(discovery)
	signal.Positive = positiveSupport / (.6 + positiveSupport)
	signal.Negative = negativeSupport / (.6 + negativeSupport)
	signal.Discovery = discoverySupport / (.6 + discoverySupport)
	signal.Confidence = (positiveSupport + negativeSupport) / (.6 + positiveSupport + negativeSupport)
	signal.Adjustment = 25 * (signal.Positive - signal.Negative)
	// Near copies offer little discovery; unrelated recipes have no bridge to
	// personal history. Relevance remains separately available in Positive.
	signal.Novelty = 4 * signal.Similarity * (1 - signal.Similarity)
	if len(positive) > 0 {
		best := positive[0]
		signal.SourceID = best.id
		signal.SourceName = m.vectors[best.id].Name
		if signal.Confidence >= .15 && signal.Positive >= .15 && signal.Positive > 1.5*signal.Negative && best.similarity >= .3 {
			signal.Reason = m.sharedHistoryReason(id, best.id)
		}
	}
	return signal
}

func historyNeighborSupport(neighbors []recommendationHistoryNeighbor) float64 {
	sort.Slice(neighbors, func(i, j int) bool {
		if neighbors[i].contribution != neighbors[j].contribution {
			return neighbors[i].contribution > neighbors[j].contribution
		}
		return neighbors[i].id < neighbors[j].id
	})
	total := 0.0
	for i := 0; i < len(neighbors) && i < 4; i++ {
		total += neighbors[i].contribution
	}
	return total
}

func (m *recommendationHistoryModel) similarity(a, b uint) float64 {
	if m == nil {
		return 0
	}
	left, leftOK := m.vectors[a]
	right, rightOK := m.vectors[b]
	if !leftOK || !rightOK {
		return 0
	}
	dot, sharedIngredient := 0.0, false
	for i, j := 0, 0; i < len(left.Tokens) && j < len(right.Tokens); {
		x, y := left.Tokens[i], right.Tokens[j]
		switch {
		case x.Name < y.Name:
			i++
		case x.Name > y.Name:
			j++
		default:
			dot += x.Weight * y.Weight
			if strings.HasPrefix(x.Name, "ingredient:") || strings.HasPrefix(x.Name, "protein:") {
				sharedIngredient = true
			}
			i++
			j++
		}
	}
	if !sharedIngredient {
		return 0
	}
	return math.Max(0, math.Min(1, dot))
}

func (m *recommendationHistoryModel) recentSimilarity(id uint, date time.Time, lastEaten map[uint]string) float64 {
	if m == nil {
		return 0
	}
	recent := 0.0
	day := recommendationDay(date)
	for otherID, lastDate := range lastEaten {
		if otherID == id {
			continue
		}
		last, err := time.ParseInLocation("2006-01-02", lastDate, recommendationLocation)
		if err != nil {
			continue
		}
		age := day.Sub(last).Hours() / 24
		if age < 0 || age > 14 {
			continue
		}
		recent = math.Max(recent, m.similarity(id, otherID)*math.Pow(.5, age/7))
	}
	return recent
}

func (m *recommendationHistoryModel) sharedHistoryReason(id, sourceID uint) string {
	candidate, source := m.vectors[id], m.vectors[sourceID]
	if source.Name == "" {
		return ""
	}
	shared, bestWeight := "", 0.0
	for _, prefix := range []string{"ingredient:", "protein:"} {
		for i, j := 0, 0; i < len(candidate.Tokens) && j < len(source.Tokens); {
			a, b := candidate.Tokens[i], source.Tokens[j]
			switch {
			case a.Name < b.Name:
				i++
			case a.Name > b.Name:
				j++
			default:
				if weight := a.Weight * b.Weight; strings.HasPrefix(a.Name, prefix) && weight > bestWeight {
					shared, bestWeight = strings.TrimPrefix(a.Name, prefix), weight
				}
				i++
				j++
			}
		}
		if shared != "" {
			break
		}
	}
	if shared == "" {
		return ""
	}
	reason := "和「" + source.Name + "」一样用到" + shared
	if candidate.Method != "" && candidate.Method == source.Method {
		reason += "，同样采用" + candidate.Method + "做法"
	}
	return reason
}

func (m *recommendationHistoryModel) fingerprint() string {
	if m == nil {
		return ""
	}
	return m.digest
}

func (m *recommendationHistoryModel) buildFingerprint() string {
	type entry struct {
		Vector recommendationHistoryVector
		Signal recommendationHistorySignal
	}
	entries := make([]entry, 0, len(m.ids))
	for _, id := range m.ids {
		entries = append(entries, entry{m.vectors[id], m.signals[id]})
	}
	raw, _ := json.Marshal(entries)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
