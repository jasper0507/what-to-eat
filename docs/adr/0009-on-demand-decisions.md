# 0009. On-demand decisions (not once-per-day)

## Status

Accepted

## Context

Eaters may need more than one decision per calendar day (or none). Slot-based lunch/dinner forces empty structure; once-per-day is too rigid.

## Decision

Decisions are **on demand**: whenever the eater asks, the system may produce a new **Decision** (pool pick or discovery per pressure rules). There is no v1 limit of one decision per day.

Anti-repeat is **per acceptance count** (see ADR 0002), so each acceptance advances cooldown/recency regardless of calendar day.

## Consequences

- Home UX is "再来一次 / 吃什么" not "今日已决定".
- Multiple pending ratings possible if several discoveries were accepted before reopening.
- Analytics and "daily streak" features are optional later; not core.
