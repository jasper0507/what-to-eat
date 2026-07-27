# What to Eat

Helps one person decide what to eat for a meal when they have decision fatigue or "nothing sounds good."

## Language

**Eater**:
The single person the system decides for. One decision always targets exactly one eater.
_Avoid_: User (ambiguous), household, family

**Meal**:
One eating occasion for one eater. Created on demand (not limited to once per calendar day). Each acceptance is one occasion for anti-repeat counting.
_Avoid_: Menu (implies multi-dish table), plan (implies multi-day schedule), daily slot (v1 is not calendar-slot based)

**Dish**:
A named food option that can be chosen for a meal (e.g. "番茄炒蛋"). Identity comes from the catalog; decisions pick dishes, not free-text alone.
_Avoid_: Food, item, meal (meal is the occasion)

**Recipe**:
The cooking instructions and details for a dish, sourced from the catalog (HowToCook). Shown after acceptance.
_Avoid_: Using "recipe" to mean the decision result itself — that is a dish

**Catalog**:
The full corpus of dishes (and their recipes) an eater may search, pool, and be decided toward. Provenance is dual: **imported** dishes from HowToCook (upstream-authoritative body text) and **operator-authored** dishes (local-authoritative). Not an eater's shortlist.
_Avoid_: Menu, library (ambiguous), candidate pool, "fixed dish pool" (say catalog)

**Catalog listing**:
Whether a catalog dish is offered to eaters for search, pool add, and new decisions. **Listed** is the default; **unlisted** hides it from those paths while keeping the row so history, past decisions, and existing pool membership can still resolve name and recipe. Unlisting is the operator's "remove from the product" action — not a hard delete of the dish row.
_Avoid_: Delete dish (ambiguous with destroying history), archive (vague), ban

**Taste profile**:
A dish's structured taste identity, extracted deterministically at catalog import from the recipe itself — main ingredients, flavor types, techniques (via dictionaries kept in the repo as data), plus the coarse catalog category. Powers discovery similarity and the per-dimension wording of decision reasons.
_Avoid_: Embeddings / learned vectors (banned for v1), LLM-generated tags (import stays deterministic and offline-capable), hand-curated per-dish tags

**Candidate pool**:
The eater's editable subset of catalog dishes that the decision algorithm may pick from. Empty pool means no decision is possible.
_Avoid_: Catalog (catalog is the full source), favorites (favorites are weight, not membership)

**Preference weight**:
The eater's relative affinity for a dish in their candidate pool (higher = more loved). Used as the base weight before cooldown / recency / session penalties. The number is internal-only: eaters express and adjust affinity exclusively through the taste-rating feeling tiers (labeled, slang), in starter-pack picks, manual pool edits, and post-meal ratings alike.
_Avoid_: Priority, score (too generic), rating (implies 5-star UI); numeric weight controls in UI (sliders, spinners, bare numbers)

**Decision**:
Exactly one dish the system presents as the answer for a specific meal of a specific eater. Not a shortlist.
_Avoid_: Suggestion, recommendation, pick, shortlist

**Decision stage**:
The single central slot on the daily home screen where a meal's decision plays out in place, with no navigation. It renders exactly one of three mutually exclusive states — empty-pool guidance, pending-rating gate, or ready-to-decide — and hosts the reveal of a decision with its accept / reroll actions. Acceptance leaves the stage (to the dish's recipe); reroll re-resolves on the stage.
_Avoid_: Modal / popup (the reveal is not an overlay dialog), "today's recommendation" (a meal is not a calendar slot; a decision is not a recommendation)

**Acceptance**:
The eater commits to a decision as what they will actually eat for that meal. Creates an eating record and navigates to that dish's recipe.
_Avoid_: Like, favorite, bookmark

**Reroll**:
The eater rejects the current decision and asks for a different dish for the same meal. Does not count as acceptance.
_Avoid_: Refresh, shuffle, next (too vague alone)

**Reroll budget**:
The per-meal cap (3) on rerolls, settled server-side. When exhausted, the eater's exits are: accept the standing decision, hand-pick a pool dish as the meal's outcome (unlocked only at exhaustion), or abandon the meal. Every reroll spends budget and doubles as a "didn't get me" signal (drift demotion, discovery pressure).
_Avoid_: Unlimited reroll (repealed 2026-07-27), daily quota (the budget is per meal, not per day)

**Meal abandonment**:
The eater closes an undecided meal without eating: the meal settles as abandoned, no eating record is created, and anti-repeat (cooldown / recency) is untouched. An abandoned meal's rerolls still count toward discovery pressure — walking away is the loudest "didn't get me" signal there is.
_Avoid_: Cancel / delete (the meal remains a settled fact), treating abandonment as acceptance

