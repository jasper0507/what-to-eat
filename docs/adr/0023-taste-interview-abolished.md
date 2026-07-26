# 0023. 废止 AI 口味访谈，入池只留起步包与手工挑菜

日期：2026-07-27
状态：已接受（取代 ADR-0010；redesign-brief §六修正案的正式化）

## 背景

ADR-0010 决定用产品托管的 NVIDIA NIM 做 onboarding 访谈，v1 已实现全套：
`/api/onboarding/*` 四条路由、`internal/onboarding` 模块（访谈状态机 + NIM
port/adapter）、`onboarding_interviews`/`onboarding_messages` 两张表、五个
专属 wire 错误码，以及前端访谈页。

段 2 真机验收时用户口头决议：**整个 AI 口味访谈功能砍掉**。理由：新旅程里
空池欢迎已被起步包（14 道经典菜当场落档）与手工挑菜两条路覆盖，访谈作为第三
条入池路径不再有位置；砍掉后每餐决策路径本就无 LLM，产品从此零 LLM 依赖，
NVIDIA_API_KEY 等外部秘密与可用性风险一并消失。前端访谈界面已在段 2
（`c8cadf3`）随 antd 退役一起移除，后端遂成孤儿代码。

## 决定

1. 访谈废止是**永久决议**，列入 brief §七级别的勿复活红线。
2. 后端整块拆除：`/api/onboarding/*` 路由与 handler、`internal/onboarding`
   包、scripted-NIM 测试脚手架。
3. wire 契约收缩：`nim_unavailable`、`interview_limit_reached`、
   `retry_required`、`retry_unavailable`、`interview_finished` 五码从
   `wire_codes.go` 与前端 `ApiErrorCode` union 同步删除。
4. schema 追加「Interview retirement」迁移：`DROP TABLE onboarding_messages`、
   `DROP TABLE onboarding_interviews`（历史访谈数据无保留价值，直接清退）。
5. 配置面清理：`NVIDIA_API_KEY`/`NIM_BASE_URL`/`NIM_MODEL`/`NIM_TIMEOUT`
   从 compose.yaml、.env.example、playwright.config.ts 移除；
   `server.Config` 不再有 NIM 字段。
6. 词表同步：CONTEXT.md 的 Taste interview 词条退役，新增 Starter pack
   词条并把访谈列入其 _Avoid_。

## 后果

- 产品运行期零 LLM、零出网依赖；部署仅需 SESSION_SECRET 一个秘密。
- 入池路径收敛为两条（起步包 / 手工挑菜），空池欢迎的叙事随之简化。
- ADR-0010 转为 Superseded；ADR-0021 的包清单中 `internal/onboarding` 行
  与「NIM port」例外条款自本决议起失效（该 ADR 原文存档不改）。
- 老库升级时访谈历史数据被删；这是有意为之，不提供导出。
