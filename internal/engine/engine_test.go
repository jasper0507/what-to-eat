package engine

import (
	"math"
	"strings"
	"testing"
)

func TestFreshnessFactorCurve(t *testing.T) {
	cases := []struct {
		distance  int
		everEaten bool
		want      float64
	}{
		{0, true, 0},
		{1, true, 0},
		{2, true, 0.35},
		{3, true, 0.6},
		{4, true, 0.8},
		{5, true, 0.9},
		{6, true, 1.0},
		{11, true, 1.0},
		{12, true, 1.2},
		{40, true, 1.2},
		{0, false, 1.0},
	}
	for _, testCase := range cases {
		got := FreshnessFactor(testCase.distance, testCase.everEaten)
		if math.Abs(got-testCase.want) > 1e-9 {
			t.Errorf(
				"FreshnessFactor(%d, %v) = %v, want %v",
				testCase.distance, testCase.everEaten, got, testCase.want,
			)
		}
	}
}

func TestOccasionFactor(t *testing.T) {
	cases := []struct {
		class OccasionClass
		hour  int
		want  float64
	}{
		{OccasionMorning, 5, 1},
		{OccasionMorning, 10, 1},
		{OccasionMorning, 4, 0.15},
		{OccasionMorning, 11, 0.15},
		{OccasionMorning, 21, 0.15},
		{OccasionNever, 8, 0},
		{OccasionNever, 20, 0},
		{OccasionAny, 8, 1},
		{OccasionAny, 23, 1},
	}
	for _, testCase := range cases {
		if got := OccasionFactor(testCase.class, testCase.hour); got != testCase.want {
			t.Errorf(
				"OccasionFactor(%v, %d) = %v, want %v",
				testCase.class, testCase.hour, got, testCase.want,
			)
		}
	}
}

func TestClassifyOccasion(t *testing.T) {
	cases := map[string]OccasionClass{
		"breakfast":      OccasionMorning,
		"condiment":      OccasionNever,
		"drink":          OccasionNever,
		"dessert":        OccasionNever,
		"semi-finished":  OccasionNever,
		"meat_dish":      OccasionAny,
		"aquatic":        OccasionAny,
		"soup":           OccasionAny,
		"staple":         OccasionAny,
		"vegetable_dish": OccasionAny,
		"":               OccasionAny,
	}
	for bucket, want := range cases {
		if got := ClassifyOccasion(bucket); got != want {
			t.Errorf("ClassifyOccasion(%q) = %v, want %v", bucket, got, want)
		}
	}
}

func TestTierMultiplierRatio(t *testing.T) {
	if TierMultiplier(TierHang) != 2.0 ||
		TierMultiplier(TierDingJian) != 1.0 ||
		TierMultiplier(TierRenShangRen) != 0.5 {
		t.Fatalf("tier multipliers out of spec")
	}
	if TierMultiplier(TierHang)/TierMultiplier(TierRenShangRen) != 4 {
		t.Fatalf("夯:人上人 must be 4:1")
	}
}

func TestScorePoolFourFactors(t *testing.T) {
	candidates := []Candidate{
		{ID: "a", Tier: TierHang, Occasion: OccasionAny, Distance: 6, EverEaten: true},
		{ID: "b", Tier: TierRenShangRen, Occasion: OccasionAny, EverEaten: false},
		{ID: "c", Tier: TierDingJian, Occasion: OccasionAny, Distance: 1, EverEaten: true},
		{ID: "d", Tier: TierDingJian, Occasion: OccasionAny, EverEaten: false, ShownThisMeal: true},
		{ID: "e", Tier: TierDingJian, Occasion: OccasionNever, EverEaten: false},
	}
	weighted, relaxation := ScorePool(candidates, 12)
	if relaxation != RelaxNone {
		t.Fatalf("relaxation = %v, want RelaxNone", relaxation)
	}
	weights := map[string]float64{}
	for _, entry := range weighted {
		weights[entry.Candidate.ID] = entry.Weight
	}
	if len(weights) != 2 {
		t.Fatalf("selectable = %v, want a and b only", weights)
	}
	if weights["a"] != 2.0 {
		t.Errorf("a: 夯×新鲜 1.0 应得 2.0，got %v", weights["a"])
	}
	if weights["b"] != 0.5 {
		t.Errorf("b: 人上人×从未吃过 应得 0.5，got %v", weights["b"])
	}
}

func TestScorePoolRelaxesFreshnessFirst(t *testing.T) {
	candidates := []Candidate{
		{ID: "a", Tier: TierHang, Occasion: OccasionAny, Distance: 0, EverEaten: true},
		{ID: "b", Tier: TierDingJian, Occasion: OccasionAny, Distance: 1, EverEaten: true},
	}
	weighted, relaxation := ScorePool(candidates, 12)
	if relaxation != RelaxFreshness {
		t.Fatalf("relaxation = %v, want RelaxFreshness", relaxation)
	}
	if len(weighted) != 2 {
		t.Fatalf("放宽新鲜感后两道都应可选，got %d", len(weighted))
	}
}

