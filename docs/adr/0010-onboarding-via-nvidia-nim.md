# 0010. Onboarding interview via product-hosted NVIDIA NIM

## Status

Accepted

## Context

First-run needs an AI interview to build the initial candidate pool and preference weights quickly. Options: product-hosted LLM API, user-supplied API keys, or non-AI questionnaire for v1. The operator already has a project-specific **NVIDIA NIM** API key.

Daily decisions remain algorithmic (pool / discovery / anti-repeat) and must not require an LLM call per meal.

## Decision

1. **Onboarding interview** uses a **product-hosted** LLM backend: **NVIDIA NIM** (server-side only).
2. The API key is **never** shipped to the client; it lives in server env / secret store.
3. NIM is **not** used for the everyday Decision path (pool pick / discovery pressure / weighted sample).
4. Rate-limit and abuse controls sit on the authenticated account (interview is post-login).

## Consequences

- Backend is mandatory for onboarding (already true with accounts).
- Dependency on NIM availability, model choice, pricing, and region.
- Need a graceful path if NIM fails mid-interview (retry / resume / fall back to manual pool search).
- Key rotation and `.env` / secret management are part of deploy checklist; keys must not be committed.
