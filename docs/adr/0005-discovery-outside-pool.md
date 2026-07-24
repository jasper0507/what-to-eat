# 0005. Discovery: occasional out-of-pool dish, labeled, similarity-based

## Status

Accepted

## Context

A small candidate pool plus anti-repeat cooldown can exhaust variety — the eater still "gets tired" because the pool itself is tiny. Pure random catalog picks ignore taste. The product needs a controlled way to introduce catalog dishes that are **not** in the pool.

## Decision

1. Introduce **Discovery**: sometimes the daily decision is a catalog dish **outside** the candidate pool.
2. Discovery dishes are chosen by **similarity** (ingredients and/or taste profile) to dishes in the pool that have **high preference weight**.
3. Discovery results are **always explicitly labeled** (never look like a normal pool decision).
4. Cadence and post-accept behavior (e.g. auto-add to pool) are product parameters to be fixed in follow-up decisions; the existence and labeling of Discovery are fixed.

## Consequences

- Need a similarity signal over HowToCook (tags, ingredients, category, embeddings, or simple heuristic).
- Decision pipeline has two modes: **Pool pick** vs **Discovery pick**, with a scheduler for when Discovery fires.
- UI must make the mode unmistakable to preserve trust in the core algorithm.
