# Jev R1 — Decision / knowing-me codebase seams

**Date:** 2026-09-22 (CST / Asia/Shanghai)  
**Repo:** `jasper0507/what-to-eat` local clone  
**Audience:** 产品架构师 (R1 Jev integration design)  
**Constraint:** Read-only survey; no app code changes in this pass.  
**Companions:** `docs/adr/0022-knowing-me-engine.md`, `docs/adr/0023-taste-interview-abolished.md`, `docs/research/jev-system-one-2026-09.md`, `docs/research/jev-github-integrations-2026-09.md`

---

## 1. Where decisions are created

| Layer | Package / symbol | Role |
| --- | --- | --- |
| HTTP handlers | `internal/server/meal.go` | `beginMeal`, `rerollDecision`, `acceptDecision`, `abandonMeal`, `handPickDish`; bind `local_hour` via `resolveLocalHour` |
| Routes | `internal/server/server.go` `(*App).routes` | `POST /api/meals`, `POST /api/decisions/:id/reroll\|accept`, `POST /api/meals/abandon\|hand-pick`, `GET /api/meals/resume` |
| Composition root | `internal/server/server.go` `newApp` | `meals: meal.New(db, candidates, decisionRandom, discovery)` |
| Lifecycle shell | `internal/meal.Lifecycle` | `Begin` / `Reroll` → `chooseDecision` → `insertDecision`; owns transactions, Reroll budget, modes `pool` \| `discovery` \| `hand_pick` |
| Selection | `internal/meal/selection.go` | `chooseDecision`, `discoveryDish`, `poolCandidates`, `discoverySignals` |
| Pure scoring | `internal/engine` | `ScorePool`, `ScoreDiscovery`, `ComposeReason`, factor helpers — **no DB, no RNG** (package comment) |
| Sampling | `internal/meal/lifecycle.go` | `weightedPick`, `(*Lifecycle).randomFloat` (mutex-guarded `*rand.Rand`) |

Call path (pool reveal):

```
beginMeal / rerollDecision
  → meal.Lifecycle.Begin / Reroll
    → chooseDecision
        → (optional) discoveryDish → engine.ScoreDiscovery → weightedPick
        → else engine.ScorePool → weightedPick → engine.ComposeReason
    → insertDecision (SQLite `decisions` row)
```

Wire types: `meal.Decision`, `meal.State` (JSON tags); errors mapped in handlers to codes in `internal/server/wire_codes.go` (`decision_not_found`, `reroll_budget_exhausted`, `hand_pick_locked`, …).

---

## 2. Knowing-me scoring (ADR-0022)

**Formula:** `Score = 喜爱 × 新鲜感 × 场合 × 本顿否决` then weighted sample.

| Factor | Code | Notes |
| --- | --- | --- |
| 喜爱 | `engine.TierMultiplier` | 人上人 0.5 / 顶尖 1.0 / 夯 2.0 (`TierRenShangRen|DingJian|Hang`) |
| 新鲜感 | `engine.FreshnessFactor(distance, everEaten)` | d≤1→0 … ≥12→1.2; never eaten → 1.0 |
| 场合 | `engine.ClassifyOccasion` + `OccasionFactor` | breakfast 5–10h; `condiment|drink|dessert|semi-finished` → `OccasionNever` (always 0) |
| 本顿否决 | `Candidate.ShownThisMeal` in `score` | weight → 0 unless relaxation reaches `RelaxShown` |

**Relaxation:** `engine.ScorePool` loops `RelaxNone → RelaxFreshness → RelaxOccasion → RelaxShown`; `OccasionNever` never revived.

**Discovery:** `engine.DiscoveryProbability(signals)` = min(0.75, signals×0.25). Signals counted in `meal.(*Lifecycle).discoverySignals` (pool small / freshness-eligible few / recent rerolls). Candidate weight via `engine.ScoreDiscovery` = max(similarity × ref tier) × occasion × freshness. Similarity: `engine.Similarity` (5×主料 + 4×味型 + 2×工艺 + 1×品类); profiles from `catalog.Profiles`.

**Reasons:** `engine.ComposeReason` — priority: 放宽 > Discovery hits > 久别重逢 > 场合 > 档位兜底.

**Config knobs (not secrets):** `meal.DiscoveryConfig` / `DefaultDiscoveryConfig()` — Enabled, MaxPoolSize=3, MaxEligibleDishes=1, MinRerolls=2, RecentMealWindow=3, MaxDiscoveriesPerMeal=2; overridable via `server.Config.Discovery`.

---

## 3. Existing test seams

