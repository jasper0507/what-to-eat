# 0006. Discovery feedback: rating gates pool admission and exclusion

## Status

Accepted

## Context

Discovery introduces out-of-pool dishes. Blind auto-add (option A) can pollute the pool; never-add (option C) fails to grow a small pool. The eater wants explicit feedback after trying a new dish.

## Decision

After a **Discovery** path that the eater accepts and tries, collect a **taste rating**:

1. **Rating ≥ high threshold** → **pool admission**: dish enters the candidate pool; **preference weight** is set from the rating (higher rating → higher weight).
2. **Rating ≤ low threshold** → **rejection mark**: do not enter the pool; never recommend this dish again (neither pool pick nor discovery).
3. **Between thresholds** → no pool admission, no rejection mark (may appear in a future discovery subject to normal rules).

Exact numeric scale and threshold values are a follow-on product decision; the gate shape is fixed.

**Scope of rating UX:**

- **Discovery: mandatory** taste rating after the try path (gates admission / rejection mark), collected as a **pending rating on next app open** with recall cues (see ADR 0008) — not before the recipe.
- **In-pool acceptances: optional** — daily path stays accept → recipe; eater may later adjust preference weight or set a rejection mark from dish/pool UI, without blocking the main flow.

## Consequences

- Acceptance alone is not enough for pool growth from discovery — rating is the gate.
- Need UI for mandatory discovery rating without destroying accept → recipe (e.g. rate on recipe page, after meal, or next open until completed).
- Rejection marks must be stored on the account and filtered in both pool and discovery pipelines.
- Mid-band dishes can be re-discovered; consider light session/recency penalties later if that feels spammy.
- In-pool weights are maintained via onboarding, optional edits, and optional ratings — not a forced daily score.
