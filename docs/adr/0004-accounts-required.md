# 0004. Accounts required (no anonymous mode in v1)

## Status

Accepted

## Context

Candidate pool, preference weights, eating records, onboarding interview results, and (later) multi-device use all need a durable owner. Options: local-only, local + optional login, or mandatory accounts.

## Decision

**Mandatory accounts** for v1. First-run means sign-up/sign-in, then onboarding interview. Pool, weights, and eating records are stored server-side against the account (not browser-local as the source of truth).

## Consequences

- Backend + auth are on the critical path (cannot ship a pure static PWA as the full product).
- Sync and "switch phone" work by default; offline is a cache optimization, not the source of truth.
- Scope and ops cost are higher than the local-first apps surveyed in research.
- Privacy/compliance and auth provider choice become follow-on decisions.
