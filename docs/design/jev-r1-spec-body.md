## Problem Statement

After using the product for a while, Decisions tend to collapse into a small island of familiar dishes. The knowing-me engine is strong at freshness, cooldown, and occasion arithmetic, but weak at “what this meal should feel like” and at breaking islanding with intentional explore. Catalog thinness hurts too, but that is a separate track. The eater wants a single, trustworthy Decision per reveal — still measurable by **首揭接受率** (first-reveal acceptance rate) — with optional cloud judgment that does not replace local anti-repeat math or invent free-form chat copy.

## Solution

Path **R1**: for each reveal, the knowing-me engine filters eligibility and builds a **Shortlist** (Top-K ≈ 16) with forced **Explore slots** (floor 2; raised to 4 when explore Noul is true). TypeSafe cloud **System One (Jev)** — using the operator’s official quota — runs a fan-out of `Choice(shortlist)` + `Noul(explore)` + `Score(fit)` and, when restriction signals exist, a conditional safety `Noul`. Go composes exactly one **Jev-composed decision** and fills the Chinese reason from local soft templates over Jev tiers (no second chat LLM; no hard-locked copy strings). Ranking failures fail-open to local weighted sample; allergy/safety Noul fails closed. Missing or invalid `TYPESAFE_API_KEY` yields a clear “Jev unavailable” error unless the eater/deploy turns on **Local-only mode** (default **off**). Optional **Meal intent** (Chinese preset tags as English enums in state + optional free text) may guide the reveal. Catalog enrichment stays a separate non-blocking Issue. Frontend in scope: Meal intent UI and the local-only switch.

## User Stories

