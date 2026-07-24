# 0008. Taste rating: 1–5 slang labels; rate on next open; ≥3 admits to pool

## Status

Accepted (revised: 人上人 admits with medium-low weight)

## Context

Discovery requires a taste rating that gates pool admission and rejection marks. Rating is deferred to **next app open** with recall cues. Informal slang labels fit the product tone better than bare numbers.

## Decision

### Scale and labels (1 = worst … 5 = best)

| Stars | Feeling label | System effect |
|------:|---------------|---------------|
| 1 | 拉完了 | **Rejection mark** — never recommend again |
| 2 | NPC | **Rejection mark** — never recommend again |
| 3 | 人上人 | **Pool admission** with **medium-low** preference weight |
| 4 | 顶级 | **Pool admission** with high preference weight |
| 5 | 夯 | **Pool admission** with highest preference weight |

**Thresholds:**  
- Rejection: ≤2  
- Pool admission: ≥3 (including 人上人)

**Default weight mapping (illustrative, in-pool):**

| Stars | Label | preference_weight |
|------:|-------|-------------------|
| 3 | 人上人 | 0.7 (medium-low) |
| 4 | 顶级 | 1.0 |
| 5 | 夯 | 1.3 |

(Exact numbers tunable; ordering 3 < 4 < 5 is fixed. Stars 1–2 never enter the pool.)

UI presents **labels first** (optionally with stars).

### When to collect

**On next app open (deferred):**

1. Discovery **Acceptance** → eating record + navigate to **recipe** + create **pending rating**.
2. **Next open** surfaces pending rating with **recall context** (dish name required; meal time and other cues recommended).
3. Pending remains until resolved; nag/block strength before new decisions is a later UX detail.

## Consequences

- Mid-band is no longer "no admission"; only 1–2 are hard outs.
- 人上人 grows the pool faster (good for small pools) but stays deprioritized vs 顶级/夯.
- Deferred rating + on-demand decisions can stack multiple pending ratings — UI should list them clearly.
