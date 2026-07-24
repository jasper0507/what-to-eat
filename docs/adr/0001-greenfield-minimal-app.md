# 0001. Greenfield minimal app (not fork)

## Status

Accepted

## Context

We need a responsive web app that produces a single **Decision** (one **Dish** for one **Eater**'s **Meal**) when the eater has decision fatigue. Open-source survey (`docs/research/github-meal-decision-apps.md`) found no ready-made product that combines Chinese/home dishes, one-tap daily decision, anti-repeat ("tired of the same food"), and a deliberately minimal UI under a permissive license.

Closest options:

- **饭搭子** — product fit is high (history, no-repeat, weighted scoring) but **All Rights Reserved**; cannot fork or reuse code/data.
- **ryanuo/whatToEat** — MIT, Nuxt/PWA, HowToCook data, spin UI; missing history/anti-satiation; recommend logic is unused. Forking would require adding our core algorithm while carrying an existing shell.

## Decision

Build a **greenfield minimal PWA/web app**. Borrow UX patterns (spin / one big action / "not this, next") and algorithm *ideas* (cooldown + weighted random + history) from research; optionally use **HowToCook** as a content/data source. Do not fork whatToEat or any non-permissive project as the product base.

## Consequences

- Full control over the domain model (Eater → Meal → Dish → Decision) and a thin UI.
- We own stack, deploy, and dish-catalog bootstrap work.
- Must re-implement (cleanly) history and anti-repeat rather than patching an upstream app.
- Legal/clarity: only MIT/Unlicense (etc.) sources are in the dependency path; 饭搭子 remains idea-only.
