// Package engine 是懂我引擎（ADR-0022）的纯函数核心：四因子评分、全零逐层
// 放宽、口味画像相似度与理由管道。无数据库、无随机数——输入快照、输出权重
// 与理由，事务与抽样留在 internal/meal。
package engine

// Tier 是 Taste rating 上三档（入池档位）：3 人上人、4 顶尖、5 夯。
const (
	TierRenShangRen = 3
	TierDingJian    = 4
	TierHang        = 5

	// DemotionSwapThreshold 是自动降档的连换阈值：pool 模式连续被换 4 次
	// （其间零接受，接受即清零计数）降一档，地板人上人。
	DemotionSwapThreshold = 4
)

// ValidTier 是入池档位的合法域（拒绝档 1/2 只存在于评分侧）。
func ValidTier(tier int) bool {
	return tier >= TierRenShangRen && tier <= TierHang
}

// TierMultiplier 是喜爱因子：夯:人上人 = 4:1。
func TierMultiplier(tier int) float64 {
	switch tier {
	case TierHang:
		return 2.0
	case TierRenShangRen:
		return 0.5
	default:
		return 1.0
	}
}

// OccasionClass 按 Catalog 品类桶划分的时段类别。
type OccasionClass int

const (
	// OccasionAny 全天可揭示。
	OccasionAny OccasionClass = iota
	// OccasionMorning 早餐类：早晨满值，其余时段重罚。
	OccasionMorning
	// OccasionNever 酱料/饮品/甜品/半成品：永不直接揭示，放宽也不复活。
	OccasionNever
)

// ClassifyOccasion 把 source_path 首段品类桶映射到时段类别。
func ClassifyOccasion(categoryBucket string) OccasionClass {
	switch categoryBucket {
	case "breakfast":
		return OccasionMorning
	case "condiment", "drink", "dessert", "semi-finished":
		return OccasionNever
	default:
		return OccasionAny
	}
}

const (
	morningStartHour   = 5
	morningEndHour     = 10
	offOccasionPenalty = 0.15
	reunionDistance    = 12
	reunionBonus       = 1.2
)

// OccasionFactor 是场合因子。hour 为客户端上报的本机小时（0–23）。
func OccasionFactor(class OccasionClass, hour int) float64 {
	switch class {
	case OccasionNever:
		return 0
	case OccasionMorning:
		if hour >= morningStartHour && hour <= morningEndHour {
			return 1
		}
		return offOccasionPenalty
	default:
		return 1
	}
}

// FreshnessFactor 是新鲜感因子。distance 为该菜上次被接受之后账号新增的
// 吃饭记录数；从未吃过按中性 1.0。
func FreshnessFactor(distance int, everEaten bool) float64 {
	if !everEaten {
		return 1.0
	}
	switch {
	case distance <= 1:
		return 0
	case distance == 2:
		return 0.35
	case distance == 3:
		return 0.6
	case distance == 4:
		return 0.8
	case distance == 5:
		return 0.9
	case distance >= reunionDistance:
		return reunionBonus
	default:
		return 1.0
	}
}

// Candidate 是一次揭示时池中一道菜的评分快照。
type Candidate struct {
	ID            string
	Name          string
	Tier          int
	Occasion      OccasionClass
	Distance      int
	EverEaten     bool
	ShownThisMeal bool
}

// Relaxation 是全零逐层放宽的档位，按序解除单个因子。
type Relaxation int

const (
	RelaxNone Relaxation = iota
	RelaxFreshness
	RelaxOccasion
	RelaxShown
)

// Weighted 是评分产物：进入加权抽样的权重与还原理由所需的因子快照。
type Weighted struct {
	Candidate Candidate
	Weight    float64
}

// score 按放宽档位计算单菜权重。放宽只解除对应因子的惩罚（置 1），
// OccasionNever 恒零不复活。
func score(candidate Candidate, hour int, relaxation Relaxation) float64 {
	if candidate.Occasion == OccasionNever {
		return 0
	}
	weight := TierMultiplier(candidate.Tier)
	if relaxation < RelaxFreshness {
		weight *= FreshnessFactor(candidate.Distance, candidate.EverEaten)
	}
	if relaxation < RelaxOccasion {
		weight *= OccasionFactor(candidate.Occasion, hour)
	}
	if relaxation < RelaxShown && candidate.ShownThisMeal {
		weight = 0
	}
	return weight
}

// ScorePool 对池做四因子评分；全零时按 新鲜感→场合→本顿否决 逐层放宽。
// 返回空切片表示池里只剩永零类（视同空池）。
func ScorePool(candidates []Candidate, hour int) ([]Weighted, Relaxation) {
	for relaxation := RelaxNone; relaxation <= RelaxShown; relaxation++ {
		weighted := make([]Weighted, 0, len(candidates))
		for _, candidate := range candidates {
			if weight := score(candidate, hour, relaxation); weight > 0 {
				weighted = append(weighted, Weighted{Candidate: candidate, Weight: weight})
			}
		}
		if len(weighted) > 0 {
			return weighted, relaxation
		}
	}
	return nil, RelaxShown
}

// DiscoveryProbability 把探索压力信号数折算为本次揭示走 Discovery 的概率。
func DiscoveryProbability(signals int) float64 {
	if signals <= 0 {
		return 0
	}
	return min(0.75, float64(signals)*0.25)
}
