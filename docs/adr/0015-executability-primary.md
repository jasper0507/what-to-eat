# 0015. Primary failure mode is executability (not preference optimization)

## Status

**Withdrawn**

## Context

Grilling briefly identified stuckness as mainly **executability** (answer cannot be cooked/handed off), with **on-hand ingredients** explored as the concrete gate.

## Decision (withdrawn)

~~Make executability constraints first-class in the decision pipeline ahead of preference / anti-repeat.~~

## Withdrawal

The operator declined to maintain a **pantry / ingredient** model (too hard to keep accurate). No other executability constraint was adopted for v1.

**v1 does not implement executability gates.** Decision order returns to: candidate pool (or discovery) → preference weight → cooldown-by-count → session penalty. Recipe-after-accept remains for handoff, but there is no inventory/time/cook filter.

If executability is revisited later, start from a constraint that is cheaper to maintain than a live pantry — do not revive this ADR as-is.

## Consequences

- ADR 0016 is also withdrawn.
- Glossary terms Executable / Constraint / Pantry / Ingredient were removed from `CONTEXT.md`.
- Research notes on ingredient-match apps remain historical only; not a v1 product requirement.
