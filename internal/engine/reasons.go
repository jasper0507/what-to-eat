package engine

import "fmt"

// 理由管道（ADR-0022）：Decision.reason 每次揭示必有。组句优先级：
// 放宽说明 > 探索命中维度 > 久别重逢 > 场合加成 > 喜爱档位兜底。
// 全中文、短句、零系统腔。

// DiscoveryReason 是探索揭示的理由素材：参照菜与相似度命中维度。
type DiscoveryReason struct {
	ReferenceName string
	Hits          SimilarityHits
}

// ReasonInput 汇集组句所需的因子快照。
type ReasonInput struct {
	Relaxation Relaxation
	Tier       int
	Distance   int
	EverEaten  bool
	Occasion   OccasionClass
	Hour       int
	Discovery  *DiscoveryReason
}

func tierLabel(tier int) string {
	switch tier {
	case TierHang:
		return "夯"
	case TierDingJian:
		return "顶尖"
	default:
		return "人上人"
	}
}

// ComposeReason 组一句人话理由。
func ComposeReason(input ReasonInput) string {
	if input.Discovery != nil {
		return discoveryReason(*input.Discovery)
	}
	switch input.Relaxation {
	case RelaxFreshness:
		return "都在冷却，破例放行。"
	case RelaxOccasion:
		return "这个点没有正对味的，放宽了时段。"
	case RelaxShown:
		return "这顿都轮过一遍了，再给你一次。"
	}
	if input.EverEaten && input.Distance >= reunionDistance {
		return "有阵子没吃它了。"
	}
	if input.Occasion == OccasionMorning &&
		input.Hour >= morningStartHour && input.Hour <= morningEndHour {
		return "早上就该来这口。"
	}
	return "你的" + tierLabel(input.Tier) + "。"
}

func discoveryReason(discovery DiscoveryReason) string {
	reference := discovery.ReferenceName
	hits := discovery.Hits
	switch {
	case len(hits.Ingredients) > 0:
		return fmt.Sprintf(
			"跟你爱的%s一样有%s，试试新的。",
			reference,
			hits.Ingredients[0],
		)
	case len(hits.Flavors) > 0:
		return fmt.Sprintf(
			"跟你爱的%s一个路子，都是%s口，试试新的。",
			reference,
			hits.Flavors[0],
		)
	case hits.Technique || hits.Category:
		return fmt.Sprintf("做法像你爱的%s，试试新的。", reference)
	default:
		return fmt.Sprintf("像你爱的%s，试试新的。", reference)
	}
}
