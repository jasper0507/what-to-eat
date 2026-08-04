package engine

// DiscoveryReference 是探索臂的参照菜：池内爱菜的画像与喜爱档。
type DiscoveryReference struct {
	ID      string
	Name    string
	Tier    int
	Profile Profile
}

// DiscoveryCandidate 是池外候选的纯评分快照（无 SQL）。
type DiscoveryCandidate struct {
	ID        string
	Name      string
	Profile   Profile
	Occasion  OccasionClass
	Distance  int
	EverEaten bool
}

// DiscoveryWeighted 是探索加权结果，附带理由素材。
type DiscoveryWeighted struct {
	ID            string
	Name          string
	Weight        float64
	ReferenceName string
	Hits          SimilarityHits
}

// ScoreDiscovery 计算探索候选权重：
// max(相似度 × 参照喜爱档) × 候选场合 × 候选新鲜感。
// 场合或新鲜感为 0 的候选不入选；返回按输入顺序、权重 > 0 的子集。
func ScoreDiscovery(
	candidates []DiscoveryCandidate,
	references []DiscoveryReference,
	hour int,
) []DiscoveryWeighted {
	if len(references) == 0 {
		return nil
	}
	out := make([]DiscoveryWeighted, 0)
	for _, candidate := range candidates {
		occasion := OccasionFactor(candidate.Occasion, hour)
		if occasion == 0 {
			continue
		}
		freshness := FreshnessFactor(candidate.Distance, candidate.EverEaten)
		if freshness == 0 {
			continue
		}
		best := DiscoveryWeighted{ID: candidate.ID, Name: candidate.Name}
		for _, reference := range references {
			similarity, hits := Similarity(candidate.Profile, reference.Profile)
			weighted := similarity * TierMultiplier(reference.Tier)
			if weighted > best.Weight {
				best.Weight = weighted
				best.ReferenceName = reference.Name
				best.Hits = hits
			}
		}
		if best.Weight <= 0 {
			continue
		}
		best.Weight *= occasion * freshness
		out = append(out, best)
	}
	return out
}