**Eating record**:
A durable fact that an eater accepted a dish for a meal (date + meal + dish). The only input to multi-day anti-repeat.
_Avoid_: View history, impression, suggestion log

**Session penalty**:
In-memory (or same-meal) downweight/exclusion of dishes already shown and rejected via reroll for the current meal. Not an eating record.
_Avoid_: Ban, blacklist (those sound permanent)

**Cooldown**:
A hard ineligibility span counted in **eating records (times)**, not calendar days: after a dish is accepted, it must not be chosen again until N further acceptances (for that eater) have occurred.
_Avoid_: Ban period, day-based cooldown (rejected for v1 anti-repeat)

**Recency window**:
A longer lookback than cooldown, also in **eating-record counts**, in which past acceptances of a dish only reduce its weight instead of forbidding it.
_Avoid_: History window (ambiguous with full history storage); day-based windows for anti-repeat

**Starter pack**:
A fixed, curated set of widely loved catalog dishes offered on the empty-pool welcome as the primary way to seed a candidate pool: the eater picks the ones they like and assigns feeling tiers on the spot. The other pool-entry path is manual catalog search. There is no AI onboarding — the taste interview was abolished 2026-07-27 (ADR-0023).
_Avoid_: Taste interview / onboarding interview (abolished, do not revive), default pool (the eater always picks; nothing enters unchosen), recommendation

**Account**:
The signed-in identity (first-party username + password) that owns an eater's candidate pool, preference weights, and eating records. Required before using the product. No third-party login providers in v1.
_Avoid_: Profile (ambiguous), user (prefer eater for the decision subject; account is the auth identity); OAuth identity; admin / operator (those are not accounts)

**Operator**:
The human who runs this deployment (typically the self-hoster), not an eater. Uses a separate operator console to maintain the catalog (add full dishes, toggle catalog listing) and to inspect or permanently remove accounts. Not an Account role and not a multi-tenant "admin user" inside the eater product.
_Avoid_: Admin user, superuser, staff account, user management portal (enterprise SaaS framing)

**Operator-authored dish**:
A catalog dish whose recipe body is owned by the operator (created via the operator console from a full recipe template), never overwritten by HowToCook import. Same decision rights as imported dishes when listed: search, pool, discovery, ratings.
_Avoid_: Custom menu item, user-generated dish (eaters do not author catalog recipes in this product), fork of a HowToCook row (imported rows are not edited in place)

**Discovery**:
A decision whose dish is drawn from the catalog **outside** the candidate pool, chosen by **taste-profile similarity** (main-ingredient / flavor / technique / category overlap, weighted by the reference dish's affinity) to resemble the dishes the eater loves most. Always explicitly labeled as new/exploratory — never presented as a normal pool pick.
_Avoid_: Recommendation, random, surprise me (vague); do not call a pool pick "discovery"; embedding-based "similar" in v1

**Taste rating**:
The eater's 1–5 feedback on a dish after trying it, each level shown with a slang feeling label (not bare numbers). Canonical tier labels: 1 拉完了, 2 NPC, 3 人上人, 4 顶尖, 5 夯. Drives pool admission (≥3 人上人+), preference weight, and rejection mark (≤2). Pool-add flows (starter pack, manual add) offer only the top three tiers — you never add a dish you already dismiss; the bottom two exist only in post-meal rating, where they mean rejection.
_Avoid_: Like/dislike only (too coarse for weight); do not present as a numeric-only control without labels

**Pending rating**:
An accepted discovery (or other rated path) that still awaits a taste rating. Surfaced on the next app open with enough dish context (name, and other recall cues) for the eater to remember the meal. While any pending rating exists, the eater cannot start a new Decision until it is resolved.
_Avoid_: Notification (channel detail), review queue (too enterprise)

**Pool admission**:
Adding a catalog dish into the candidate pool because its taste rating met the admission threshold (≥3 / 人上人+); preference weight is derived from that rating (人上人 = medium-low).
_Avoid_: Import, favorite

**Rejection mark**:
A durable flag that a dish must not be offered again (pool pick or discovery) because its taste rating fell below the low threshold.
_Avoid_: Ban, dislike (dislike might be soft; rejection mark is hard)

**Discovery pressure**:
A dynamic signal (from small pool size, high recent rerolls, few cooldown-eligible pool dishes, etc.) that raises the chance the next decision is a discovery rather than a pool pick. Not a fixed calendar interval.
_Avoid_: Schedule, quota, weekly slot
