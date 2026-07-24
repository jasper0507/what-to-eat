# GitHub 开源调研：每日「吃什么」决策 / 家常选餐

**调研日期：** 2026-07-21  
**问题场景：** 用户每天不知道吃什么（决策疲劳）；母亲负责做饭并每天询问；需要简单的响应式 Web App（手机 + 桌面）自动决定菜单；算法应避免连续多天重复同一道菜（吃腻）；中文家常菜语境优先；尽量复用成熟开源，用户技能有限。

**方法：** 以 GitHub 仓库为主源（`gh` 元数据 + README + 关键源码片段），辅以公开 demo 链接；优先核对 stars、最近推送、许可证、技术栈与真实功能。

---

## 1. Executive summary

- **没有一款成熟开源产品**能「开箱即用」完整覆盖：中文家常菜目录 + 每日一键决策 + 吃腻/防连日重复 + 给妈妈用的极简 UI + 纯轻量 Web/PWA。
- **产品形态最接近需求的是「饭搭子」**（`meimengchengzhen/jin-tian-chi-shenme`）：家庭一键今晚饭、历史降权、不吃重复、一周菜单、冰箱/忌口。但许可证为 **All Rights Reserved（非开源）**，**不可 fork、不可抄代码/数据池**，只能从设计与算法思路上「观摩借鉴」。
- **最适合合法 fork / 改造的开源底座是 `ryanuo/whatToEat`**（MIT，~89★，Nuxt 4 + Vue，PWA，demo 在线，数据对接 HowToCook）。现状是「转盘式随机 + 分类筛选」，**尚无历史/防重复**；推荐规则代码存在但 README 与注释均表明「简易算法暂未使用」。
- **Mealie / Tandoor** 是业界最成熟的「菜谱库 + 周计划 + 购物清单」自托管方案（各 8k–12k★），带「随机填餐」规则，但定位是**家庭食谱管理系统**，部署与心智负担远超「每天帮妈妈决定一道/一桌菜」；中文家常语境需自建菜谱。
- **HowToCook**（~101k★，Unlicense）是优质**中文菜谱内容源**，不是决策 App；其《如何决策吃什么》给出荤素数量公式，并直接指向 `whatToEat` 作为选菜工具。
- **诚实结论：** 宜 **薄 fork `whatToEat` + 加历史/防重复 + 家庭菜谱可编辑**；或 **greenfield 极简 PWA** 借鉴经典转盘 UX + 饭搭子评分思想 + HowToCook 数据。不建议一上来上 Mealie/RAG 全家桶。

---

## 2. Comparison table（Top candidates）

