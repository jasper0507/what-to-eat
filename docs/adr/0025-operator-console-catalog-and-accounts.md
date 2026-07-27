# 0025. Operator console: dual-source catalog, listing, thin account ops

Status: accepted

Self-hosted deploy needs a way for the **operator** (not an eater) to grow the catalog beyond HowToCook and clean up accounts. We reject folding this into Account roles or a multi-tenant admin product.

**Catalog.** Keep HowToCook import as upstream authority for imported `source_path` rows; operator-authored dishes live in a separate provenance and are never overwritten by import. Eater-facing "remove dish" is **unlist** (catalog listing off): hidden from search, pool add, and new decisions; row retained so history and existing references still resolve. Imported dishes may only be listed/unlisted in the console—not body-edited. HowToCook upgrades happen at deploy/import time, not via a console "sync from GitHub" action. New operator dishes are **decision-ready** (full recipe template + same deterministic taste-profile extraction as import).

**Accounts.** Console v1: read-only account list/search plus **hard delete** with cascade of that account's pool, meals, decisions, records, and sessions. Catalog is untouched. No disable/reset-password UI in v1.

**Auth boundary.** Operator credentials and session are separate from Account login (not a role bit on accounts).