| Seam | Location | What it buys |
| --- | --- | --- |
| Table-driven pure unit tests | `internal/engine/engine_test.go` | Factor curves, `ScorePool` / relaxation, `ScoreDiscovery`, `ComposeReason`, similarity — **no interface needed** |
| Seeded RNG | `server.NewWithDecisionRandomSeedForTest` (`test_support_test.go`) → `newApp(..., rand.NewSource(seed))` | Deterministic Begin/Reroll/Discovery HTTP tests |
| Public HTTP / Meal lifecycle | `meal_lifecycle_test.go`, `pending_rating_test.go`, `deployment_test.go` | ADR-0019 preferred regression surface |
| Internal DB queryer | `meal.sqlQueryer` | Same read helpers on `*sql.DB` or `*sql.Tx` — not a decision seam |
| FE stage pure model | `frontend/src/lib/stage.test.ts` | `toStage` / exhausted split |

**No** exported `DecisionEngine` / `Chooser` interface today. ADR-0019 + ADR-0021: concrete modules; interfaces only when a **second real adapter** appears (historical NIM port abolished by ADR-0023). Randomness is already a private constructor dependency — precedent for injecting a second chooser.

---

## 4. Env / config patterns for secrets

| Source | Keys |
| --- | --- |
| `.env.example` | `SESSION_SECRET=` only |
| `compose.yaml` | `SESSION_SECRET: ${SESSION_SECRET:?…}` (required) |
| `server.ConfigFromEnv` | `SESSION_SECRET` (≥32 bytes enforced in `newApp`), `DATABASE_PATH`, `APP_ENV`→`SecureCookies`, `WEB_DIR`, `CATALOG_DIR` |
| Discovery | **not** from env — struct on `Config.Discovery` / defaults |

**Post–ADR-0023:** runtime is zero outbound LLM; `NVIDIA_API_KEY` / `NIM_*` removed. Any Jev R1 secret should follow the same pattern: env → `Config` field → validate at `newApp`, document in `.env.example` + compose (optional flag so default deploy stays zero-network).

---

## 5. Frontend Decision-stage / reveal entry points

| File | Role |
| --- | --- |
| `frontend/src/lib/stage.ts` | `toStage(MealState)` → `empty_pool` \| `pending_ratings` \| `ready` \| `reveal` \| `exhausted`; `DISCOVERY_STAGE_BADGE` |
| `frontend/src/pages/HomePage.tsx` | Stage adapter; `Reveal` UI (name + reason + reroll/accept; exhausted → hand-pick / abandon) |
| `frontend/src/api/endpoints.ts` | `beginMeal` / `rerollDecision` send `{ local_hour }`; accept / abandon / hand-pick |
| `frontend/src/api/hooks.ts` | `useMealState`, `useBeginMeal`, `useReroll`, `useAccept`, `useHandPick`, `useAbandonMeal` |
| `frontend/src/api/types.ts` | `Decision.mode`, `MealState`, wire error union |

R1 should **not** need FE changes if Jev only changes which dish/reason is returned inside existing `MealState` / `Decision` wire.

---

## 6. ADR summaries

### ADR-0022 — 懂我引擎 (`docs/adr/0022-knowing-me-engine.md`)

Accepted 2026-07-27. Replaces sequential cooldown/recency/leastShown stack with multiplicative four-factor scoring, layer-wise relaxation, probabilistic Discovery (s/4), taste-profile similarity (not name bigrams), mandatory Chinese `Decision.reason`, tier storage, Reroll budget 3 + three exits. Consequence: pure `internal/engine`; `internal/meal` = transaction shell + bookkeeping.

### ADR-0023 — 废止 AI 口味访谈 (`docs/adr/0023-taste-interview-abolished.md`)

Accepted 2026-07-27; supersedes ADR-0010. Removes `/api/onboarding/*`, `internal/onboarding`, NIM config/secrets, interview tables. Product runtime: **zero LLM / zero egress**; only `SESSION_SECRET`. Onboarding path = starter pack + hand-pick only. Do-not-resurrect for taste interview — **does not** forbid a separate, optional System One integration if product later opts in (treat as new ADR, not revival of NIM interview).

---

## 7. Outbound HTTP client patterns

**None in production code today.** `net/http` usages are inbound server + `httptest` tests + `cmd/server` listen. Former pattern (abolished): private NIM port inside `internal/onboarding` with production vs scripted-test adapters (ADR-0019 / 0010 / 0023). Closest live Go product shape for optional Jev: SiYuan-style conf (`Endpoint`, model, timeout) cited in research note — not in this repo.

R1 will need a **new** outbound client package; do not invent one by copying interview code (gone). Prefer: thin `internal/jev` (or `internal/systemone`) with `Client` interface + real HTTP + fake for tests, composed **above** `engine` arithmetic.

---

## Proposed deepest external seam for Jev R1 (one high seam)

