# 0020. Repository layout: 根目录只放产品与契约，教学材料归入 docs/

## Status

Accepted；决定第 3 条（`internal/server` 保持单一扁平包）已被 ADR-0021 取代；
决定第 2 条的 v1 验收教学包已于 2026-07-27 完成使命后整体删除（存档在 git 历史），
`docs/acceptance/` 不复存在，现行验收基准是 `frontend/tests/` 规格与 `docs/copy.md`

## Context

根目录曾同时存放产品代码、运行配置和 v1 验收教学包（`MISSION.md`、
`RESOURCES.md`、`lessons/`、`assets/`、`reference/`）。教学包是一个内部相对
链接自洽的整体，与产品运行无关；散在根目录使新读者难以分辨"什么是产品、
什么是围绕产品的材料"。

同时，`internal/server` 是单一扁平 Go 包（约十个按模块命名的源文件），
存在"要不要拆成多个 Go 包"的疑问。

## Decision

1. 根目录只保留：产品代码（`cmd/`、`internal/`、`frontend/`）、构建与部署
   （`Dockerfile`、`compose.yaml`、`.env.example`、`go.mod`）、以及三份
   根级契约（`README.md`、`CONTEXT.md`、`AGENTS.md`——后两者的位置是
   agent 工具链的约定，见 `docs/agents/domain.md`）。
2. v1 验收教学包整体平移至 `docs/acceptance/`，保留其内部
   `lessons/`、`assets/`、`reference/` 结构，使包内相对链接不变。
3. `internal/server` **保持单一扁平包**。ADR-0019 刻意把 handler 与各
   concrete module 同居一包共享未导出类型；按 Go 惯例，包在出现真实的
   第二个包级消费者之前不拆。文件即模块（`auth.go`、`candidate_pool.go`、
   `catalog.go`、`meal_lifecycle.go`、`onboarding*.go`、`schema.go`、
   `wire_codes.go`），这已经是该规模下的组织单位。

## Considered Options

- 按模块拆 `internal/server` 为多个 Go 包：被拒。会迫使大量内部标识符
  导出、切断共享的响应类型与错误写出口，且没有任何包级第二消费者——
  正是 ADR-0019 所拒绝的假设性接缝，只是换成了目录形态。
- 教学包并入 `README`/`docs` 散页：被拒。教学包是自洽整体，拆散会断链。

## Consequences

- 新读者从根目录即可分辨产品与材料；教学包整体可移动、可归档。
- 未来新增教学/验收材料进 `docs/acceptance/`，新增 ADR 进 `docs/adr/`。
- 若某后端模块出现真实的第二个包级消费者（例如另一个二进制复用
  Catalog），届时再为该模块单独立包，并以新 ADR 记录。
