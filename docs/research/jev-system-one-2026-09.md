# Research note: TypeSafe / Jev (System One decision model)

**Date:** 2026-09-22 (CST / Asia/Shanghai)  
**Purpose:** Product architecture decision input for what-to-eat (meal / preference-style decisions).  
**Constraint:** Primary sources only (official docs, launch blog, GitHub/HF repos, privacy policy). Secondary aggregator sites are listed only as discovery breadcrumbs, not as facts.

---

## Executive summary

1. **Jev** is TypeSafe AI’s first **System One** model: `state + typed questions → Choice / Score / Noul` answers with probabilities (and confidence for Choice/Score)—**not** free-form text generation ([docs](https://docs.typesafe.ai/concepts/system-one.md), [launch](https://typesafe.ai/blog/introducing-system-one-models-and-jev)).
2. **Maker:** TypeSafe AI, Inc. Product is a **closed managed SaaS API** (early access / waitlist). Weights are **not** released; no official self-host ([models](https://docs.typesafe.ai/models.md), [coding-agents](https://docs.typesafe.ai/introduction/coding-agents.md)).
3. **Wire API:** `POST https://api.typesafe.ai/v1/systemone` with Bearer `TYPESAFE_API_KEY`; body `{ model, state, questions }` → `{ model, answers, usage }` ([API](https://docs.typesafe.ai/api.md)).
4. **Published economics / speed (vendor-claimed):** input **$0.042 / MTok**, output free; end-to-end **~70–500 ms** (launch); docs also say “most queries ~100 ms”; rate limits **1,200 RPM / 250k tokens/s** (may change) ([launch](https://typesafe.ai/blog/introducing-system-one-models-and-jev), [models](https://docs.typesafe.ai/models.md)).
5. **Explicit non-goals:** not a chat/coding LLM; no text/code generation; weak at math/counting/dates/indirection; text-only input (no image/audio/video yet) ([jaggedness](https://docs.typesafe.ai/model-jaggedness/jev-1.13.md), [coding-agents](https://docs.typesafe.ai/introduction/coding-agents.md)).
6. **Open / local “Jev-shaped” alternatives exist** (HF `openjev/openjev`, GitHub `Mapika/decider`, `ekzhang/openjev-sglang`, `razorback16/openjev`, LLM-shim playgrounds). They claim compatible `POST /v1/systemone` shapes but are **not** TypeSafe Jev; maturity and licenses vary (CC BY-NC vs Apache-2.0 vs MIT).
7. **Privacy (published):** no training on customer Input; Services hosted in the **U.S.**; enterprise **ZDR** via sales ([privacy](https://typesafe.ai/privacy), [legal](https://docs.typesafe.ai/legal.md)). Exact region pinning / EU residency path is **not** fully specified in public docs beyond US hosting.
8. **Architecture fit for what-to-eat:** strong for “pick one cuisine / score preference / yes-no dietary gate” once access exists; poor if you need offline, on-prem, long explanations, or arithmetic over nutrition numbers without code.

---

## Capability boundary table

| Dimension | TypeSafe Jev (official) | Typical open “OpenJev” / Decider stacks |
| --- | --- | --- |
| Product form | Closed SaaS API, early access | Self-host weights + helper, or LLM micro-scorer wrappers |
| Decision contract | Choice (≤255 opts), Score (2–10 levels), Noul (P(yes)) | Often same wire shapes; option caps differ (e.g. OpenJev HF ≤52/pass) |
| Output | Typed answers + probabilities; Choice/Score + `confidence` | Compatible responses claimed; calibration ≠ TypeSafe RLCD |
| Text generation | Explicitly **not** supported | Varies; true System One clones avoid generation; LLM shims still call chat models |
| Hosting | Cloud only (`api.typesafe.ai`); no official self-host | Local GPU / Mac MLX / Modal / Codiv-hosted demos |
| Auth | `TYPESAFE_API_KEY` Bearer | Own keys, or none on demo endpoints |
| Multimodal | Text only (string / JSON / text arrays) | Some open builds add screenshot/DOM (e.g. HF OpenJev, Mapika vision) |
| Offline | No | Yes if you run weights locally |
| License of model | Proprietary product | Apache-2.0 (Decider, razorback16), CC BY-NC (openjev/openjev weights), MIT (some playgrounds) |

Sources: [System One](https://docs.typesafe.ai/concepts/system-one.md), [API](https://docs.typesafe.ai/api.md), [Models](https://docs.typesafe.ai/models.md), [HF openjev](https://huggingface.co/openjev/openjev), [Mapika/decider](https://github.com/Mapika/decider).

---

## API / integration sketch (cited shapes)

### Endpoint and auth

```http
POST https://api.typesafe.ai/v1/systemone
Authorization: Bearer <API_KEY>
Content-Type: application/json
```

Also: `GET https://api.typesafe.ai/v1/models` ([API](https://docs.typesafe.ai/api.md), [Models](https://docs.typesafe.ai/models.md)).

Env var used by official SDKs: `TYPESAFE_API_KEY` ([Quickstart path via docs index](https://docs.typesafe.ai/introduction/quickstart.md) / [Models](https://docs.typesafe.ai/models.md)).  
Python: `typesafe-sdk` · JS/TS: `@typesafe-ai/sdk` ([docs SDK index](https://docs.typesafe.ai/sdk.md)).

### Request shape

```json
{
  "model": "jev-latest",
  "state": "Help! My payouts have been failing for 3 days.",
  "questions": {
    "is_urgent": {
      "type": "noul",
      "instructions": "Does this convey urgency?",
      "criteria": { "true": "Explicitly time-sensitive", "false": "No urgency expressed" }
    },
    "department": {
      "type": "choice",
      "instructions": "Which team should handle this?",
      "criteria": {
        "billing": "Payments, invoicing, refunds",
        "technical": "Bugs, outages, integrations",
        "sales": "Pricing, upgrades, new accounts"
      }
    },
    "frustration": {
      "type": "score",
      "instructions": "How frustrated is the customer?",
      "criteria": ["Calm", "Frustrated", "Very angry"]
    }
  }
}
```

Field rules from official API:

| Field | Role | Documented constraints |
| --- | --- | --- |
| `state` | Content to evaluate | String, JSON object, or array of text; **no** image/audio/video ([State](https://docs.typesafe.ai/concepts/state.md)) |
| `model` | Model selector | `jev-1.13.0`, aliases `jev-latest` / `jev-preview` (both → `jev-1.13.0` as of docs) ([Models](https://docs.typesafe.ai/models.md)) |
| `questions` | Map of id → typed question | Keys are client-chosen; not used in inference ([API](https://docs.typesafe.ai/api.md)) |

### Primitives → answers

| Type | Ask | Returns (official examples) |
| --- | --- | --- |
| **Noul** | Yes/no | `{ "type": "noul", "noul": 0.95 }` — **no** separate `confidence` ([API](https://docs.typesafe.ai/api.md), [Confidence](https://docs.typesafe.ai/confidence.md)) |
| **Choice** | One of named options (max **255**) | `{ "type": "choice", "choice": "...", "probabilities": {...}, "confidence": 0–1 }` |
| **Score** | Position on **2–10** ordered descriptive levels | `{ "type": "score", "score": <weighted mean>, "legend": {...}, "probabilities": {...}, "confidence": 0–1 }` |

Questions in one request are evaluated **in parallel** against the same state ([Introduction](https://docs.typesafe.ai/introduction.md)).

### Example response fragment

```json
{
  "model": "jev-1.13.0",
  "answers": {
    "department": {
      "type": "choice",
      "choice": "billing",
      "probabilities": { "billing": 0.88, "technical": 0.12, "sales": 0.0 },
      "confidence": 0.81
    }
  },
  "usage": { "input_tokens": 318, "output_tokens": 34 }
}
```

([API](https://docs.typesafe.ai/api.md))

### Context budgets (published)

- **64k tokens** per request (state + all questions combined)  
- **32k tokens** for `state` + the single longest question  
([Models](https://docs.typesafe.ai/models.md))

### Errors (HTTP)

`401` auth · `422` validation · `429` rate limit · `529` overloaded ([API](https://docs.typesafe.ai/api.md)).

---

## Latency & cost notes

| Claim | Value | Source | Caveat |
| --- | --- | --- | --- |
| Input price | **$0.042 / million tokens** ($42 / billion) | [Launch blog](https://typesafe.ai/blog/introducing-system-one-models-and-jev), [Models](https://docs.typesafe.ai/models.md) | Vendor list price; sustainability not independently verified |
| Output price | **Free** (“too cheap to meter”) | Same | Output tokens appear in `usage` but bill at $0 |
| E2E latency | **70–500 ms** | [Launch blog](https://typesafe.ai/blog/introducing-system-one-models-and-jev) | Measured from West Coast laptops; service “currently based” there |
| Typical latency (docs) | “Most queries complete in about **100 ms**” | [How to build](https://docs.typesafe.ai/concepts/how-to-build-with-system-one.md) | Not a hard SLA |
| Use-case map | “Frontier intelligence at real-time speeds (**150 ms**)” | [Use-case map](https://docs.typesafe.ai/concepts/use-case-map.md) | Illustrative marketing figure |
| Rate limits | **250,000 tokens/s** · **1,200 requests/min** | [Models](https://docs.typesafe.ai/models.md) | “Adjusting dynamically… can change without notice” |
| High-cardinality Choice | Up to 255; “2 stage-system” may cause occasional slowdown | [Launch blog](https://typesafe.ai/blog/introducing-system-one-models-and-jev) | Vendor nuance |
| Cookbook wall-clock | e.g. structure-recovery pass ~0.32s / 0.51s for many parallel questions | [autoformat cookbook](https://docs.typesafe.ai/cookbooks/autoformat.md) | Cached demo numbers; not our measurement |

**Not independently measured in this research pass:** p50/p99 from Asia/Shanghai, cold-start, or overloaded (`529`) behavior. Do **not** treat 70–500 ms as a contractual guarantee.

---

## Local / open alternatives comparison

> None of these are TypeSafe products. TypeSafe has not released Jev weights ([Models](https://docs.typesafe.ai/models.md) data handling / customizing; community guides echo “no self-host”).

| Project | What it claims | License (as published) | GPU / LLM needs | Offline | Maturity signals (snapshot 2026-09-22) |
| --- | --- | --- | --- | --- | --- |
| [openjev/openjev](https://huggingface.co/openjev/openjev) | Open-weights decision model; `POST /v1/systemone`; letter-logprob readout; text + screenshot; ~80–210 ms on H100 (vendor-measured) | Weights **CC BY-NC 4.0**; helper/serve **Apache-2.0**; “not affiliated with TypeSafe” | Yes (vLLM FP8 on ~80 GB GPU; also MLX Mac builds) | Yes if self-hosted | Full model card + eval tables vs hosted Jev; commercial use restricted by NC |
| [Mapika/decider](https://github.com/Mapika/decider) | Open System One-style family (2B / 35B-A3B / vision); TypeSafe wire format via serve; one-pass typed decisions | **Apache-2.0** | CUDA (~4 GB for 2B bf16); larger for 35B | Yes | Active training docs, HF weights, serve scripts; higher star count among OSS decision clones |
| [ekzhang/openjev-sglang](https://github.com/ekzhang/openjev-sglang) | Jev-compatible API on Qwen via SGLang; Modal deploy | Repo license field empty on GitHub API at fetch time — **verify in-repo** | Yes (B200-class Modal recipe) | Possible if you bring GPUs | Public Modal endpoint pattern; smoke tests for Noul/Choice/Score |
| [razorback16/openjev](https://github.com/razorback16/openjev) | System One server on DiffusionGemma; Codiv hosted demo; SDK-compatible base URL | **Apache-2.0** | GPU or Apple MLX | Yes | Documents `POST /v1/systemone` + chat completions; hosted free tier claims |
| [SiliconLabAI/OpenJev](https://github.com/SiliconLabAI/OpenJev) | Playground: parallel LLM micro-scorers / oneshot JSON / Mapika decider backend | **MIT** | Depends on mode (cloud LLM keys **or** local Decider GPU) | Partial (decider mode) | Young repo; demo UI on `:3001` |
| Many other `*/openjev` forks | Name collision after Jev launch | Mixed / often unset | Varies | Varies | Treat as experimental unless README + license + tests are clear |

**Architectural families (do not conflate):**

1. **True System One clones** — single forward pass / logprob readout over fixed options (HF OpenJev, Mapika Decider).  
2. **LLM micro-scorer shims** — N parallel “P(this option)” chat calls + softmax (SiliconLabAI parallel mode). Latency/cost follow the underlying chat LLM; not TypeSafe RLCD.

---

## What it is NOT good for (explicit non-goals / jagged edges)

From official docs (prefer these over secondary blogs):

| Non-goal / weakness | Official guidance |
| --- | --- |
| Chat, code completion, agent “brain” | “Jev is not a drop-in replacement for the LLM behind Claude Code, Cursor…” ([coding-agents](https://docs.typesafe.ai/introduction/coding-agents.md)) |
| Text / code / explanation generation | “do not write replies, produce code, or generate explanations”; Generation section: chaining Choices for generation “will not work well” ([System One](https://docs.typesafe.ai/concepts/system-one.md), [jaggedness](https://docs.typesafe.ai/model-jaggedness/jev-1.13.md)) |
| Math, counting, numeric precision | “Keep the arithmetic in code”; Score levels “weak in numerical calibration” for reconstructing exact magnitudes ([jaggedness](https://docs.typesafe.ai/model-jaggedness/jev-1.13.md)) |
| Date/time ordering & intervals | Extract parts with Choice; compare in code ([jaggedness](https://docs.typesafe.ai/model-jaggedness/jev-1.13.md)) |
| Heavy indirection / System Two multi-hop | Reduce hops; atomic questions ([jaggedness](https://docs.typesafe.ai/model-jaggedness/jev-1.13.md)) |
| Large noisy state | Filter first; “context rot” ([jaggedness](https://docs.typesafe.ai/model-jaggedness/jev-1.13.md)) |
| Adversarial / prompt-injection-laden state | “state is data… does not treat it as hostile by default” — screen in product code ([jaggedness](https://docs.typesafe.ai/model-jaggedness/jev-1.13.md)) |
| Multimodal raw inputs | Text only for now ([State](https://docs.typesafe.ai/concepts/state.md), [Models](https://docs.typesafe.ai/models.md)) |
| Self-host / on-prem Jev weights | Not offered; customize via state/criteria, not LoRA ([Models](https://docs.typesafe.ai/models.md)) |
| Guaranteed identity between Noul and Choice(yes/no) | Structural invariants not guaranteed ([jaggedness](https://docs.typesafe.ai/model-jaggedness/jev-1.13.md)) |
| Non-English as first-class | English primary; other languages “not equally well” — test yourself ([Models](https://docs.typesafe.ai/models.md)) |

Architecture stance: **AI-powered software** (code owns control flow), **not** autonomous agents that choose their own next action ([How to build](https://docs.typesafe.ai/concepts/how-to-build-with-system-one.md)).

---

## Pricing, keys, privacy / residency (published)

| Topic | Published fact | Source |
| --- | --- | --- |
| Price | $0.042 / MTok input; output free | [Models](https://docs.typesafe.ai/models.md), [Launch](https://typesafe.ai/blog/introducing-system-one-models-and-jev) |
| Access | Closed early access / waitlist → console keys | [Launch](https://typesafe.ai/blog/introducing-system-one-models-and-jev); console referenced as `console.typesafe.ai` in docs skill/agent pages |
| API key | Bearer token; SDKs read `TYPESAFE_API_KEY` | [API](https://docs.typesafe.ai/api.md), [Models](https://docs.typesafe.ai/models.md) |
| Training on customer data | “will not train or fine tune… on your prompts or other Input”; Models: “not trained on customer requests or responses” | [Privacy](https://typesafe.ai/privacy), [Models](https://docs.typesafe.ai/models.md) |
| Hosting / transfers | “Services are hosted in the United States”; EEA/UK users transfer data to U.S. | [Privacy](https://typesafe.ai/privacy) |
| ZDR | Zero data retention for **enterprise**; contact `privacy@typesafe.ai` | [Legal](https://docs.typesafe.ai/legal.md) |
| Fine-tuning | No customer LoRA; same weights for all accounts | [Models](https://docs.typesafe.ai/models.md) |

**Unverified / not found in primary docs:** detailed per-region residency options (EU-only cluster), SOC2/ISO attestations URLs, public SLA, published waitlist duration.

---

## Open questions / unknowns

1. **Access timing:** Waitlist length and eligibility criteria are unpublished; cannot assume production keys for what-to-eat without TypeSafe approval.  
2. **Latency from Asia/Shanghai (UTC+8):** Vendor numbers are West Coast–biased; no official regional latency matrix found.  
3. **Rate-limit stability:** Docs warn limits change dynamically under demand.  
4. **OpenJev ↔ Jev quality gap:** HF card reports OpenJev ~84.0% vs hosted Jev ~85.4% on their 10k text set — **their** methodology, not an independent audit.  
5. **License risk for commercial what-to-eat:** HF OpenJev weights are **CC BY-NC**; Decider Apache-2.0 may be safer for commercial self-host—**legal review still required**.  
6. **Whether TypeSafe offers VPC / private connectivity:** Not found in fetched public docs (enterprise sales path only).  
7. **Exact `confidence` formula for hosted Jev:** Docs say it is a statistic of the distribution but do not publish the closed-form equation on the Confidence page; some open servers publish `1 − H(p)/ln K`—**do not assume** hosted Jev uses the same formula without vendor confirmation.

---

## Primary source URLs used

### TypeSafe / Jev (authoritative)

- https://typesafe.ai/blog/introducing-system-one-models-and-jev  
- https://docs.typesafe.ai/introduction.md  
- https://docs.typesafe.ai/concepts/system-one.md  
- https://docs.typesafe.ai/concepts/state.md  
- https://docs.typesafe.ai/concepts/how-to-build-with-system-one.md  
- https://docs.typesafe.ai/concepts/use-case-map.md  
- https://docs.typesafe.ai/introduction/coding-agents.md  
- https://docs.typesafe.ai/api.md  
- https://docs.typesafe.ai/models.md  
- https://docs.typesafe.ai/confidence.md  
- https://docs.typesafe.ai/model-jaggedness/jev-1.13.md  
- https://docs.typesafe.ai/legal.md  
- https://docs.typesafe.ai/llms.txt  
- https://typesafe.ai/privacy  
- https://github.com/typesafe-ai/skills  

### Open / local alternatives (primary for those projects)

- https://huggingface.co/openjev/openjev  
- https://github.com/Mapika/decider  
- https://github.com/ekzhang/openjev-sglang  
- https://github.com/razorback16/openjev  
- https://github.com/SiliconLabAI/OpenJev  

### Discovery-only (not used as factual authority)

- https://systemonemodels.org/guides/choice-score-noul/ (community mirror of docs; useful for navigation)  
- Various SEO/aggregator “Jev API” pages (jevapi.org, 4sapi, goldie.agency, etc.) — **not cited for claims**

---

## Method notes

- Research performed 2026-09-22 via WebFetch of official docs + `gh`/`curl` for GitHub/HF metadata.  
- No live authenticated calls to `api.typesafe.ai` (no key in this environment).  
- Application code under `/workspace/dev/what-to-eat` was **not** modified beyond adding this research file.
