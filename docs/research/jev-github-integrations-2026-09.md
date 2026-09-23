# Research note: GitHub product integrations of TypeSafe Jev / System One

**Date:** 2026-09-22 (CST / Asia/Shanghai)  
**Purpose:** How *real applications* wire Choice / Score / Noul into products — patterns for combining Jev with what-to-eat’s existing multiplicative knowing-me engine — not another API clone survey.  
**Companion:** [`jev-system-one-2026-09.md`](./jev-system-one-2026-09.md) (API, economics, jaggedness). Do not redo TypeSafe docs from scratch here.  
**Constraint:** Primary sources only (official docs/cookbooks/skills, and GitHub repos that call `api.typesafe.ai` or `@typesafe-ai/sdk` / `typesafe-sdk`). Awesome-lists and aggregator blogs are discovery only.

---

## Executive summary

1. **Jev is already embedded in shipped product code**, not only demos: email triage (Inbox Zero), RAG rerank (Volcengine OpenViking, Hippo memory), Composio tool routing, CopilotKit generative UI, LangChain classifier, LiteLLM guardrails/complexity router, SiYuan decision config, hedge-fund adapters, news threat classification (worldmonitor), Claude-Code mods (skill suggestion / model router / guardrails).
2. **Almost nobody has shipped a meal / cuisine / restaurant personalization app on real Jev.** Public “what to eat” repos found in discovery use OpenAI/LLM agents, not `api.typesafe.ai`. Closest analogues for ranking + personalization are: **prefilter → Noul/Score rerank**, **composite Score dimensions + code weights**, **Choice shortlist + Noul gates**, and **CopilotKit’s workspace picker** (candidate Score + panel Choice).
3. **Official integration doctrine** (skills + how-to-build + cookbooks): keep control flow in code; fan out atomic questions; combine with *your* weights/thresholds; gate on confidence / Noul means; escalate when uncertain. Thresholds in cookbooks are **examples to tune**, not universal constants ([skill](https://github.com/typesafe-ai/skills/blob/main/skills/typesafe-ai/SKILL.md), [how-to-build](https://docs.typesafe.ai/concepts/how-to-build-with-system-one.md)).
4. **Best fit for what-to-eat:** treat knowing-me (喜爱 × 新鲜感 × 场合 × 本顿否决) as the **deterministic prefilter / base weight**, then optionally layer Jev for *semantic* judgments code cannot express (mood/occasion soft fit, dietary gate from free text, discovery “does this resemble what I love?”, confidence-gated “ask the eater”). Do **not** replace arithmetic freshness / cooldown with Score (documented jagged edge: weak numeric calibration).
5. **Do not copy:** OpenJev / Von / Decider / SiliconLabAI parallel LLM micro-scorers / `typesafe-ai/system-one-adapter-python` (official *LLM shim* that mimics the SDK) / playgrounds that only speak the wire format. They are not hosted Jev.

---

## 1. Repos that call real TypeSafe (apps / SDKs / demos)

Grouped by *product role*, not star count. URLs are primary.

### Official TypeSafe surface

| Asset | Role | URL |
| --- | --- | --- |
| `typesafe-ai/skills` | Agent skill: read live docs, fan-out, confidence, pattern index | https://github.com/typesafe-ai/skills |
| `typesafe-ai/typesafe-sdk-js` | Official `@typesafe-ai/sdk` | https://github.com/typesafe-ai/typesafe-sdk-js |
| `typesafe-ai/typesafe-sdk-python` | Official `typesafe-sdk` | https://github.com/typesafe-ai/typesafe-sdk-python |
| Cookbooks index | Skill suggestion, rerank, guardrails, parallel questions, … | https://docs.typesafe.ai/cookbooks.md |
| Patterns | Fan-out, confidence routing, composite scoring, intent routing | https://docs.typesafe.ai/patterns.md |

### Product / framework integrations (call `api.typesafe.ai` or official SDK)

| Repo | What it does with Jev | Integration sketch |
| --- | --- | --- |
| [elie222/inbox-zero](https://github.com/elie222/inbox-zero) | Email decision model via raw `POST …/v1/systemone` | Zod-validated Choice/Noul; maps `yesNo` ↔ `noul`; 30s timeout; Bearer key server-side ([`typesafe.ts`](https://github.com/elie222/inbox-zero/blob/main/apps/web/utils/decision-model/typesafe.ts)) |
| [volcengine/OpenViking](https://github.com/volcengine/OpenViking) | RAG **rerank**: one Noul per candidate | `state={query, candidate_documents[]}`; `relevance_i` Noul → yes-prob as score; on failure return `None` → fall back to vector scores ([`jev_rerank.py`](https://github.com/volcengine/OpenViking/blob/main/openviking/models/rerank/jev_rerank.py)) |
| [ComposioHQ/composio](https://github.com/ComposioHQ/composio) `@composio/typesafe` | Tool pick + closed-set arg bind; abstain / partial / call | Compiles tools → questions; thresholds `routing=0.6`, `gate=0.3`, `argument=0.6`; destructive tools floor 0.9; pin `jev-1.13.0` in prod ([README](https://github.com/ComposioHQ/composio/blob/main/ts/packages/providers/typesafe/README.md)) |
| [CopilotKit/CopilotKit](https://github.com/CopilotKit/CopilotKit) cookbook | Generative UI: Choice(panel) + Score(per candidate) | One `systemOne` call; sort Scores in code; `agent` Choice → LLM fallback ([jev-generative-ui](https://github.com/CopilotKit/CopilotKit/blob/main/showcase/shell-docs/src/content/docs/cookbook/jev-generative-ui.mdx)) |
| [kitfunso/hippo-memory](https://github.com/kitfunso/hippo-memory) | Opt-in memory **reranker** | Local BM25/cross-encoder first; `--reranker jev` lifts R@1; fall back when key missing (README receipts) |
| [koala73/worldmonitor](https://github.com/koala73/worldmonitor) | Headline threat Choice (level + category) | Batched multi-headline state `h0..hN`; contrastive criteria; sum `P(critical)+P(high)` as alert score; skip non-Latin titles ([`jev-classify.js`](https://github.com/koala73/worldmonitor/blob/main/shared/jev-classify.js)) |
| [langchain-ai/langchain](https://github.com/langchain-ai/langchain) `langchain_typesafe` | Runnable classifier | Multi question types in one invoke; preserves probs/confidence ([`classifier.py`](https://github.com/langchain-ai/langchain/blob/master/libs/partners/typesafe/langchain_typesafe/classifier.py)) |
| [vercel/ai](https://github.com/vercel/ai) `packages/typesafe-ai` | AI SDK provider for System One | Default base `https://api.typesafe.ai/v1` |
| [BerriAI/litellm](https://github.com/BerriAI/litellm) | Guardrails + complexity router | TypeSafe hook / router strategy |
| [siyuan-note/siyuan](https://github.com/siyuan-note/siyuan) | Note app **Decision** config | Go struct defaults Endpoint `…/v1/systemone`, model `jev-latest`, Timeout 30 ([`ai.go`](https://github.com/siyuan-note/siyuan/blob/master/kernel/conf/ai.go)) — relevant as a **Go + optional Jev** product shape |
| [virattt/ai-hedge-fund](https://github.com/virattt/ai-hedge-fund) | Native Jev adapter with retries | Retry `429,500,502,503,504,529`; honor `Retry-After`; redact key from diagnostics ([`client.py`](https://github.com/virattt/ai-hedge-fund/blob/main/hedge_fund/llm/client.py)) |
| [davila7/claude-code-templates](https://github.com/davila7/claude-code-templates) | Mods: skill-suggestion, model-router, guardrails | HTTP (no SDK in mod sandbox); dual backend TypeSafe vs Vercel Gateway; confidence-gated |
| [trycua/cua](https://github.com/trycua/cua) `jev-use` example | Bounded action Choice from browser snapshot | `@typesafe-ai/sdk` + code-built candidate list |
| [Nancy-Chauhan/hearth-jev-rental-search](https://github.com/Nancy-Chauhan/hearth-jev-rental-search) | Multi-source rental search | Browser automation + Jev decisions (fan-out); closest “catalog pick” consumer product found |
| [samchon/typia](https://github.com/samchon/typia) `@typia/jev` | Compile TypeScript types → Jev questions | SDK client + `toJevQuestions` |

### Provider / gateway wrappers (not full apps, but production wiring)

OpenRouter SystemOne SDK, Pydantic-AI TypeSafeModel, Effect `packages/ai/typesafe`, Bifrost, Maximhq, RubyLLM, Rig (`rig-typesafeai`), Spring AI community Java SDK, etc. Useful for transport; thin on ranking UX.

---

## 2. Patterns for combining System One with an existing rule/score engine

Mapped to what-to-eat’s knowing-me kernel (`Score = 喜爱 × 新鲜感 × 场合 × 本顿否决`, ADR-0022).

### Pattern A — Deterministic prefilter → Choice (shortlist winner)

**Seen in:** Official [skill_suggestion](https://docs.typesafe.ai/cookbooks/skill_suggestion.md) (wide Choice over 182 → top-3 → second Choice); Composio `shortlistTools`; Hippo/OpenViking (BM25/vector first).

**what-to-eat mapping:**

1. Code: multiply knowing-me factors; drop zeros (cooldown / session veto / occasion-forbidden buckets).
2. Take top-K (e.g. 5–30) by weight or by weighted sample candidates.
3. Jev **Choice** over those dish IDs with criteria = taste-profile blurbs / eater notes / “why this dish”.
4. Optional second pass with richer criteria (same as skill cookbook Call 2).

**Why:** Choice is *relative* (probs sum to 1 even if all options are mediocre). Never use Choice alone as “should we serve food at all.”

### Pattern B — Score as multiplier / soft feature (composite scoring)

**Seen in:** Official [composite-scoring](https://docs.typesafe.ai/patterns/composite-scoring.md); CopilotKit per-candidate Score; how-to-build weighted Noul sum.

**what-to-eat mapping:**

Keep knowing-me as base. Optionally multiply (or blend) by a normalized Jev Score:

```
final ∝ knowing_me_weight × soft_fit
soft_fit = (jev_score / max_level)   # or use probabilities as features
```

Example Score dimensions (ask in **one** fan-out request per reveal):

- `mood_fit` — how well dish matches stated mood / free-text craving  
- `heaviness_fit` — lightness vs heavy for this occasion  
- `novelty_appeal` — exploratory vs comfort (for Discovery arm)

**Do not** ask Jev to recompute “days since last eaten” or calorie arithmetic — keep that in Go ([jaggedness](https://docs.typesafe.ai/model-jaggedness/jev-1.13.md)).

### Pattern C — Noul as hard/soft gate

**Seen in:** Skill suggestion gate Nouls (`GATE_THRESHOLD=0.30` mean); Composio action gate `0.3`; OpenViking relevance Noul; guardrails hazard Nouls; how-to-build spam_risk weighted Nouls.

**what-to-eat mapping:**

| Gate | Noul question | Use |
| --- | --- | --- |
| Dietary / allergy | “Would this dish violate stated restrictions?” | If noul ≥ τ → force veto (×0) |
| “Still want a suggest?” | “Does the eater want a new suggestion vs stop?” | Below τ → stop / hand-pick path |
| Discovery fitness | “Does this out-of-pool dish resemble the eater’s loves?” | Below τ → drop Discovery candidate |
| Absolute “any good?” | Per shortlist dish “Would the eater accept this tonight?” | If *all* low → fall back to knowing-me only |

Remember: **Noul has no separate confidence**; near 0.5 means yes≈no, not “medium intensity” ([skill](https://raw.githubusercontent.com/typesafe-ai/skills/main/skills/typesafe-ai/SKILL.md)).

### Pattern D — Confidence threshold (Choice/Score only)

**Seen in:** [confidence-routing](https://docs.typesafe.ai/patterns/confidence-routing.md); Composio `routing`/`argument` thresholds; Inbox Zero / LangChain preserve confidence for app policy; worldmonitor drops malformed answers to fallback.

**what-to-eat mapping:**

```
if choice.confidence < 0.6:  # example — tune on your logs
    // fall back to pure knowing-me weighted sample
else:
    // bias sample toward choice winner, or present as top pick
```

Risk-tiered thresholds (banking cookbook): low-stakes suggest @0.6; high-stakes dietary auto-block @0.85+ or always confirm.

### Pattern E — Speculative fan-out + code composition

**Seen in:** [how-to-build](https://docs.typesafe.ai/concepts/how-to-build-with-system-one.md) ticket triage; [fan-out pattern](https://docs.typesafe.ai/patterns/fan-out.md); worldmonitor level+category together; CopilotKit control+Scores.

One request, many questions; **code** decides which answers matter. For what-to-eat: pack `mood_fit`, dietary Nouls, and a Choice over shortlist into a single reveal call (~100 ms class latency claimed).

### Pattern F — Fail-open / fail-closed by product risk

**Seen in:** OpenViking (`None` → vector fallback); Hippo (Jev opt-in); Composio (API errors throw — never fake `abstain`); worldmonitor (invalid answer → heuristic fallback); hedge-fund (retry then raise).

**what-to-eat recommendation:** meal suggestions should **fail open** to knowing-me (never blank the UX on 429/529). Dietary/allergy gates should **fail closed** or force human confirmation if Jev is down.

### Pattern G — Retries and timeouts

**Official SDK:** Python `RetryPolicy` defaults `max_retries=2`, statuses `{408,429}+5xx`, respect `Retry-After`, total budget ~30s ([retries](https://docs.typesafe.ai/sdk/python/api/retries.md)). JS SDK merges per-call retry overrides ([typesafe-sdk-js](https://github.com/typesafe-ai/typesafe-sdk-js)).

**In the wild:** hedge-fund retries `{429,500,502,503,504,529}` once with `Retry-After`; Inbox Zero uses `AbortSignal.timeout(30_000)` without retry (app-level).

For Go what-to-eat: prefer a small client with 1–2 retries on 429/529, short timeout (e.g. 1–2 s on the hot reveal path) so the local engine remains responsive.

### Pattern H — Structured state + contrastive criteria

**Seen in:** Official how-to-build nested JSON + `` `path` `` references; worldmonitor situation-described levels (avoids “10+ killed” counting); Composio `{request, context}` split so context cannot hijack routing.

**what-to-eat:** `state = { eater_profile, occasion_hour_bucket, shortlist:[{id,name,taste_profile,affinity_tier}], session_vetoes, free_text_mood? }` — never dump whole catalog.

---

## 3. Recommendation / ranking / personalization apps

| Project | Ranking shape | Personalization? | Meal-like? |
| --- | --- | --- | --- |
| Official rerank cookbook | BM25 → Noul per pair | Query-conditioned | No (legal passages) |
| OpenViking `JevRerankClient` | Vector/BM25 → Noul batch | Query vs docs | No |
| Hippo memory | BM25 → opt-in Jev rerank | Memory relevance | No |
| Skill suggestion cookbook | Choice rank + Noul gate + rerank | Turn context | No (skills roster) |
| Composite scoring pattern | Multi Score → weighted sum | Role-specific weights | Closest *scoring* analogue |
| CopilotKit workspace picker | Score per room + Choice panel | Message + guidance | **Closest UX analogue** (pick among candidates) |
| Hearth rental search | Fan-out decisions over listings | Search criteria | Catalog shortlist, not meals |
| Composio shortlistTools | Choice over tools | Request text | Tool roster, not food |
| Inbox Zero | Email classify/route | Mailbox rules | No |

**Gap statement:** As of 2026-09-22 search across `api.typesafe.ai` / `@typesafe-ai/sdk` callers and web discovery, **no primary-source GitHub product was found that ships meal / cuisine / restaurant recommendation on hosted Jev**. Public “What2Eat” style repos use generative LLMs. what-to-eat would be early in that niche if it wires knowing-me ⊕ Jev carefully.

---

## 4. Official cookbooks, skills, sample patterns (with citations)

### Agent skill — `typesafe-ai/skills`

- Install: `claude plugin marketplace add typesafe-ai/skills` or `npx skills add typesafe-ai/skills`  
- Source of truth: live docs via [llms.txt](https://docs.typesafe.ai/llms.txt); skill warns agents not to invent fields or ask one question per call.  
- Raw: https://raw.githubusercontent.com/typesafe-ai/skills/main/skills/typesafe-ai/SKILL.md  

Key directives quoted in spirit: route/fill args; select don’t generate; retrieve then judge; composite scores as reusable features; verify and escalate; keep credentials server-side.

### Cookbooks most relevant to what-to-eat

| Cookbook | Pattern to steal | URL |
| --- | --- | --- |
| Skill suggestion | Prefilter roster → Choice + gate Nouls → shortlist rerank + fits Nouls; thresholds 0.30 | https://docs.typesafe.ai/cookbooks/skill_suggestion.md |
| Re-ranking | Cheap shortlist → Noul relevance score → sort | https://docs.typesafe.ai/cookbooks/rerank_typesafe.md |
| Parallel questions | Batch many questions / one state (cost & latency) | https://docs.typesafe.ai/cookbooks/parallel_questions.md |
| Guardrails | Hazard Nouls + severity Score; pass/review/block | https://docs.typesafe.ai/cookbooks/llm_guardrails.md |
| Classifying RAG passages | Score/Noul filter before expensive model | https://docs.typesafe.ai/cookbooks/classifying_rag_passages.md |
| Classification using confidence | Confidence → broaden/abstain | https://docs.typesafe.ai/cookbooks/classification_using_confidence.md |
| Function calling | Closed-set args via Choice; open text stays in code | https://docs.typesafe.ai/cookbooks/function_calling.md |

### Patterns (architecture, not notebooks)

| Pattern | One-liner | URL |
| --- | --- | --- |
| Speculative fan-out | Ask speculative questions now; code ignores irrelevant answers | https://docs.typesafe.ai/patterns/fan-out.md |
| Confidence-gated routing | Answer = what; confidence = whether | https://docs.typesafe.ai/patterns/confidence-routing.md |
| Composite scoring | Atomic Scores + **your** weights | https://docs.typesafe.ai/patterns/composite-scoring.md |
| Intent routing | Route to code / specialist LLM / human | https://docs.typesafe.ai/patterns/intent-routing.md |

### Illustrative code shapes (from primary sources)

**Skill suggestion gate + Choice (official cookbook):**

```python
# Call 1: rank all options + absolute “need anything?” nouls
questions = {
    "which": Choice(instructions=..., criteria={name: desc for ...}),
}
for key, text in GATE_QUESTIONS.items():
    questions[f"gate::{key}"] = Noul(instructions=text)
# if mean(oriented_nouls) < 0.30: suggest nothing
# else: Call 2 over top-3 with fits::{name} Nouls; drop if max(fits) < 0.30
```

Source: https://docs.typesafe.ai/cookbooks/skill_suggestion.md  

**OpenViking Noul-as-rerank-score:**

```python
state = {"query": query, "candidate_documents": documents}
questions = {
    f"relevance_{i}": {
        "type": "noul",
        "instructions": {"candidate_index": i, "question": "... match query?"},
        "criteria": {"true": "...", "false": "..."},
    }
    for i in range(len(documents))
}
# sort by answers[f"relevance_{i}"].noul
```

Source: https://github.com/volcengine/OpenViking/blob/main/openviking/models/rerank/jev_rerank.py  

**CopilotKit Choice + per-candidate Score:**

```ts
questions.control = choice("Choose the useful next control...", {
  clarification: "...", comparison: "...", agent: "...",
});
for (const c of candidates) {
  questions[`fit_${c.id}`] = score(`How well does candidate ${c.id} fit...?`,
    ["Poor fit", "Unclear fit", "Good fit", "Strong fit"]);
}
// sort candidates by score; if control === "agent" → LLM fallback
```

Source: https://github.com/CopilotKit/CopilotKit/blob/main/showcase/shell-docs/src/content/docs/cookbook/jev-generative-ui.mdx  

**Composio threshold table (productized gating):** routing 0.6 (0.9 destructive), gate 0.3, argument 0.6; pin full model id in production.  
Source: https://github.com/ComposioHQ/composio/blob/main/ts/packages/providers/typesafe/README.md  

---

## 5. What NOT to copy

| Avoid | Why | Examples |
| --- | --- | --- |
| Open-weight “OpenJev” / Decider / Von servers | Different calibration, licenses (often NC), not TypeSafe RLCD | `openjev/openjev`, `Mapika/decider`, `wfzyx/von` |
| LLM micro-scorer playgrounds | N parallel chat calls + softmax; latency/cost ≠ Jev | `SiliconLabAI/OpenJev` parallel mode |
| Official LLM adapter as “Jev” | Explicitly “Drop-in TypeSafeClient replacement **backed by LLM APIs**” | https://github.com/typesafe-ai/system-one-adapter-python |
| Wire-compatible demos without TypeSafe key path | Prove UX only; not production calibration | Various Codiv / Modal hosted clones |
| Using Choice as absolute quality | Relative distribution always picks a winner | Skill cookbook warns; use Noul gates |
| Asking Jev to do math / cooldown counts | Documented jaggedness | Keep knowing-me freshness in Go |
| Client-side API keys in PWA | Skill: keep credentials server-side | what-to-eat should proxy via Go backend |
| Copying cookbook thresholds blindly | “examples to evaluate, not universal rules” | Tune on your accept/reroll logs |

---

## Recommended combination sketch for what-to-eat (research only — no code change)

```
Reveal request
  │
  ├─[Go] knowing-me weights (喜爱 × 新鲜感 × 场合 × 本顿否决)
  ├─[Go] hard filters (occasion-forbidden buckets, session veto)
  ├─[Go] take shortlist Top-K / discovery candidates
  │
  ├─[optional Jev fan-out] state = {profile, shortlist, mood?, hour_bucket}
  │     Choice: which of shortlist fits now?          → bias / present
  │     Score:  mood_fit / heaviness (per dish or once) → × soft multiplier
  │     Noul:   dietary_violation / discovery_fit     → gate
  │     if confidence < τ or timeout/5xx → ignore Jev, pure knowing-me
  │
  └─[Go] weighted sample + reason strings (local factors + optional Jev labels)
```

Phased rollout that matches how mature repos ship:

1. **Shadow mode** — log Jev answers beside knowing-me picks; no UX change (oh-my-claudecode pattern: heuristic twin + Jev).  
2. **Soft rerank** — multiply weights by Score/Noul; keep sample random.  
3. **Gated Choice** — only override when confidence ≥ τ.  
4. Never remove offline-capable knowing-me path (PWA / API quota / 529).

---

## Top 5 repos (why each matters for what-to-eat)

1. **[typesafe-ai/skills](https://github.com/typesafe-ai/skills)** — Canonical “how to think” + links to live cookbooks; install before any agent-written integration.  
2. **[volcengine/OpenViking](https://github.com/volcengine/OpenViking) (`jev_rerank.py`)** — Clearest production **shortlist → Noul score → fall back to local rank** pattern (maps to knowing-me prefilter).  
3. **[CopilotKit/CopilotKit](https://github.com/CopilotKit/CopilotKit) (jev-generative-ui)** — Closest **candidate Score + control Choice** UX to “pick a dish / ask a clarifying occasion question.”  
4. **[ComposioHQ/composio](https://github.com/ComposioHQ/composio) (`@composio/typesafe`)** — Battle-tested **threshold table, abstain vs error, pin model version, destructive floors** — copy the *policy* shape, not tools.  
5. **[elie222/inbox-zero](https://github.com/elie222/inbox-zero)** — Minimal **Go-adjacent lesson for a product backend**: raw HTTPS + schema validation + timeout, no SDK required (handy for Go+SQLite stack).

Honorable: `kitfunso/hippo-memory` (opt-in rerank receipts), `koala73/worldmonitor` (batched Choice + fallback), `siyuan-note/siyuan` (Go Decision config defaults), official skill_suggestion cookbook (two-pass rank+gate).

---

## Gaps

- **No meal-style personalization reference implementation** on hosted Jev found in primary GitHub callers (2026-09-22).  
- Few public **Go** clients beyond config stubs (SiYuan) and community experiments — expect to write a thin `POST /v1/systemone` client (Inbox Zero / OpenViking style) rather than import an official Go SDK.  
- Ranking apps overwhelmingly do **retrieve → Noul/Score**, not end-to-end generative recommenders — aligns with TypeSafe doctrine (select, don’t generate).  
- Thresholds and confidence formulas are application-specific; no vendor-published meal-domain calibration.  
- Latency from Asia/Shanghai and PWA cold paths still unmeasured here (see companion note).

---

## Method

- Read companion note `jev-system-one-2026-09.md`; fetched official cookbooks/patterns/skills/retries/confidence-routing/composite-scoring/how-to-build.  
- `gh search code` / GitHub MCP `search_code` for `api.typesafe.ai`, `@typesafe-ai/sdk`, `jev-latest`; inspected source files via GitHub Contents API / raw.githubusercontent.com.  
- WebSearch for meal/food/restaurant + Jev — only LLM-based What2Eat apps, not TypeSafe callers.  
- No live calls to `api.typesafe.ai`; no what-to-eat application code modified; no git push; no production contact.

---

## Primary URL index

- https://docs.typesafe.ai/llms.txt  
- https://docs.typesafe.ai/concepts/how-to-build-with-system-one.md  
- https://docs.typesafe.ai/cookbooks/skill_suggestion.md  
- https://docs.typesafe.ai/cookbooks/rerank_typesafe.md  
- https://docs.typesafe.ai/patterns/composite-scoring.md  
- https://docs.typesafe.ai/patterns/confidence-routing.md  
- https://docs.typesafe.ai/sdk/python/api/retries.md  
- https://github.com/typesafe-ai/skills  
- https://github.com/volcengine/OpenViking/blob/main/openviking/models/rerank/jev_rerank.py  
- https://github.com/CopilotKit/CopilotKit/blob/main/showcase/shell-docs/src/content/docs/cookbook/jev-generative-ui.mdx  
- https://github.com/ComposioHQ/composio/blob/main/ts/packages/providers/typesafe/README.md  
- https://github.com/elie222/inbox-zero/blob/main/apps/web/utils/decision-model/typesafe.ts  
- https://github.com/koala73/worldmonitor/blob/main/shared/jev-classify.js  
- https://github.com/virattt/ai-hedge-fund/blob/main/hedge_fund/llm/client.py  
