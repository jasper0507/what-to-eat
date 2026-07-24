# 0016. Primary executability constraint: pantry (on-hand ingredients)

## Status

**Withdrawn**

## Context

Following ADR 0015, on-hand ingredients were chosen as the primary executability constraint (pantry match before preference weights).

## Decision (withdrawn)

~~v1 pantry as hard/soft gate on pool and discovery picks.~~

## Withdrawal

Operator decision: **do not model materials/pantry** — maintenance burden is too high and would leave the product wrong when data is stale.

No pantry, ingredient inventory, or recipe–pantry matching in v1. See withdrawn ADR 0015.

## Consequences

- Do not implement pantry UX, staple lists, or post-cook stock deduction.
- HowToCook ingredient lines may still appear inside **recipe display** after acceptance; they are not used for decision filtering.