1. As an eater, I want one clear Decision per reveal, so that I am not left choosing among a list.
2. As an eater, I want Decisions that still respect cooldown and freshness, so that I do not get the same dish too soon.
3. As an eater, I want occasional exploratory dishes that feel intentional, so that my meals do not island on a few favorites.
4. As an eater, I want Explore slots reserved in the Shortlist, so that familiar high-weight dishes cannot crowd out exploration.
5. As an eater, I want explore seats to expand when System One says explore is warranted, so that anti-island pressure can intensify for that reveal.
6. As an eater, I want optional Meal intent tags in Chinese before I begin, so that “清淡 / 重口 / 快手” style wishes shape this meal without a permanent profile rewrite.
7. As an eater, I want those tags stored as stable English enums in state, so that System One and the backend share unambiguous criteria.
8. As an eater, I want an optional free-text line for “这顿想怎样”, so that I can add nuance the presets do not cover.
9. As an eater, I want to skip Meal intent entirely, so that one-tap begin still works when I have decision fatigue.
10. As an eater, I want Meal intent to apply on Begin and on Reroll when I change my mind mid-meal, so that each reveal can follow the latest wish.
11. As an eater, I want a Chinese reason under the Decision that sounds human, so that the pick feels explained rather than random.
12. As an eater, I want reasons to come from local templates over System One tiers, so that copy stays on-brand without a chat model.
13. As an eater, I want soft (non-hard-locked) reason wording, so that product copy can iterate without Spec rewrites for every phrase.
14. As an eater, I want Discovery-mode Decisions still clearly labeled as exploratory, so that I know when I am leaving my Candidate pool.
15. As an eater, I want Accept and Reroll to behave as today at the Decision stage, so that cloud assist does not change the meal choreography.
16. As an eater, I want Reroll budget and the three exits (accept / hand-pick when exhausted / abandon) unchanged, so that server-settled limits still hold.
17. As an eater, I want pending-rating gates to still block a new Decision, so that discovery feedback is not skipped for cloud path.
18. As an eater, I want OccasionNever categories never revealed as the meal’s dish, so that condiments and drinks cannot win via Choice relative ranking.
19. As an eater, I want allergy or restriction safety to fail closed, so that a flaky cloud answer cannot serve something unsafe when restriction signals were present.
20. As an eater, I want ordinary ranking or fit failures to fall back to local weighted sample, so that a transient System One outage does not strand an empty Decision stage.
21. As an eater, I want fallback Decisions still to feel like real Decisions (with observable local path), so that I am not misled into thinking Jev always ran.
22. As an an operator with official TypeSafe quota, I want the product to call cloud System One with my key, so that Jev assists Decision composition.
23. As an operator, I want a clear error when `TYPESAFE_API_KEY` is missing or invalid, so that I know Jev is unavailable rather than silently drifting.
24. As an operator or eater, I want an explicit「仅本地模式」/ Local-only mode switch defaulted off, so that pure knowing-me reveals are an intentional choice.
25. As a demo-site operator without a key, I want to turn Local-only on, so that the site can still demonstrate knowing-me Decisions.
26. As an eater with Local-only off and no valid key, I want Begin/Reroll to refuse with a clear wire error, so that I am not given a fake “cloud” experience.
27. As an eater with Local-only on, I want knowing-me weighted sampling under the same lifecycle rules, so that seeded tests and demos stay deterministic.
28. As a product owner, I want success judged by 首揭接受率, so that R1 is optimized for first-reveal accept rather than endless reroll entertainment.
29. As a product owner, I want catalog enrichment tracked in a separate Issue, so that R1 can ship without waiting on corpus growth.
30. As a product owner, I want anti-island behavior from forced Explore slots plus explore Noul, so that islanding is addressed even before catalog expansion.
31. As a developer, I want knowing-me to own eligibility and Shortlist construction, so that System One never invents ineligible dishes.
32. As a developer, I want Top-K Shortlist size about 16, so that Choice stays within a bounded relative set.
33. As a developer, I want Explore slot floor of 2 and Noul(explore)=true → 4, so that explore quota is testable and explicit.
34. As a developer, I want one System One fan-out per reveal for Choice + explore Noul + fit Score, so that latency stays one round-trip when possible.
35. As a developer, I want conditional restriction Noul only when signals exist, so that we do not pay safety questions on every bland reveal.
36. As a developer, I want Go to compose the final Decision from Shortlist + answers, so that arithmetic and persistence stay in-process.
37. As a developer, I want a Chooser injected at chooseDecision, so that LocalChooser and JevComposedChooser share one lifecycle call site.
38. As a developer, I want not to seam inside engine.ScorePool, so that four-factor freshness truth remains table-testable.
39. As a developer, I want HTTP Meal lifecycle (ADR-0019) as the primary regression surface, so that Begin/Reroll/Accept invariants stay the contract.
40. As a developer, I want a fake jev.Client for unit tests, so that fan-out packing, timeouts, and fail-open/fail-closed policies are deterministic.
41. As a developer, I want engine table tests unchanged as the source of truth for 喜爱×新鲜感×场合×本顿否决, so that Jev cannot rewrite cooldown math.
42. As a developer, I want secrets only on the Go backend via env/config, so that the SPA never holds TYPESAFE_API_KEY.
43. As a developer, I want ADR-0026 to document the ADR-0023 exception, so that reviewers do not think the taste interview returned.
44. As a developer, I want FE Meal intent controls on the Decision stage path, so that tags and free text reach Begin/Reroll payloads.
45. As a developer, I want FE Local-only switch in settings or an equivalent deploy-facing control, so that the default-off policy is operable.
46. As an eater, I want existing Decision.reason and mode fields to carry the reveal, so that the stage UI does not need a parallel cloud panel.
47. As an eater, I want hand-pick after budget exhaustion to remain local pool selection, so that System One is not required for that exit.
48. As an eater, I want abandonment to still count toward discovery pressure without creating an eating record, so that walking away remains a loud signal.
49. As an operator, I want endpoint/timeout/model pin configurable with safe defaults, so that quota and latency can be tuned without code edits.
50. As a QA engineer, I want missing-key + Local-only off to assert a specific error code at HTTP, so that misconfig is regression-tested.
51. As a QA engineer, I want Local-only on + seeded RNG to match prior knowing-me Begin/Reroll behavior, so that R1 does not silently change local picks.
52. As a QA engineer, I want Shortlist size and Explore slot 2→4 covered in JevComposedChooser tests, so that anti-island seats cannot regress.
53. As a QA engineer, I want safety Noul failure to refuse the unsafe option (fail-closed), so that policy is proven in fake-client tests.
54. As a QA engineer, I want ranking failure to produce a local weighted Decision (fail-open), so that availability policy is proven.
55. As a privacy-conscious operator, I want only Shortlist-scale state sent to System One, so that we do not upload the entire Catalog every reveal.
56. As an eater, I want rejection marks and cooldown exclusions honored before Shortlist entry, so that Jev cannot resurrect banned or cooling dishes.
57. As an eater, I want session-shown dishes excluded or zeroed per knowing-me rules before Choice, so that rerolls do not reshuffle the same face.
58. As a product architect, I want glossary terms System One (Jev), Shortlist, Explore slot, Meal intent, Local-only mode, and Jev-composed decision in CONTEXT.md, so that Specs and code share vocabulary.
59. As a future implementer, I want Out of Scope to exclude OpenJev self-host and chat-written reasons, so that R1 stays on official TypeSafe System One.
60. As a stakeholder, I want this Spec Issue labelled ready-for-agent, so that an AFK agent can implement without re-grilling locks.

