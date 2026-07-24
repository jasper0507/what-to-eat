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
The full external corpus of dishes (and their recipes) available to search and import — HowToCook.
_Avoid_: Menu, library (ambiguous), candidate pool

**Candidate pool**:
The eater's editable subset of catalog dishes that the decision algorithm may pick from. Empty pool means no decision is possible.
_Avoid_: Catalog (catalog is the full source), favorites (favorites are weight, not membership)

**Preference weight**:
The eater's relative affinity for a dish in their candidate pool (higher = more loved). Used as the base weight before cooldown / recency / session penalties.
_Avoid_: Priority, score (too generic), rating (implies 5-star UI)

**Decision**:
Exactly one dish the system presents as the answer for a specific meal of a specific eater. Not a shortlist.
_Avoid_: Suggestion, recommendation, pick, shortlist

**Acceptance**:
The eater commits to a decision as what they will actually eat for that meal. Creates an eating record and navigates to that dish's recipe.
_Avoid_: Like, favorite, bookmark

**Reroll**:
The eater rejects the current decision and asks for a different dish for the same meal. Does not count as acceptance.
_Avoid_: Refresh, shuffle, next (too vague alone)

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

**Onboarding interview**:
A first-run guided AI conversation (product-hosted, via NVIDIA NIM on the server) that helps the eater build an initial candidate pool and set preference weights. Not used for the daily decision itself.
_Avoid_: Chatbot, daily assistant (daily path is non-AI decision); client-side API keys

**Account**:
The signed-in identity (first-party username + password) that owns an eater's candidate pool, preference weights, eating records, and onboarding results. Required before using the product. No third-party login providers in v1.
_Avoid_: Profile (ambiguous), user (prefer eater for the decision subject; account is the auth identity); OAuth identity

**Discovery**:
A decision whose dish is drawn from the catalog **outside** the candidate pool, chosen by **category/tag/name heuristics** to resemble high preference-weight pool dishes. Always explicitly labeled as new/exploratory — never presented as a normal pool pick.
_Avoid_: Recommendation, random, surprise me (vague); do not call a pool pick "discovery"; embedding-based "similar" in v1

**Taste rating**:
The eater's 1–5 feedback on a dish after trying it, each level shown with a slang feeling label (not bare numbers). Drives pool admission (≥3), preference weight, and rejection mark (≤2).
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