func TestScorePoolRelaxesOccasionSecond(t *testing.T) {
	// 全部为冷却中的早餐，夜里揭示：先放新鲜感（0.15 存活），不必放场合。
	candidates := []Candidate{
		{ID: "a", Tier: TierDingJian, Occasion: OccasionMorning, Distance: 0, EverEaten: true},
	}
	weighted, relaxation := ScorePool(candidates, 22)
	if relaxation != RelaxFreshness {
		t.Fatalf("relaxation = %v, want RelaxFreshness", relaxation)
	}
	if math.Abs(weighted[0].Weight-0.15) > 1e-9 {
		t.Fatalf("weight = %v, want 0.15（场合罚仍生效）", weighted[0].Weight)
	}
}

func TestScorePoolRelaxesShownLast(t *testing.T) {
	candidates := []Candidate{
		{ID: "a", Tier: TierDingJian, Occasion: OccasionAny, EverEaten: false, ShownThisMeal: true},
	}
	weighted, relaxation := ScorePool(candidates, 12)
	if relaxation != RelaxShown {
		t.Fatalf("relaxation = %v, want RelaxShown", relaxation)
	}
	if len(weighted) != 1 {
		t.Fatalf("放宽本顿否决后应可选，got %d", len(weighted))
	}
}

func TestScorePoolNeverClassStaysDead(t *testing.T) {
	candidates := []Candidate{
		{ID: "a", Tier: TierHang, Occasion: OccasionNever, EverEaten: false},
	}
	weighted, _ := ScorePool(candidates, 12)
	if len(weighted) != 0 {
		t.Fatalf("永零类放宽到底也不复活，got %v", weighted)
	}
}

func TestDiscoveryProbability(t *testing.T) {
	cases := map[int]float64{0: 0, 1: 0.25, 2: 0.5, 3: 0.75, 4: 0.75}
	for signals, want := range cases {
		if got := DiscoveryProbability(signals); got != want {
			t.Errorf("DiscoveryProbability(%d) = %v, want %v", signals, got, want)
		}
	}
}

func TestSimilarityFormula(t *testing.T) {
	candidate := Profile{
		Ingredients: []string{"鸡肉", "花生"},
		Flavors:     []string{"麻辣"},
		Techniques:  []string{"凉拌"},
		Category:    "meat_dish",
	}
	reference := Profile{
		Ingredients: []string{"鸡肉"},
		Flavors:     []string{"麻辣", "酸甜"},
		Techniques:  []string{"凉拌"},
		Category:    "meat_dish",
	}
	score, hits := Similarity(candidate, reference)
	// 5×1 主料 + 4×1 味型 + 2 工艺 + 1 品类 = 12
	if score != 12 {
		t.Fatalf("score = %v, want 12", score)
	}
	if len(hits.Ingredients) != 1 || hits.Ingredients[0] != "鸡肉" {
		t.Errorf("ingredient hits = %v", hits.Ingredients)
	}
	if !hits.Technique || !hits.Category {
		t.Errorf("technique/category hits missing")
	}
}

func TestSimilarityNoOverlap(t *testing.T) {
	score, _ := Similarity(
		Profile{Ingredients: []string{"牛肉"}, Category: "meat_dish"},
		Profile{Ingredients: []string{"豆腐"}, Category: "vegetable_dish"},
	)
	if score != 0 {
		t.Fatalf("score = %v, want 0", score)
	}
}

func TestComposeReasonPriority(t *testing.T) {
	// 放宽压过一切
	reason := ComposeReason(ReasonInput{
		Relaxation: RelaxFreshness,
		Tier:       TierHang,
		Distance:   20,
		EverEaten:  true,
	})
	if reason != "都在冷却，破例放行。" {
		t.Errorf("relaxation reason = %q", reason)
	}
	// 探索压过久别重逢
	reason = ComposeReason(ReasonInput{
		Discovery: &DiscoveryReason{
			ReferenceName: "口水鸡",
			Hits:          SimilarityHits{Ingredients: []string{"鸡肉"}},
		},
		Distance:  20,
		EverEaten: true,
	})
	if !strings.Contains(reason, "口水鸡") || !strings.Contains(reason, "鸡肉") {
		t.Errorf("discovery reason = %q", reason)
	}
	// 久别重逢
	reason = ComposeReason(ReasonInput{
		Tier: TierDingJian, Distance: 12, EverEaten: true,
	})
	if reason != "有阵子没吃它了。" {
		t.Errorf("reunion reason = %q", reason)
	}
	// 早晨的早餐
	reason = ComposeReason(ReasonInput{
		Tier: TierDingJian, Occasion: OccasionMorning, Hour: 7, EverEaten: false,
	})
	if reason != "早上就该来这口。" {
		t.Errorf("morning reason = %q", reason)
	}
	// 档位兜底，永不为空
	reason = ComposeReason(ReasonInput{Tier: TierHang, Hour: 12})
	if reason != "你的夯。" {
		t.Errorf("tier fallback reason = %q", reason)
	}
}