## Implementation Decisions

- **Metric.** Optimize and report against **首揭接受率** (first Decision of a meal accepted without needing reroll success framing). Do not redefine success as engagement-through-rerolls.
- **Cloud vendor.** Use TypeSafe managed **System One**; product speech **Jev**. Operator supplies official quota via `TYPESAFE_API_KEY` (or the repo’s chosen unified env name documented in config). No self-hosted OpenJev in R1.
- **Path R1 composition.** knowing-me eligibility + Shortlist → System One final answers → **compose in Go** into one Decision (dish id, mode pool|discovery, reason). System One does not compute cooldown distances or freshness curves.
- **Shortlist.** Top-K ≈ **16** from eligible pool / discovery candidates after knowing-me filters (zeros, OccasionNever, rejection marks, session shown, etc.).
- **Explore slots.** Floor **2** seats reserved for high-freshness / Discovery-oriented candidates; when explore **Noul** is true, raise reserved seats to **4**. This is the anti-island mechanism paired with explore Noul (catalog enrichment is separate).
- **Fan-out.** Single System One request when possible: `Choice(shortlist)` + `Noul(explore)` + `Score(fit to Meal intent / occasion)` + **conditional** `Noul(restriction/safety)` only when restriction signals are present in state.
- **Failure policy.** Ranking / Choice / fit Score path: **fail-open** to local weighted sample (LocalChooser-equivalent over the Shortlist or full local path). Allergy/safety Noul: **fail-closed** (do not serve the risky option; surface safe refusal / local safe path per existing domain rules).
- **Missing key.** No or invalid API key → clear “Jev unavailable” style wire error; Decision assist does not silently pretend. Explicit **Local-only mode** switch, **default OFF**, enables pure knowing-me reveals without System One.
- **Reasons.** Local Chinese templates keyed off System One Score/Choice tiers and existing knowing-me reason priorities. Soft copy only — Spec does not freeze exact strings. **No** second chat LLM for reasons.
- **Meal intent.** Optional Chinese preset tags (English enums in System One / app state) + optional free text. Absence is valid. Passed on Begin/Reroll.
- **Catalog enrichment.** Separate Issue; non-blocking for R1 acceptance.
- **Seams (user-confirmed).**
  - Inject **Chooser** at meal lifecycle `chooseDecision`: adapters **LocalChooser** vs **JevComposedChooser**.
  - Primary regression surface: **HTTP Meal lifecycle** (ADR-0019).
  - Unit-test System One via **jev.Client** fake.
  - **Do not** seam inside `engine.ScorePool`.
- **ADR-0026.** Accepted exception to ADR-0023 zero-egress: optional System One Decision assist only; not a revival of the NIM taste interview.
- **Modules (names, not paths).** Meal lifecycle hosts Chooser; knowing-me **engine** stays pure arithmetic; new **jev** client module for System One HTTP + fake; server config wires key, timeouts, model pin, and Local-only preference persistence (eater preference and/or deploy config — pick one durable place in implementation and document in README/env example).
- **Frontend in scope.** Meal intent UI before/with Begin (and available on Reroll as needed); Local-only switch UI; reuse existing Decision stage reveal (reason + mode). No new chat surface.
- **Wire.** Prefer extending Begin/Reroll request payloads with Meal intent fields and a dedicated error code for Jev-unavailable-when-required; keep Decision response shape (id, dish, mode, reason, budget fields) stable unless a minimal additive observability flag is needed for “composed vs local fallback” (optional, not user-facing hard copy).
- **Security.** API key never shipped to the browser; outbound calls only from Go backend.

