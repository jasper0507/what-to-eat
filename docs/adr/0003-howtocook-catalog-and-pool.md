# 0003. HowToCook as catalog; editable candidate pool; recipe after acceptance

## Status

Accepted

## Context

The decision algorithm needs a set of dishes. Options ranged from empty user-built lists, a small baked-in seed list, to large recipe corpora. The eater also needs real cooking guidance after they commit to a dish. HowToCook (https://github.com/Anduin2017/HowToCook, Unlicense) is a large Chinese home-cooking markdown corpus and is already the de-facto data source for related tools (e.g. whatToEat).

Using the entire corpus as the daily pick set would inject many dishes the eater will never cook, harming decision quality. A personal **candidate pool** is required.

## Decision

1. **Catalog** = HowToCook (dishes + recipes).
2. **Candidate pool** = eater-edited subset of the catalog. Membership is explicit (search/type name → pick from catalog → add to pool). Pool is what the daily algorithm samples from.
3. **Preference weight** lives on pool membership (loved higher, disliked lower) and is the base weight before anti-repeat adjustments.
4. **Onboarding interview** (AI, first run) accelerates initial pool + weights; it is not the daily decider.
5. **Acceptance** writes an eating record **and** navigates to that dish's **recipe**.

## Consequences

- Product depends on packaging or fetching HowToCook content (build-time sync, CDN, or git submodule) and a stable dish id ↔ recipe path mapping.
- Search-over-catalog UX is required for pool editing.
- AI is in the critical path for first-run quality, which implies provider choice, cost, and a non-AI fallback if interview is skipped.
- Daily path stays algorithm + pool (no LLM required per meal).