func TestComposeReasonNeverEmpty(t *testing.T) {
	for tier := TierRenShangRen; tier <= TierHang; tier++ {
		if ComposeReason(ReasonInput{Tier: tier, Hour: 15}) == "" {
			t.Fatalf("reason must never be empty (tier %d)", tier)
		}
	}
}

func TestScoreDiscoveryWeightFormula(t *testing.T) {
	// 相似度：5×主料 + 1×品类 = 6；× 夯 2.0 × 场合 1 × 新鲜感 1 = 12
	refs := []DiscoveryReference{{
		ID:   "pool/ref",
		Name: "口水鸡",
		Tier: TierHang,
		Profile: Profile{
			Ingredients: []string{"鸡肉"},
			Category:    "meat_dish",
		},
	}}
	candidates := []DiscoveryCandidate{{
		ID:       "out/a",
		Name:     "白切鸡",
		Profile:  Profile{Ingredients: []string{"鸡肉"}, Category: "meat_dish"},
		Occasion: OccasionAny,
	}}
	got := ScoreDiscovery(candidates, refs, 12)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	// 5×1 主料 + 1 品类 = 6；× 夯 2.0 = 12
	if math.Abs(got[0].Weight-12) > 1e-9 {
		t.Errorf("weight = %v, want 12", got[0].Weight)
	}
	if got[0].ReferenceName != "口水鸡" {
		t.Errorf("reference = %q", got[0].ReferenceName)
	}
}

func TestScoreDiscoveryDropsZeroOccasionAndFreshness(t *testing.T) {
	refs := []DiscoveryReference{{
		ID: "r", Name: "ref", Tier: TierDingJian,
		Profile: Profile{Ingredients: []string{"豆腐"}, Category: "vegetable_dish"},
	}}
	candidates := []DiscoveryCandidate{
		{
			ID: "never", Name: "柠檬水",
			Profile:  Profile{Ingredients: []string{"豆腐"}, Category: "drink"},
			Occasion: OccasionNever,
		},
		{
			ID: "cooldown", Name: "刚吃过",
			Profile:   Profile{Ingredients: []string{"豆腐"}, Category: "vegetable_dish"},
			Occasion:  OccasionAny,
			Distance:  1,
			EverEaten: true,
		},
		{
			ID: "ok", Name: "可以",
			Profile:  Profile{Ingredients: []string{"豆腐"}, Category: "vegetable_dish"},
			Occasion: OccasionAny,
		},
	}
	got := ScoreDiscovery(candidates, refs, 12)
	if len(got) != 1 || got[0].ID != "ok" {
		t.Fatalf("got %#v, want only ok", got)
	}
}

func TestScoreDiscoveryPicksBestReferenceByLove(t *testing.T) {
	// 同一相似度下，夯参照应压过顶尖参照。
	refs := []DiscoveryReference{
		{
			ID: "low", Name: "人上人参照", Tier: TierRenShangRen,
			Profile: Profile{Ingredients: []string{"鸡蛋"}, Category: "vegetable_dish"},
		},
		{
			ID: "high", Name: "夯参照", Tier: TierHang,
			Profile: Profile{Ingredients: []string{"鸡蛋"}, Category: "vegetable_dish"},
		},
	}
	candidates := []DiscoveryCandidate{{
		ID: "c", Name: "番茄炒蛋",
		Profile:  Profile{Ingredients: []string{"鸡蛋"}, Category: "vegetable_dish"},
		Occasion: OccasionAny,
	}}
	got := ScoreDiscovery(candidates, refs, 12)
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].ReferenceName != "夯参照" {
		t.Errorf("picked %q, want 夯参照", got[0].ReferenceName)
	}
	// 相似度 5+1=6 × 夯 2.0 = 12（人上人仅 3）
	if math.Abs(got[0].Weight-12) > 1e-9 {
		t.Errorf("weight = %v, want 12", got[0].Weight)
	}
}

func TestScoreDiscoveryEmptyReferences(t *testing.T) {
	got := ScoreDiscovery([]DiscoveryCandidate{{
		ID: "c", Name: "x", Profile: Profile{Ingredients: []string{"a"}}, Occasion: OccasionAny,
	}}, nil, 12)
	if got != nil {
		t.Fatalf("want nil, got %#v", got)
	}
}