## Testing Decisions

- Prefer **external behavior** over internal spies: assert HTTP Meal lifecycle outcomes (state transitions, Decision fields, error codes) and Chooser-visible policies, not ScorePool internals rewritten for Jev.
- **Primary seam:** authenticated HTTP Meal lifecycle tests (ADR-0019 prior art in meal lifecycle / pending-rating HTTP tests) — Begin, Reroll, Accept, abandon, hand-pick still obey budget and gates; missing key + Local-only off → clear error; Local-only on + decision RNG seed → compatible with historical local picks.
- **Engine table tests:** unchanged regression for four-factor scores, relaxation, Discovery weights, ComposeReason baselines — proving arithmetic did not move into System One.
- **jev.Client fake unit tests:** request fan-out shape (Choice + Noul explore + Score fit + conditional safety Noul), timeouts/429/errors, ranking fail-open, safety fail-closed, explore Noul true elevating Explore slots 2→4.
- **JevComposedChooser unit tests:** Shortlist size ≈16, explore seat reservation, composition of final Decision + templated reason, fallback observability.
- **FE:** Meal intent controls submit enums + optional text; Local-only switch default off and persists; stage still renders one Decision with reason/mode (extend existing stage pure tests where cheap).
- Do not require live TypeSafe calls in CI; fake client is mandatory for deterministic AFK runs.

## Out of Scope

- Catalog bulk enrichment / import pipeline (separate Issue; non-blocking).
- Self-hosted OpenJev / local GPU / third-party System One clones as production Decision backend.
- Chat LLM (or any second model) writing free-form Chinese reasons or interview dialogue.
- Reviving NVIDIA NIM taste interview / onboarding routes (ADR-0023 remains; ADR-0026 is not a loophole for interview).
- Changing the four-factor curve formulas themselves except minimal glue if Shortlist construction discovers a conflict.
- Production DNS, secret placement on live hosts, billing/quota purchasing UX beyond documenting env key + official quota assumption.
- Seaming or rewriting selection inside `engine.ScorePool`.
- Multi-eater / household decisions; third-party OAuth; operator console changes unrelated to Local-only/deploy docs.

## Further Notes

- **Grilling locks** encoded above: metric 首揭接受率; Path R1 knowing-me Shortlist → Jev final → Go compose; Top-K≈16; Explore slots floor 2 / Noul true→4; anti-island via forced explore quota + explore Noul; ranking fail-open / safety fail-closed; missing key clear error + Local-only default off; local Chinese reason templates (soft copy); Meal intent tags+text; catalog enrichment separate; fan-out Choice+Noul(explore)+Score(fit)+conditional Noul(restriction); FE Meal intent + Local-only; seams Chooser at chooseDecision, HTTP lifecycle primary, jev.Client fake, no ScorePool seam.
- **Research:** `docs/research/jev-system-one-2026-09.md`, `docs/research/jev-github-integrations-2026-09.md`.
- **Design:** `docs/design/jev-r1-product-tech-2026-09.md`, `docs/design/jev-r1-codebase-seams-2026-09.md` (seams confirmed by user; FE-in-scope supersedes any earlier “zero FE” checklist line in the seams survey).
- **ADRs:** ADR-0022 (knowing-me engine arithmetic stays), ADR-0023 (interview abolished; zero-egress default tempered by ADR-0026), ADR-0019 (Meal lifecycle HTTP primary regression; second adapter allowed), **ADR-0026** (optional System One egress exception).
- **Glossary:** root `CONTEXT.md` terms System One (Jev), Shortlist, Explore slot, Meal intent, Local-only mode, Jev-composed decision.
- Spec body also mirrored at `docs/design/jev-r1-spec-body.md` for local review; GitHub Issue is the agent execution ticket.