### Recommendation: inject a `Chooser` into `meal.Lifecycle`, replace body of `chooseDecision`

Grounded names:

```text
internal/meal/selection.go     chooseDecision / discoveryDish   ← call site
internal/engine                ScorePool / ScoreDiscovery / …   ← keep as Local prefilter
internal/server/server.go      newApp                           ← wire adapter
```

Proposed shape (names illustrative; package placement to confirm in design ADR):

```go
// Prefer package meal (call site) or a tiny internal/decision port.
type Chooser interface {
    Choose(ctx context.Context, in ChoiceInput) (ChoiceResult, error)
}

// ChoiceInput = what chooseDecision already has after poolCandidates:
// snapshot candidates, shown, excludeDishID, rerolls, discoveryCount, hour,
// accountID + *sql.Tx only if adapter must load profiles (Local needs this).
```

| Adapter | Behavior |
| --- | --- |
| `LocalChooser` (default) | Exact current path: discovery coin → `ScoreDiscovery` or `ScorePool` → `weightedPick` → `ComposeReason` |
| `JevComposedChooser` | **Always** run Local knowing-me to drop zeros / build shortlist + base weights; then Jev Choice/Score/Noul overlay per research Pattern A/B; on Jev failure / low confidence → fall back to Local sample; reason may append or replace via `ComposeReason` + Jev blurb |

**Why this seam (not deeper / not shallower):**

- Deeper (`ScorePool` internals): would force Jev into numeric freshness/cooldown — research explicitly warns against Score for arithmetic; breaks table-driven engine tests as the unit of truth.
- Shallower (HTTP handlers / FE): duplicates lifecycle rules (budget, idle resume, discovery caps); violates ADR-0019 deep module.
- `Chooser` at `chooseDecision` is the **first point** where “pick one dish + mode + reason” is a single function, and ADR-0019 already authorizes a second adapter when one appears.

**Keep outside the seam:** `Begin`/`Reroll` transaction structure, `insertDecision`, demotion (`pool`), pending-rating gates, FE stage — unchanged for R1.

---

## Files R1 is likely to touch

| Likely | Why |
| --- | --- |
| `internal/meal/lifecycle.go` | Inject `Chooser` (or optional field) on `Lifecycle` / `New` |
| `internal/meal/selection.go` | Delegate `chooseDecision` to interface; keep snapshot SQL here or pass into Local |
| `internal/server/server.go` | Config + wire Local vs Jev-composed |
| **New** `internal/jev/` (client + composed adapter + fake) | Outbound System One + fallback |
| `.env.example`, `compose.yaml`, README config table | Optional `JEV_*` / `TYPESAFE_*` secret + endpoint |
| `internal/server/meal_lifecycle_test.go` (+ new jev unit tests) | Fake Chooser / seeded Local regression |
| Possibly `internal/engine/reasons.go` | Only if reason wire needs a Jev-specific sentence helper |

| Unlikely for R1 | Why |
| --- | --- |
| `frontend/**` | Wire already carries `reason` + `mode` |
| `internal/engine/*.go` factor math | Stay deterministic prefilter |
| Schema / pool demotion | Unrelated |

---

## Risks if the seam is wrong

1. **Seam inside `engine.ScorePool`:** Jev replaces or muddies freshness/cooldown → unverifiable “懂我” arithmetic; ADR-0022 curves become dead code mixed with model noise.
2. **Seam at HTTP only:** Lifecycle invariants (idempotent reroll, budget, discovery caps) reimplemented or bypassed; HTTP tests no longer prove selection policy.
3. **Replacing Local entirely with Jev Choice over full catalog:** Choice is relative (probs sum to 1); can “win” mediocre options; OccasionNever / cooldown can resurface — product regressions vs empty-pool / never-reveal rules.
4. **Blocking Begin on network:** Today Begin is local SQLite; hard dependency on Jev breaks offline/single-node deploy story (ADR-0014) and ADR-0023 zero-egress default — must be opt-in + soft-fallback.
5. **Reviving NIM-shaped interview module:** Conflicts with ADR-0023 do-not-resurrect; Jev must be framed as optional decision assist, not onboarding interview.
6. **Skipping a fake client:** Only HTTP lifecycle tests with live API → flaky CI, secret leakage risk, no deterministic seed story (`NewWithDecisionRandomSeedForTest` becomes insufficient).

---

## Success checklist for the follow-on design ADR

- [ ] Default binary path == today’s LocalChooser (bit-compatible under same RNG seed).
- [ ] Jev path is config-gated; missing key ⇒ Local.
- [ ] Engine table tests remain the source of truth for four factors.
- [ ] One fake `Chooser` / `jev.Client` used in unit tests; HTTP tests still use seeded Local.
- [ ] No FE contract change required for R1 MVP.
