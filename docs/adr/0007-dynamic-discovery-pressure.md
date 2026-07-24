# 0007. Dynamic discovery pressure (not fixed cadence)

## Status

Accepted

## Context

Discovery should grow variety when the candidate pool is too small or the eater is stuck. Fixed every-D-days is simple but ignores state. Fully opaque "AI decides when" is hard to trust and tune.

## Decision

Use **dynamic discovery pressure** to choose between **pool pick** and **discovery** for a given decision:

Inputs that raise pressure (illustrative; exact formula is implementation):

- Small candidate pool size
- Few dishes left eligible after cooldown / recency filters
- High recent reroll count (same meal or recent meals)
- Optionally: long time since last discovery acceptance / last pool admission

When pressure is high, the system is more likely (or certain above a threshold) to emit a **labeled Discovery** decision. The eater can still **Reroll** within discovery rules; rejection marks and similarity constraints still apply.

v1 may start with a transparent weighted score + threshold rather than a learned model.

## Consequences

- Need telemetry-like counters on the account (pool size, rerolls, last discovery time, eligible count).
- Harder to explain than "every Monday" — UI should still say *why* when useful ("常吃里可选项不多，今天试试新菜").
- Must cap discovery spam (e.g. max one discovery decision per meal, cooldown after a discovery even if pressure stays high).
- Fixed cadence remains a possible future override, not the default policy.
