# 0013. Discovery similarity: category/tag heuristics (not embeddings)

## Status

Accepted

## Context

Discovery picks a catalog dish outside the candidate pool that should feel similar (ingredients/taste) to high preference-weight pool dishes. HowToCook is markdown; options were coarse tags/categories, parsed ingredient overlap, or embeddings via NIM.

## Decision

v1 uses **heuristic similarity only**:

- Features from HowToCook structure and metadata: **category/path**, **dish name keywords**, difficulty or other light tags when present.
- Score candidates by overlap / match with the eater's **high preference-weight** pool dishes.
- No ingredient-graph parsing required for v1; no vector embeddings for discovery.
- NVIDIA NIM remains reserved for **onboarding interview**, not daily similarity.

If heuristics are too weak in practice, ingredient overlap (former option B) is the next upgrade path — not embeddings first.

## Consequences

- Implementable entirely in Go with catalog metadata at ingest time.
- Similarity quality is "same lane" not "same pantry"; labeling Discovery still sets expectations.
- Catalog ingest should preserve category and searchable name fields reliably.
