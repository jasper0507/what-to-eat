# 0026. Optional System One egress for Decision assist

Status: accepted

Date: 2026-09-22

## Context

ADR-0023 abolished the NVIDIA NIM taste interview and set the product runtime to zero outbound LLM / zero egress beyond ordinary self-host needs. That red line must not be read as forbidding every future cloud call. Product wants an optional TypeSafe **System One (Jev)** assist on the Decision path: knowing-me still owns eligibility, freshness, and Shortlist arithmetic; System One answers typed Choice / Score / Noul; Go composes the final Decision.

## Decision

1. **Narrow exception to ADR-0023.** Optional outbound HTTP is allowed **only** for TypeSafe System One used as Decision assist. This does **not** revive the taste interview, NIM onboarding, chat LLMs, or any second model that writes free-form Chinese reasons.
2. **Default without key / Local-only.** Missing or invalid `TYPESAFE_API_KEY` means System One is unavailable: surface a clear error. Reveals without System One require an explicit **Local-only mode** switch (default **off**). Operators who want a pure local demo turn that switch on; they do not get a silent local path while believing Jev is configured.
3. **Chooser as second adapter.** Inject a Chooser at meal lifecycle `chooseDecision`: `LocalChooser` (today's knowing-me path) vs `JevComposedChooser` (Shortlist → System One fan-out → Go composition). ADR-0019 already allows a second real adapter when one appears. Do **not** open a seam inside `engine.ScorePool`; four-factor arithmetic stays in the knowing-me engine.
4. **Failure policy.** Ranking / Choice / fit Score failures **fail-open** to local weighted sample from the Shortlist (or equivalent local path). Allergy / safety Noul failures **fail-closed**. Reasons stay local Chinese templates driven by System One tiers — soft copy, no hard-locked strings, no chat LLM.

## Consequences

- ADR-0023's "do not resurrect interview" remains in force; readers who see egress must check this ADR, not assume NIM is back.
- Deployments without a key stay zero-egress if Local-only is off (product refuses cloud Decision) or run local knowing-me only when Local-only is on.
- HTTP Meal lifecycle remains the primary regression surface; `jev.Client` is faked in unit tests.
- Catalog enrichment and self-hosted OpenJev remain out of this decision.
