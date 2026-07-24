# 0019. Meal lifecycle uses one deep module at the public HTTP seam

## Status

Accepted

## Context

The daily path spans **Decision**, **Reroll**, **Acceptance**, anti-repeat, **Discovery**, **Pending rating**, **Taste rating**, and rating-driven **Pool admission** or **Rejection mark**. Splitting those rules into exported policies and per-table repositories would make callers assemble domain state, coordinate transactions, and reproduce ordering rules. A single generic command dispatcher would reduce method count without reducing what callers must understand.

## Decision

Place the stable seam at the authenticated **Meal lifecycle** exposed through HTTP. Behind it, one deep `MealLifecycle` module owns the lifecycle rules, persistence coordination, and returned domain results.

Its interface has five behaviours:

1. `Resume` observes the current state without creating a **Meal**. It returns one of: pending ratings, active decision, empty candidate pool, or ready, in that precedence order.
2. `Begin` creates an on-demand **Meal** from ready state and returns exactly one **Decision**.
3. `Reroll` replaces the current decision for the same meal without creating an **Eating record**. Repeating the same request returns the same replacement decision.
4. `Accept` idempotently and atomically creates at most one eating record and returns the accepted dish's **Recipe** reference plus any pending rating.
5. `Rate` idempotently resolves a pending rating and atomically applies either pool admission or a rejection mark. Repeating the same rating returns the original outcome; a conflicting replacement is rejected.

The authenticated **Account** comes from the session, never from the request payload. Resources belonging to another account are indistinguishable from missing resources.

The module hides candidate selection, preference weighting, cooldown, recency, relaxation, session penalties, discovery pressure and similarity, rejection filtering, eating-record ordering, SQLite queries, and transaction structure. Its results expose domain identities and outcomes, not candidate lists, intermediate scores, or storage details.

Use a concrete Go implementation rather than defining a Go `interface` for the module while only one implementation exists. Gin remains in the HTTP adapter. Production and tests both use real SQLite, with tests using an isolated database and fixture catalog; do not introduce per-table repository interfaces. NVIDIA NIM belongs only to the **Onboarding interview** module behind a private port with production and scripted-test adapters. Clock and randomness may vary through private constructor dependencies without joining the public interface.

Tests exercise the same public HTTP seam as callers. They assert observable state transitions and domain invariants without querying SQLite directly or calling internal selection functions.

## Considered Options

- A stateless chooser accepting candidate pool, history, policy, and randomness was rejected because it leaks the implementation state through its interface and leaves persistence and transaction rules in callers.
- Separate cooldown, recency, discovery, and per-table repository modules were rejected as shallow modules with hypothetical seams.
- A single `Execute(Command)` method was rejected because its command variants preserve all five behaviours and their ordering constraints while obscuring their error modes.

## Consequences

- Policy, storage, and UI changes can evolve behind one stable seam while callers learn only five behaviours.
- `MealLifecycle` is intentionally implementation-heavy; splitting it is justified only when a second real adapter or an independently changing responsibility appears.
- Public HTTP behaviour is the primary regression surface. Lower-level tests are added only when this seam cannot provide precise, economical evidence.
- A future database replacement should first demonstrate a second adapter before introducing a storage port.
