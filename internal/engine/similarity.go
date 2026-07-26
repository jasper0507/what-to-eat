package engine

// Profile 是一道菜的 Taste profile：导入期过词典确定性萃取（ADR-0022）。
type Profile struct {
	Ingredients []string
	Flavors     []string
	Techniques  []string
	Category    string
}

// SimilarityHits 记录相似度的命中维度，供探索理由行取材。
type SimilarityHits struct {
	Ingredients []string
	Flavors     []string
	Technique   bool
	Category    bool
}

// Similarity = 5×|主料交| + 4×|味型交| + 2×[工艺交非空] + 1×[品类同]。
func Similarity(candidate, reference Profile) (float64, SimilarityHits) {
	var hits SimilarityHits
	hits.Ingredients = intersect(candidate.Ingredients, reference.Ingredients)
	hits.Flavors = intersect(candidate.Flavors, reference.Flavors)
	hits.Technique = len(intersect(candidate.Techniques, reference.Techniques)) > 0
	hits.Category = candidate.Category != "" && candidate.Category == reference.Category

	score := 5*float64(len(hits.Ingredients)) + 4*float64(len(hits.Flavors))
	if hits.Technique {
		score += 2
	}
	if hits.Category {
		score += 1
	}
	return score, hits
}

func intersect(left, right []string) []string {
	inRight := make(map[string]bool, len(right))
	for _, item := range right {
		inRight[item] = true
	}
	shared := make([]string, 0)
	for _, item := range left {
		if inRight[item] {
			shared = append(shared, item)
			inRight[item] = false
		}
	}
	return shared
}
