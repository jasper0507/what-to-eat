# 0017. v1 success criteria: stop fretting + anti-satiation (recipe ancillary)

## Status

Fulfilled — v1 shipped and passed independent acceptance (2026-07-26). Historical record; the current success metric is the redesign north star (first-reveal acceptance rate, computable from the decisions table) defined in docs/redesign-brief.md.

## Context

Executability/pantry was explored and **withdrawn** (ADRs 0015–0016). Need an explicit v1 success bar so implementation does not reintroduce heavy constraint systems.

## Decision

v1 optimizes for **both**:

1. **Stop fretting** — one hard **Decision**, clear Acceptance / Reroll; eater can end the loop quickly.
2. **Anti-satiation** — cooldown and recency by acceptance **count**, plus discovery/rating as already specified.

**Recipe after Acceptance** (HowToCook) is **ancillary handoff** (help start cooking / explain the dish), **not** a constraint engine and not measured as “kitchen inventory correctness.”

Out of scope for the v1 success bar: pantry match, cook-time gates, multi-person cook assignment.

## Consequences

- Prioritize decision UX + weight/history pipeline + pool/onboarding over inventory features.
- Do not block release on “is this cookable with what’s in the fridge.”
