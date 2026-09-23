# Jev R1 — 产品与技术设计（grilling 收口稿）

**日期：** 2026-09-22（Asia/Shanghai）  
**状态：** 待确认测试接缝后 → to-spec 发 GitHub Issue  
**非目标本文件：** 不实现、不改生产、不发 Spec Issue（等接缝确认）

---

## 1. 问题（用户视角）

用一段时间后 Decision 易缩成少数几道菜（孤岛）；Catalog 偏薄（另开 Issue）；懂我引擎对「这顿想怎样」语义不够。希望引入官方云 TypeSafe **System One（产品口语：Jev）** 做终裁，同时保住可测的新鲜感/冷却算术与可运行演示站。

成功指标不变：**首揭接受率**（可由 `decisions` / 接受路径计算）。

---

## 2. 方案（用户视角）

每次揭示：

1. 可选：点选中文标签 + 一行「这顿想怎样」（Meal intent）。
2. 懂我引擎做资格过滤与短名单（Top-16；探索槽底 2，Noul 要探索时抬到 4）。
3. 一次 Jev fan-out：`Choice(短名单)` + `Noul(要不要探索)` + `Score(合不合这顿)`；有忌口/限制信号时再加安全 `Noul`。
4. Go 合成唯一 Decision；中文理由用本地模板吃 Jev 档位（不硬锁文案、不接聊天模型写理由）。
5. 有密钥但 Jev 调用失败：推荐回退本地加权抽样；安全 Noul 失败则 fail-closed。
6. 无有效密钥：默认不可用并提示；显式「仅本地模式」开关（默认关）才允许纯本地开饭。

菜谱扩容：**并行另开 Issue**，不挡本 Spec 验收。

---

## 3. 领域词（拟写入 CONTEXT.md）

| 词 | 含义 |
|----|------|
| **System One（Jev）** | TypeSafe 托管的 System One API（Choice / Score / Noul）。产品口语可称 Jev；文档正式名 System One。不是聊天 LLM，不生成自由中文段落。 |
| **Shortlist** | 懂我引擎从 Candidate pool（及 Discovery 探索槽）筛出的有限候选集（本轮固定 Top-16），供 System One Choice。 |
| **Explore slot** | Shortlist 中强制留给高新鲜感 / Discovery 的名额（底 2；探索 Noul 为真时 4）。 |
| **Meal intent** | 本顿可选输入：中文预设标签（state 内英文枚举）+ 可选自由文本。 |
| **Local-only mode** | Eater/部署显式开关（默认关）：无 System One 密钥时仍可用纯懂我路径揭示。 |
| **Jev-composed decision** | 短名单 + System One 终裁合成的 Decision；失败时的本地回退仍是 Decision，须可观测（日志/可选弱提示，文案不硬锁）。 |

_Avoid_：把 System One 叫成「口味访谈」或复活 NIM onboarding；把 Shortlist 叫成最终 Decision；让 Jev 做冷却天数/次数算术。

---

## 4. 与现有 ADR 关系

| ADR | 关系 |
|-----|------|
| **0022 懂我引擎** | Shortlist / 探索槽 / 本地回退仍遵守四因子与 OccasionNever；不把算术交给 Jev。 |
| **0023 废止访谈 / 零出网** | **本轮新 ADR（建议 0026）** 开例外：可选出站仅限 System One；默认无密钥则不走 Jev；不是复活口味访谈。 |
| **0019 Meal lifecycle 深模块** | HTTP 五行为接缝仍是主回归面；Chooser 是 lifecycle 内部第二适配器，不把规则抬到 handler。 |

---

## 5. 模块与接缝（技术）

### 5.1 推荐接缝（一个主内部接缝 + 沿用公共 HTTP）

```
HTTP Meal lifecycle（Begin / Reroll / …）     ← 主验收 / ADR-0019 测试面
  └─ meal.Lifecycle.chooseDecision
        └─ Chooser.Choose(...)               ← R1 新内部接缝（第二适配器出现）
              ├─ LocalChooser                ← 今日路径（无密钥且仅本地开；或回退）
              └─ JevComposedChooser
                    ├─ Local prefilter → Shortlist（engine 算术不动）
                    ├─ internal/jev Client   ← System One HTTP（可 fake）
                    └─ 合成 Decision + 理由模板
```

**不**把接缝开进 `engine.ScorePool`（会污染新鲜感真理源）。  
**不**只在 HTTP/FE 拼装（会拆散 lifecycle 不变量）。

### 5.2 配置

- `TYPESAFE_API_KEY`（或项目统一前缀）：可选；缺失行为见 Q20=D。
- Endpoint / timeout / model pin：设计阶段定默认值；密钥只在 Go 后端。
- Local-only：持久化偏好或部署配置（实现时二选一写进 Spec Implementation Decisions）。

### 5.3 前端（grilling 已锁定，接缝笔记「零 FE」作废）

本轮需要 FE：

- Begin 前：Meal intent（标签 + 可选文本）→ 随 begin/reroll 上传。
- 设置或部署面：**仅本地模式**开关（默认关）。
- Reveal 仍用现有 Decision.reason / mode；理由文案不硬锁。

### 5.4 测试策略

| 面 | 测什么 |
|----|--------|
| `internal/engine` 表测 | 四因子 / 放宽 / Discovery 权重 — 回归真理源 |
| `jev.Client` fake | fan-out 组包、超时、429、安全 Noul fail-closed、推荐 fail-open |
| `JevComposedChooser` 单测 | Shortlist 大小、探索槽 2→4、回退路径 |
| HTTP lifecycle（主） | Begin/Reroll 仍吐合法 Decision；缺密钥+未开仅本地 → 明确错误码；开仅本地 → 与旧种子 RNG 行为兼容 |

---

## 6. 风险（Spec 必写）

1. 中文自由文本对英文偏置模型效果不稳 → 标签枚举托底。  
2. 缺密钥默认不可用 vs 演示站可用性 → 依赖运维配密钥或开仅本地。  
3. Choice 相对选择陷阱 → 只在 Shortlist 上 Choice，且保留 OccasionNever。  
4. ADR-0023 读者惊讶 → 必须有例外 ADR。  
5. 孤岛：仅靠 2 席探索槽可能不够 → 扩容另 Issue；本 Spec 验收含探索槽行为测试。

---

## 7. Out of scope（本 Spec）

- Catalog 大批量扩容 / 导入流水线（另 Issue）
- 聊天 LLM 写理由；自托管 OpenJev/GPU
- 改四因子曲线公式本身（除非实现发现与 Shortlist 冲突的最小修补）
- 生产部署 / DNS / 密钥上机（需另批）

