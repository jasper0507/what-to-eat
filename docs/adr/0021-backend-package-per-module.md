# 0021. 后端按模块拆分为多个 Go 包

## Status

Accepted（取代 ADR-0020 决定第 3 条"internal/server 保持单一扁平包"）

## Context

操作者要求把后端从单一大目录拆开以提升可维护性。此前 `internal/server`
以"文件即模块"组织约十个源文件；ADR-0019 的深模块设计（concrete 类型、
handler 与模块同居、五行为 HTTP seam）依赖同包共享未导出类型。

## Decision

按既有的真实模块边界拆包，`internal/server` 退为 HTTP adapter 兼
composition root：

- `internal/catalog` — source_path 分类学、Dish/Recipe 视图、导入与搜索
- `internal/pool` — Candidate pool 的 membership、weight 合法域与
  rejection mark 写侧语义
- `internal/account` — 会话模块（注册、登录限流、签发与校验）
- `internal/meal` — ADR-0019 的 Meal lifecycle 深模块，五行为原样整体平移
- `internal/onboarding` — 访谈模块与其私有 NIM port/adapters
- `internal/schema` — 有序迁移台账（`Migrate` 单一导出）
- `internal/server` — App、路由、全部 handler、wire 错误码目录、中间件；
  `Config` 上以类型别名（`DiscoveryConfig`、`NIMConfig`）转发模块配置

约束：模块仍是 concrete 类型，除既有 NIM port 外不新增 Go interface，
不引入 per-table repository；测试继续只打 `internal/server` 的公共 HTTP
seam——本次拆分的验收标准即"外部测试文件零改动、全部通过"。

## Consequences

- 模块接缝由包边界强制：各包的导出面就是其 interface，误触内部实现
  会在编译期被拒。
- 依赖方向固定为 catalog ← pool ← {meal, onboarding}，server 汇总；
  新代码不得反向依赖 server。
- 每个模块的响应视图（JSON 标签）随模块走，adapter 直接序列化模块结果，
  与 ADR-0019 "结果暴露领域标识"一致。
- ADR-0020 的仓库顶层布局决定（第 1、2 条）不受影响，继续有效。
