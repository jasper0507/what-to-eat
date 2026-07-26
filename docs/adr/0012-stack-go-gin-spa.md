# 0012. Stack: Go/Gin API + React SPA (Ant Design)

## Status

Partially superseded: the UI-kit decision (Ant Design) is superseded by [ADR-0024](0024-tailwind-shadcn-ui.md) (2026-07-27, Tailwind v4 + shadcn/ui); the NIM onboarding items are superseded by [ADR-0023](0023-taste-interview-abolished.md). Backend, build tooling, routing, and SQLite decisions remain accepted.

## Context

Greenfield product needs accounts, decision APIs, HowToCook-backed catalog, and server-side NVIDIA NIM for onboarding. Operator prefers **Go + Gin** and a **split frontend/backend**. Operator is **not frontend-fluent** and wants **existing component libraries** rather than hand-rolled UI. Cost and ops stay low (SQLite, no third-party auth).

## Decision

1. **Backend:** **Go + Gin** JSON API — auth (username/password + server session/JWT), candidate pool, decisions, eating records, discovery, pending ratings, NIM proxy for onboarding.
2. **Frontend:** **Vite + React + TypeScript** SPA, responsive for mobile and desktop.
3. **UI kit:** **Ant Design (antd) v5** as the primary component library (Form, Button, Rate, List, Layout, Modal, Message, etc.). Prefer composing antd primitives over custom CSS-heavy widgets. Feeling-labeled taste ratings can use `Rate` / radio cards built from antd components.
4. **Routing:** React Router (or equivalent) for login, home decision, recipe, pool edit, onboarding chat.
5. **Data:** **SQLite** for v1 single-node deploy via the Go service.
6. **Secrets:** NIM API key and auth secrets only on the Go server.
7. **Catalog:** HowToCook ingested/vendored; search and recipe content served by Go.

## Consequences

- Two surfaces behind one reverse proxy (`/api` → Gin, `/` → SPA).
- Chinese-friendly antd docs help a non-frontend operator read examples later.
- UI stays consistent; risk is over-using dense enterprise patterns — keep pages sparse (one primary CTA for Decision).
- antd is desktop-strong; mobile layout must be checked explicitly (fluid width, large tap targets).
