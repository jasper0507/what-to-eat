# 0002. Anti-repeat: hard cooldown + soft downweight (by acceptance count)

## Status

Superseded by [ADR-0022](0022-knowing-me-engine.md) (2026-07-27: the sequential cooldown → downweight → least-shown stack was replaced by the multiplicative four-factor engine; the event-based anti-repeat semantics — counting acceptances, not calendar days — carry forward as the freshness factor)

## Context

Eaters get tired of the same dish when it repeats too soon. Decisions are **on demand** (any number per day), so calendar-day cooldowns mis-count (two lunches same day would look like "one day"). Anti-repeat must track **how many times** the eater has accepted dishes since a given dish was last eaten.

## Decision

Use **hard cooldown + soft downweight**, measured in **eating records (acceptances)** for that eater:

1. **Cooldown (N):** A dish with an eating record among the eater's last **N** acceptances is **ineligible**. Default **N = 2** (not the same dish again until two other acceptances have happened — tune later).
2. **Recency window (W):** Among the last **W** acceptances (W > N), more appearances → lower weight. Default **W = 7**.
3. Beyond W acceptances ago, base **preference weight** only (plus session penalty for rerolls).
4. If the eligible set is empty, **relax** in order: soft downweight first, then temporarily shorten cooldown — never hard-fail when the pool is non-empty.
5. **Session penalty** for rerolls is unchanged (same decision session; not an eating record).

Discovery pressure may still use pool size / eligible count / rerolls; those are separate from this counter.

## Consequences

- Eating records need a total order (timestamp or monotonic sequence) per eater.
- "Days" never appear in the anti-repeat formula for v1.
- Multiple decisions on the same calendar day each advance the cooldown counters.
- Defaults (N=2, W=7) remain tunable without changing the event-based shape.
