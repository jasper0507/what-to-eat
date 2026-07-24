# 0018. Pending taste ratings hard-block new decisions

## Status

Accepted

## Context

Discovery acceptances create a **pending rating**, collected on the next app open with recall cues. Options: hard-block new decisions until rated, nag-but-skip, or silent queue.

## Decision

**Hard block (A):** if the account has any unresolved **pending rating**, the eater must complete the taste rating(s) before they can request a new **Decision**.

- Next open surfaces pending items first (dish name + recall context).
- Multiple pendings must all be cleared (or product may require one-by-one in order).
- In-pool optional ratings never create this block — only mandatory discovery (rated path) pendings do.

## Consequences

- Higher completion rate for pool admission / rejection marks.
- Risk: eater is hungry and blocked; keep rating UI extremely fast (five labeled choices, no essay).
- Acceptance → recipe still works on the discovery day; block applies to the **next** decision request, not to reading the recipe.