| 项目 | Stars（约） | License | 最近活动 | Stack | 自动决策 | 历史/防重复 | 中文家常 | 移动 Web | Verdict |
|------|------------|---------|----------|-------|----------|-------------|----------|----------|---------|
| [ryanuo/whatToEat](https://github.com/ryanuo/whatToEat) | ~89 | MIT | 2026-07 活跃 | Nuxt/Vue, PWA, Docker | ✅ 转盘随机 + 分类 | ❌（算法草稿未接） | ✅ HowToCook | ✅ demo | **fork & adapt** |
| [meimengchengzhen/jin-tian-chi-shenme](https://github.com/meimengchengzhen/jin-tian-chi-shenme)（饭搭子） | ~1 | **All Rights Reserved** | 2026-05 | React+Vite+Tailwind PWA | ✅ 多动线一键 | ✅ 历史+noRepeat+swap 降权 | ✅ 强 | ✅ Pages demo | **borrow ideas only** |
| [YunYouJun/cook](https://github.com/YunYouJun/cook) | ~6.4k | MIT | 2026-04 push | TS, Docker, 曾 PWA | ⚠️ 按食材反查菜 | ❌ | ✅ 强 | ✅ | **borrow ideas only**（冰箱场景） |
| [Anduin2017/HowToCook](https://github.com/Anduin2017/HowToCook) | ~101k | Unlicense | 极活跃 | Markdown 菜谱 | ⚠️ 公式/链接工具 | ❌ | ✅ 数据金库 | Web viewer | **数据源 / 内容** |
| [mealie-recipes/mealie](https://github.com/mealie-recipes/mealie) | ~12.7k | AGPL-3.0 | 极活跃 | Python+Vue, Docker | ✅ 随机 meal plan + 规则 | ⚠️ 计划日历有，非「吃腻」专用 | ⚠️ 需自导 | ✅ 移动优化 | **skip 主路径** / 可选远期自托管 |
| [TandoorRecipes/recipes](https://github.com/TandoorRecipes/recipes) | ~8.5k | AGPL-3.0（0.10+） | 活跃 | Django 等, Docker | ✅ 周计划 | ⚠️ 同 Mealie 类 | ⚠️ | ✅ | **skip 主路径** |
| [huangyafei/chishenme](https://github.com/huangyafei/chishenme) | ~84 | WTFPL | 2023 push | jQuery 静态页 | ✅ 自定义列表转盘 | ❌ | 自定义文本 | ✅ 极简 | **borrow UX only** |
| [MskTmi/WhatToEat](https://github.com/MskTmi/WhatToEat) | ~2 | MIT | 2026-04 | 静态 + PWA | ✅ 转盘 | ❌ | 外卖/食堂语境 | ✅ | **borrow UX** |
| [Chunyu33/what-to-eat](https://github.com/Chunyu33/what-to-eat) | ~2 | 未声明 | 2025-12 | React+Vite+TW | ✅ 按时段随机 | ❌ | 有 | ✅ | **skip / 参考** |
| [boby4/Meal-Planner](https://github.com/boby4/Meal-Planner) | ~1 | MIT | 2026-07 | Next.js + DeepSeek | ✅ 随机/AI/冰箱 | ⚠️ 换一批去重 | ✅ 中文 H5 | ✅ | **fork 可选**（依赖 API Key） |
| [FutureUnreal/What-to-eat-today](https://github.com/FutureUnreal/What-to-eat-today) | ~202 | MIT | 2026-02 | Next+Flask+Neo4j+Milvus | ✅ 对话 RAG | 对话态 | ✅ 教程案例 | ✅ | **skip**（过重） |
| [lzx2005/WhatToEat](https://github.com/lzx2005/WhatToEat) | ~157 | 无 | 2018 停更 | 微信小程序 | ✅ | ? | ✅ | 小程序 | **skip** |
| [juriansluiman/groceri.es](https://github.com/juriansluiman/groceri.es) | ~392 | MIT | 2024-05 | 自托管 recipe+plan | 计划向 | 计划向 | ⚠️ 英文向 | Web | **skip 主路径** |
| [peterthepeter/groly](https://github.com/peterthepeter/groly) | ~52 | AGPL-3.0 | 2026-07 | Svelte PWA Docker | 周计划 | 追踪向 | ⚠️ | ✅ 强 | **skip**（买菜/营养，非决策） |

---

## 3. Detailed notes per project

### 3.1 ryanuo/whatToEat — 今天吃什么决策工具

- **URL:** https://github.com/ryanuo/whatToEat  
- **Demo:** https://eat.ryanuo.cc/  
- **Stars / License / Activity:** ~89★ · MIT · 2026-07 仍有 push（含 GHCR 镜像、CI）  
- **Stack:** Nuxt（Vue 3）、UnoCSS、Pinia、`@vite-pwa/nuxt`、Docker / Netlify / Vercel  
- **相关能力（仓库证据）：**
  - 主交互：`Eat.vue` 中分类筛选 + **快速随机滚动转盘**，点停即选中。
  - 菜谱来自远程 JSON / `public/recipes.json`，服务端 `server/routes/api/recipes.ts`；README 写明数据参考 [HowToCook](https://github.com/Anduin2017/HowToCook)。
  - `useRecommend.ts` 实现按时段/天气/菜系关键词过滤后随机，但文件头注释：**「简易算法推荐 目前暂未使用」**。
  - 分类选择用 `useStorage` 持久化到 localStorage；**无用餐历史、无防连日重复**。
  - 明确支持 PWA 配置与 Docker 一键部署。
- **Gaps：** 缺「昨天/近 N 天吃过」；缺家庭菜谱自定义（妈妈拿手菜清单）；缺一桌荤素搭配（只有单道随机）；推荐算法未接线。
- **Verdict: fork & adapt** — 场景、语言、栈、许可证、demo、部署路径都最贴「轻量中文决策 Web」。

### 3.2 meimengchengzhen/jin-tian-chi-shenme（饭搭子 · Fanda）

- **URL:** https://github.com/meimengchengzhen/jin-tian-chi-shenme  
- **Demo:** https://meimengchengzhen.github.io/jin-tian-chi-shenme/  
- **Stars / License / Activity:** ~1★ · **All Rights Reserved（明确禁止复制/修改/分发/训练）** · 2026-05  
- **Stack:** React + Vite + Tailwind · 纯前端 PWA · localStorage · 可选 Supabase 同步  
- **相关能力（README + `client/src/lib/*`）：**
  - 单人/家庭一键今晚饭、一周菜单、冰箱、剩菜改造、忌口协调、海报式方案、购物清单。
  - `history.ts`：最多 30 条「就吃这个」历史；`recentRecipeIds(limit=7)` 供降权；`noRepeat` 开关。
  - `recommend.ts` 评分：随机扰动 + 口味/菜系/难度 + 档案 + 环境 + 热量 + 场景（长辈/儿童/家庭桌面结构）+ **收藏加分 / 历史降权 / 换一批强降权**；忌口硬过滤；软约束可逐级放宽。
  - 内置大量中文菜谱与内容池（README 自称 1000+ 菜等）——但**受版权保留限制**。
- **Gaps vs 合法复用：** 不能作为代码或数据起点；stars 低、个人作品；功能面极宽，对「技能有限」用户若整站使用反而复杂。
- **Verdict: borrow ideas only** — 算法与信息架构的最佳参考，**不可 fork**。

### 3.3 YunYouJun/cook — 来做菜

- **URL:** https://github.com/YunYouJun/cook  
- **Demo:** https://cook.yunyoujun.cn  
- **Stars / License / Activity:** ~6.4k★ · MIT · 2026-04 push  
- **Stack:** TypeScript、Docker；文档齐全；曾强调 PWA，README 现称在做新 App  
- **相关能力：** 按**已有食材 + 厨具**匹配可做菜谱（模糊/严格匹配），中文居家场景；**不是**「不知道吃什么时的随机决策器」。
- **Gaps：** 不解决决策疲劳主路径；无防重复选餐历史。
- **Verdict: borrow ideas only** — 若后续加「冰箱里有什么」模块可参考匹配思路。

### 3.4 Anduin2017/HowToCook — 程序员做饭指南

- **URL:** https://github.com/Anduin2017/HowToCook  
- **站点:** https://howtocook.aiursoft.com  
- **Stars / License / Activity:** ~101k★ · Unlicense · 极活跃  
- **Stack:** Markdown 菜谱集合 + 社区 viewer  
- **相关能力：**
  - 大量中文家常做法，难度分级、分类清晰。
  - `tips/如何选择现在吃什么.md`：用公式算荤素数量（`a=floor((N+1)/2)`, `b=ceil((N+1)/2)`），并建议肉类轮换；**明确推荐**使用 [whatToEat](https://github.com/ryanuo/whatToEat) 做选菜。
- **Gaps：** 本身不是决策 App。
- **Verdict: 内容/数据底座** — 任何自建方案都应优先对接或引用。

### 3.5 mealie-recipes/mealie

- **URL:** https://github.com/mealie-recipes/mealie  
- **Docs / Demo:** https://docs.mealie.io · demo.mealie.io  
- **Stars / License / Activity:** ~12.7k★ · AGPL-3.0 · 极活跃  
- **Stack:** Python 后端 + Vue 前端 · Docker 自托管  
- **相关能力：** 菜谱抓取/导入、分类标签、**日历式 Meal Planning**、购物清单、家庭协作；`POST .../random` 按家庭 mealplan rules 随机填餐（`order_by=random` + 规则过滤）。
- **Gaps：** 安装运维门槛高；默认菜谱生态偏西方站点抓取；随机是「规划器功能」而非「妈妈每天一问」；AGPL 传染性对再分发需注意；无内置「连吃几天降权」的吃腻模型。
- **Verdict: skip 作为主交付**；仅当家庭已自托管、要做完整食谱库时再考虑。

### 3.6 TandoorRecipes/recipes

- **URL:** https://github.com/TandoorRecipes/recipes  
- **Demo:** https://app.tandoor.dev/e/demo-auto-login/  
- **Stars / License / Activity:** ~8.5k★ · AGPL-3.0（0.10+）· 活跃  
- **能力：** 菜谱管理、多餐/日计划、购物清单、Cookbook、移动优化、AI 辅助等 — 与 Mealie 同赛道。
- **Verdict: skip 主路径**（过重，同 Mealie）。

### 3.7 huangyafei/chishenme 与同源「中午吃什么」转盘

- **URL:** https://github.com/huangyafei/chishenme  
- **Stars / License / Activity:** ~84★ · WTFPL · 2023 最后 push  
- **Stack:** 单页 HTML + jQuery；`random.js` 对用户文本列表 `setInterval` 随机刷字，点停锁定。
- **相关：** 经典「吃什么？→ 吃这个！」交互，办公室语境；用户自填选项。  
- **变体：** [MskTmi/WhatToEat](https://github.com/MskTmi/WhatToEat)（MIT，加 PWA，demo https://eat.msktmi.com）；[SChen1024/WhatToEat](https://github.com/SChen1024/WhatToEat) 等同源。
- **Gaps：** 无结构化菜谱、无历史防重复、几乎不维护。
- **Verdict: borrow UX only**（转盘节奏与「不行换一个」文案）。

### 3.8 Chunyu33/what-to-eat

- **URL:** https://github.com/Chunyu33/what-to-eat  
- **Demo:** https://what-to-eat-rouge.vercel.app  
- **~2★，许可证未声明** · React 19 + Vite + Tailwind + Framer Motion  
- **能力：** 按时段（早/中/下午茶/晚/宵夜）严格分类随机；搜索；暗色；「愤怒模式」彩蛋。  
- **Gaps：** 无历史；许可证不清；体量玩具级。  
- **Verdict: skip / 仅 UX 参考**。

### 3.9 boby4/Meal-Planner — AI 今天吃什么

- **URL:** https://github.com/boby4/Meal-Planner  
- **Demo:** https://meal-planner-nu-five.vercel.app  
- **~1★ · MIT · 2026-07 活跃** · Next.js 16 + DeepSeek + 下厨房语料  
- **能力：** 随机 / AI 条件 / 冰箱食材；「换一道」自动去重；移动优先。  
- **Gaps：** 强依赖 API Key 与外网模型；维护与成本对「给妈妈用」不稳定；社区几乎为零。  
- **Verdict: 可选参考**（去重交互、三种入口），不宜作为唯一底座。

### 3.10 FutureUnreal/What-to-eat-today

- **URL:** https://github.com/FutureUnreal/What-to-eat-today  
- **~202★ · MIT** · Next + Flask + Neo4j + Milvus 图 RAG 教学案例  
- **Gaps：** 基础设施过重；非日常家庭产品形态。  
- **Verdict: skip**。

### 3.11 lzx2005/WhatToEat（微信小程序）

- **URL:** https://github.com/lzx2005/WhatToEat · ~157★ · 无许可证 · **2018 停更**  
- **Verdict: skip**（平台与活跃度均不符）。

### 3.12 其他扫过的类别

| 类别 | 代表 | 说明 |
|------|------|------|
| 附近餐厅 roulette | puremana/food-roulette 等 | 解决「出去吃哪家」，非家里做什么 |
| 机器人插件 | A-kirami/whattoeat（NoneBot） | 聊天指令，非 Web App |
| 原生 Android | ZhangMiao147/TodayEat | Java 摇一摇，非 Web |
| 自托管全家桶 | groly、groceri.es | 买菜/营养/计划，决策路径弱 |

---

## 4. Recommendation（排序）

### (A) 直接采用现有产品（Adopt as-is）

1. **无完美候选。**  
2. **若只想「先能用、不开发」且接受非开源在线服务：** 可试用 [饭搭子 GitHub Pages](https://meimengchengzhen.github.io/jin-tian-chi-shenme/) 做体验对照（数据仅存本机浏览器），**不要**基于其代码建产品。  
3. **若家庭已会 Docker、要完整食谱库：** Mealie demo/自托管可作「周计划」补充，但仍非「每天一键决策」最优解。

### (B) Fork 改造（推荐主路径）

| 排序 | 仓库 | 原因 | 建议改造点 |
|------|------|------|------------|
| **1** | **ryanuo/whatToEat** | MIT、中文、PWA、demo、HowToCook 数据、栈现代、部署简单 | ① 记录每日确认菜品（localStorage）② 近 N 天硬排除或软降权 ③ 可编辑「家里常做菜」清单 ④ 一桌：主菜+素+汤（可套用 HowToCook 公式）⑤ 把未使用的 `useRecommend` 接上或替换为加权抽样 |
| 2 | boby4/Meal-Planner | MIT、中文 H5、已有「换一批去重」 | 去掉或可选化 AI；强化本地历史；控制 API 成本 |
| 3 | MskTmi/WhatToEat 或 chishenme | 极简转盘 | 仅当坚持「零框架静态页」时；需自建菜列表与历史 |

### (C) Greenfield + 借鉴

适合目标极简（「一个大按钮 + 家里 30–80 道家常菜 + 不连日重复」）且不想背 Nuxt 历史包袱时：

| 借鉴来源 | 借什么 | 不要借什么 |
|----------|--------|------------|
| chishenme / whatToEat | 转盘「开始/停止/不行换一个」节奏 | 陈旧 jQuery 实现 |
| 饭搭子（思想层） | 硬忌口 / 软降权 / recentSwap 强惩罚 / 一桌结构 / 历史 30 条 | 源码、数据池、品牌与文案整段 |
| HowToCook | 菜谱正文、荤素数量公式、肉类轮换建议 | 把整个 monorepo 当 App |
| cook.yunyoujun | 食材匹配启发 | 整站产品定位 |
| Mealie | 随机 + rules 的产品概念 | 过重架构作为 v1 |

**对技能有限用户的务实建议：** 优先 **B1 fork whatToEat**，增量加 2–3 个文件级功能（history + weighted pick），比从零搭 PWA 或部署 Mealie 更可控。

---

## 5. Algorithm patterns observed（附来源）

### 5.1 纯均匀随机 + UI 转盘（最常见）

- **模式：** 从当前候选列表 `list[Math.floor(Math.random()*n)]`，用短间隔刷新制造「抽奖感」，用户点击停止锁定。  
- **来源：**  
  - `huangyafei/chishenme` → `random.js`  
  - `ryanuo/whatToEat` → `app/components/Eat.vue`（`startRandom` / `stopRandom`）  
- **优点：** 实现极简、心理上「把决策外包给运气」。  
- **缺点：** 无状态 → 易连日撞车。

### 5.2 规则关键词过滤后再随机（草稿级）

- **模式：** 按时段/天气/菜系 keyword 过滤菜名，失败则回退全量，再均匀随机。  
- **来源：** `ryanuo/whatToEat` → `app/composables/useRecommend.ts`（**未接入主流程**）。

### 5.3 加权评分 + 历史软/强降权（最贴近「防吃腻」）

- **模式（饭搭子 `recommend.ts` / `history.ts`）：**  
  1. **硬约束：** 忌口、过敏、素食/无辣等 → 直接剔除。  
  2. **软约束分级放宽：** 用时 → 菜系 → 难度，候选不足时放宽 level。  
  3. **得分：** `s = random()*0.6 + 口味/菜系/难度/环境/热量/场景…`  
  4. **历史：** `noRepeat` 时近期 id **−1.5**；「换一批」池 **−3.5**（强降权，避免连抽同一道）。  
  5. **反馈：** 喜欢 +1.6 / 不喜欢 −4.0 / 收藏 +1.2；相似菜系口味轻加分。  
  6. **历史存储：** localStorage，最多 30 条；`recentRecipeIds(7)`。  
- **来源：** https://github.com/meimengchengzhen/jin-tian-chi-shenme （**思想可参考，实现不可复制**）。

### 5.4 计划器内「按规则随机填一天」

- **模式：** 对某 date + meal type，用家庭规则生成 query filter，再 `order_by=random` 取 1 道写入 meal plan。  
- **来源：** Mealie `controller_mealplan.py` → `create_random_meal` / `_get_random_recipes_from_mealplan`。  
- **特点：** 强在「规则化食谱库」，弱在「连续几天多样性」需用户自己看日历或另写规则。

### 5.5 人数 → 荤素道数公式 + 肉类轮换启发式

- **模式：** `菜数 = 人数 + 1`；荤素数量接近且荤可略多；大桌考虑鱼；避免同一动物肉连上。  
- **来源：** HowToCook `tips/如何选择现在吃什么.md`。  
- **适合：** 「一桌菜」生成器的外层结构，内层再套 5.1 或 5.3。

### 5.6 食材集合匹配（反向：有什么做什么）

- **模式：** 用户勾选食材/厨具 → 模糊或严格匹配菜谱集合。  
- **来源：** YunYouJun/cook README/features。  
- **适合：** 决策后的可行性约束，或「冰箱优先」模式，而非纯随机。

### 5.7 AI / RAG 对话推荐

- **模式：** LLM 或图+向量检索对话出菜。  
- **来源：** boby4（DeepSeek）、FutureUnreal（Neo4j+Milvus）。  
- **风险：** 成本、延迟、可重复性差、运维重；对「每天固定问妈妈」场景性价比低。

### 5.8 建议落地的最小防吃腻算法（综合、可自研）

面向本问题的 **v1 实用算法**（综合开源常见做法，非复制任一仓库）：

```
候选 = 用户启用的家常菜列表（可从 HowToCook 子集 + 妈妈自定义）
过滤：硬排除近 cooldownDays（如 2～3 天）内已做的菜
权重：base = 1
      + 偏好加分
      - 近 windowDays（如 7～14 天）出现次数 * penalty
      （可选）同类肉/同类做法多样性惩罚
抽样：按权重加权随机；UI 仍可用转盘展示最终候选动画
确认：「就吃这个」写入 history[date]
```

比「纯随机」多约一个 localStorage 历史表即可，符合技能有限、移动 Web 的约束。

---

## 6. Sources（primary）

| 资源 | 链接 |
|------|------|
| ryanuo/whatToEat | https://github.com/ryanuo/whatToEat |
| whatToEat demo | https://eat.ryanuo.cc/ |
| 饭搭子 | https://github.com/meimengchengzhen/jin-tian-chi-shenme |
| 饭搭子 demo | https://meimengchengzhen.github.io/jin-tian-chi-shenme/ |
| YunYouJun/cook | https://github.com/YunYouJun/cook |
| HowToCook | https://github.com/Anduin2017/HowToCook |
| HowToCook 决策 tip | https://github.com/Anduin2017/HowToCook/blob/master/tips/如何选择现在吃什么.md |
| Mealie | https://github.com/mealie-recipes/mealie |
| Tandoor | https://github.com/TandoorRecipes/recipes |
| chishenme | https://github.com/huangyafei/chishenme |
| MskTmi/WhatToEat | https://github.com/MskTmi/WhatToEat |
| boby4/Meal-Planner | https://github.com/boby4/Meal-Planner |
| FutureUnreal RAG 案例 | https://github.com/FutureUnreal/What-to-eat-today |
| lzx2005 小程序 | https://github.com/lzx2005/WhatToEat |
| groceri.es | https://github.com/juriansluiman/groceri.es |
| groly | https://github.com/peterthepeter/groly |

**元数据说明：** stars、pushedAt、license 等取自 2026-07-21 前后 `gh repo view` / `gh search repos`；功能以各仓库 README 与抽查源码为准。stars 为近似值，会随时间变化。

---

## 7. Bottom line

| 问题 | 答案 |
|------|------|
| 能直接复用一个完美开源吗？ | **不能。** |
| 最近似的完整产品？ | **饭搭子** — 但 **非开源**。 |
| 最好的开源起点？ | **`ryanuo/whatToEat`（fork + 历史/防重复/家庭菜库）**。 |
| 最好的数据？ | **HowToCook（Unlicense）**。 |
| 最不该一上来做的？ | Mealie/Tandoor 全家桶、图 RAG、强绑定付费 LLM。 |
| 算法核心？ | **候选池 + 近 N 天冷却/降权 + 加权随机**；UI 保留转盘确认感。 |
