package meal

import (
	"context"
	"database/sql"
	"errors"
	"slices"

	"github.com/jasper0507/what-to-eat/internal/catalog"
	"github.com/jasper0507/what-to-eat/internal/engine"
)

// poolSnapshot 是一次揭示的评分输入：池成员的引擎候选快照与新鲜感账本。
type poolSnapshot struct {
	candidates []engine.Candidate
	tiers      map[string]int
	// freshnessEligible 是新鲜感 > 0 的池菜数（探索压力信号之一）。
	freshnessEligible int
	// lastAccepted / totalRecords 供 Discovery 复算任意菜的新鲜感。
	lastAccepted map[string]int64
	totalRecords int64
}

func (s poolSnapshot) freshnessOf(dishID string) float64 {
	lastSequence, everEaten := s.lastAccepted[dishID]
	return engine.FreshnessFactor(int(s.totalRecords-lastSequence), everEaten)
}

func (m *Lifecycle) poolCandidates(
	context context.Context,
	transaction *sql.Tx,
	accountID int64,
	shown map[string]int,
) (poolSnapshot, error) {
	snapshot := poolSnapshot{
		tiers:        make(map[string]int),
		lastAccepted: make(map[string]int64),
	}

	if err := transaction.QueryRowContext(
		context,
		"SELECT COALESCE(MAX(sequence), 0) FROM eating_records WHERE account_id = ?",
		accountID,
	).Scan(&snapshot.totalRecords); err != nil {
		return poolSnapshot{}, err
	}
	historyRows, err := transaction.QueryContext(
		context,
		`SELECT dish_id, MAX(sequence)
		 FROM eating_records
		 WHERE account_id = ?
		 GROUP BY dish_id`,
		accountID,
	)
	if err != nil {
		return poolSnapshot{}, err
	}
	for historyRows.Next() {
		var dishID string
		var lastSequence int64
		if err := historyRows.Scan(&dishID, &lastSequence); err != nil {
			historyRows.Close()
			return poolSnapshot{}, err
		}
		snapshot.lastAccepted[dishID] = lastSequence
	}
	if err := errors.Join(historyRows.Close(), historyRows.Err()); err != nil {
		return poolSnapshot{}, err
	}

	rows, err := transaction.QueryContext(
		context,
		`SELECT catalog_dishes.source_path, catalog_dishes.name, candidate_pool.tier
		 FROM candidate_pool
		 JOIN catalog_dishes ON catalog_dishes.source_path = candidate_pool.dish_id
		 WHERE candidate_pool.account_id = ?
		   AND NOT EXISTS (
			SELECT 1
			FROM rejection_marks
			WHERE rejection_marks.account_id = candidate_pool.account_id
			  AND rejection_marks.dish_id = candidate_pool.dish_id
		   )
		 ORDER BY catalog_dishes.source_path`,
		accountID,
	)
	if err != nil {
		return poolSnapshot{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var candidate engine.Candidate
		if err := rows.Scan(&candidate.ID, &candidate.Name, &candidate.Tier); err != nil {
			return poolSnapshot{}, err
		}
		lastSequence, everEaten := snapshot.lastAccepted[candidate.ID]
		candidate.EverEaten = everEaten
		candidate.Distance = int(snapshot.totalRecords - lastSequence)
		candidate.Occasion = engine.ClassifyOccasion(catalog.PathCategory(candidate.ID))
		candidate.ShownThisMeal = shown[candidate.ID] > 0
		snapshot.tiers[candidate.ID] = candidate.Tier
		snapshot.candidates = append(snapshot.candidates, candidate)
		if engine.FreshnessFactor(candidate.Distance, candidate.EverEaten) > 0 {
			snapshot.freshnessEligible++
		}
	}
	return snapshot, rows.Err()
}

// recentRerolls 统计探索压力的 Reroll 信号：窗口为最近 N 个已了结
// （接受或放弃）的 Meal——放弃也是响亮的「没懂我」（ADR-0022 修正案）。
func (m *Lifecycle) recentRerolls(
	context context.Context,
	transaction *sql.Tx,
	accountID int64,
) (int, error) {
	if !m.discovery.Enabled || m.discovery.RecentMealWindow <= 0 {
		return 0, nil
	}
	var rerolls int
	err := transaction.QueryRowContext(
		context,
		`SELECT COUNT(*)
		 FROM decisions
		 WHERE rerolled_to_id IS NOT NULL AND meal_id IN (
			SELECT id
			FROM meals
			WHERE account_id = ? AND status IN ('accepted', 'abandoned')
			ORDER BY id DESC
			LIMIT ?
		 )`,
		accountID,
		m.discovery.RecentMealWindow,
	).Scan(&rerolls)
	return rerolls, err
}

// mealShownCounts 统计本 Meal 内已展示的 Dish（本顿否决的输入）与 Discovery 数。
func mealShownCounts(
	context context.Context,
	transaction *sql.Tx,
	mealID int64,
) (shown map[string]int, decisionCount, discoveryCount int, err error) {
	rows, err := transaction.QueryContext(
		context,
		"SELECT dish_id, mode FROM decisions WHERE meal_id = ?",
		mealID,
	)
	if err != nil {
		return nil, 0, 0, err
	}
	shown = make(map[string]int)
	for rows.Next() {
		var dishID, mode string
		if err := rows.Scan(&dishID, &mode); err != nil {
			rows.Close()
			return nil, 0, 0, err
		}
		shown[dishID]++
		decisionCount++
		if mode == string(ModeDiscovery) {
			discoveryCount++
		}
	}
	if err := rows.Close(); err != nil {
		return nil, 0, 0, err
	}
	return shown, decisionCount, discoveryCount, rows.Err()
}

// discoverySignals 数探索压力信号：池小 / 新鲜感可选少 / 近窗 Reroll 多。
func (m *Lifecycle) discoverySignals(snapshot poolSnapshot, rerolls int) int {
	if !m.discovery.Enabled {
		return 0
	}
	signals := 0
	if len(snapshot.candidates) <= m.discovery.MaxPoolSize {
		signals++
	}
	if snapshot.freshnessEligible <= m.discovery.MaxEligibleDishes {
		signals++
	}
	if rerolls >= m.discovery.MinRerolls {
		signals++
	}
	return signals
}

type decisionChoice struct {
	dishID string
	name   string
	mode   Mode
	reason string
}

// discoveryDish 从池外候选装配纯快照，经 engine.ScoreDiscovery 加权后抽样。
func (m *Lifecycle) discoveryDish(
	context context.Context,
	transaction *sql.Tx,
	accountID int64,
	snapshot poolSnapshot,
	shown map[string]int,
	hour int,
) (decisionChoice, bool, error) {
	profiles, names, err := catalog.Profiles(context, transaction)
	if err != nil {
		return decisionChoice{}, false, err
	}

	rejectedRows, err := transaction.QueryContext(
		context,
		"SELECT dish_id FROM rejection_marks WHERE account_id = ?",
		accountID,
	)
	if err != nil {
		return decisionChoice{}, false, err
	}
	rejected := make(map[string]bool)
	for rejectedRows.Next() {
		var dishID string
		if err := rejectedRows.Scan(&dishID); err != nil {
			rejectedRows.Close()
			return decisionChoice{}, false, err
		}
		rejected[dishID] = true
	}
	if err := errors.Join(rejectedRows.Close(), rejectedRows.Err()); err != nil {
		return decisionChoice{}, false, err
	}

	references := make([]engine.DiscoveryReference, 0, len(snapshot.candidates))
	for _, member := range snapshot.candidates {
		profile, ok := profiles[member.ID]
		if !ok {
			continue
		}
		references = append(references, engine.DiscoveryReference{
			ID:      member.ID,
			Name:    member.Name,
			Tier:    member.Tier,
			Profile: profile,
		})
	}

	// 候选序按 dishID 排定：种子注入承诺同种子同行为（测试确定性），
	// 抽样序不能来自 map 迭代。
	dishIDs := make([]string, 0, len(profiles))
	for dishID := range profiles {
		dishIDs = append(dishIDs, dishID)
	}
	slices.Sort(dishIDs)
	candidates := make([]engine.DiscoveryCandidate, 0)
	for _, dishID := range dishIDs {
		if _, inPool := snapshot.tiers[dishID]; inPool || rejected[dishID] || shown[dishID] > 0 {
			continue
		}
		lastSequence, everEaten := snapshot.lastAccepted[dishID]
		candidates = append(candidates, engine.DiscoveryCandidate{
			ID:        dishID,
			Name:      names[dishID],
			Profile:   profiles[dishID],
			Occasion:  engine.ClassifyOccasion(catalog.PathCategory(dishID)),
			Distance:  int(snapshot.totalRecords - lastSequence),
			EverEaten: everEaten,
		})
	}

	weighted := engine.ScoreDiscovery(candidates, references, hour)
	if len(weighted) == 0 {
		return decisionChoice{}, false, nil
	}

	selected := weightedPick(
		m.randomFloat(),
		weighted,
		func(entry engine.DiscoveryWeighted) float64 { return entry.Weight },
	)
	reason := engine.ComposeReason(engine.ReasonInput{
		Discovery: &engine.DiscoveryReason{
			ReferenceName: selected.ReferenceName,
			Hits:          selected.Hits,
		},
	})
	return decisionChoice{
		dishID: selected.ID,
		name:   selected.Name,
		mode:   ModeDiscovery,
		reason: reason,
	}, true, nil
}

type selectionInput struct {
	snapshot       poolSnapshot
	shown          map[string]int
	excludeDishID  string
	rerolls        int
	discoveryCount int
	hour           int
}

// chooseDecision 先掷探索概率，未中或无候选则回落池内四因子抽样。
func (m *Lifecycle) chooseDecision(
	context context.Context,
	transaction *sql.Tx,
	accountID int64,
	input selectionInput,
) (decisionChoice, error) {
	signals := m.discoverySignals(input.snapshot, input.rerolls)
	if signals > 0 &&
		input.discoveryCount < m.discovery.MaxDiscoveriesPerMeal &&
		m.randomFloat() < engine.DiscoveryProbability(signals) {
		discovery, found, err := m.discoveryDish(
			context,
			transaction,
			accountID,
			input.snapshot,
			input.shown,
			input.hour,
		)
		if err != nil {
			return decisionChoice{}, err
		}
		if found {
			return discovery, nil
		}
	}

	weighted, relaxation := engine.ScorePool(input.snapshot.candidates, input.hour)
	// 放宽到本顿否决时才可能重现已展示的菜；只要还有别的选择，就不把
	// 正被换掉的那道原样端回来。
	if input.excludeDishID != "" && len(weighted) > 1 {
		remaining := make([]engine.Weighted, 0, len(weighted))
		for _, entry := range weighted {
			if entry.Candidate.ID != input.excludeDishID {
				remaining = append(remaining, entry)
			}
		}
		if len(remaining) > 0 {
			weighted = remaining
		}
	}
	if len(weighted) == 0 {
		return decisionChoice{}, ErrCandidatePoolEmpty
	}

	selected := weightedPick(
		m.randomFloat(),
		weighted,
		func(entry engine.Weighted) float64 { return entry.Weight },
	)
	reason := engine.ComposeReason(engine.ReasonInput{
		Relaxation: relaxation,
		Tier:       selected.Candidate.Tier,
		Distance:   selected.Candidate.Distance,
		EverEaten:  selected.Candidate.EverEaten,
		Occasion:   selected.Candidate.Occasion,
		Hour:       input.hour,
	})
	return decisionChoice{
		dishID: selected.Candidate.ID,
		name:   selected.Candidate.Name,
		mode:   ModePool,
		reason: reason,
	}, nil
}
