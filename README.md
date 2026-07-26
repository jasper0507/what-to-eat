# 今天吃什么？

一个只回答一个问题的自托管 Web 应用：这一顿吃什么。

给「每天不知道吃什么」的决策疲劳做的单人工具。你把爱吃的菜放进池子，
每到饭点让它揭示一道——不给清单、不给推荐流，一次只上一道菜，
不满意就换，换满为止。全程没有 LLM、没有外部服务调用，
一个 Go 二进制加一个 SQLite 文件就是全部。

## 它怎么定这一顿

**池子**。决策只从你亲手维护的候选池里挑。空池开局有两条路：
经典起步包（14 道家常菜一键入池）或去菜谱库手工挑菜。菜谱数据来自
[HowToCook](https://github.com/Anduin2017/HowToCook)，导入时顺手解析
主料、味型、工艺、难度与耗时。

**情感刻度，不打分数**。对菜的喜欢程度用五档黑话表达：
拉完了 / NPC / 人上人 / 顶尖 / 夯。入池只开上三档——你不会把讨厌的菜
放进池子；下两档只在饭后评价出现，评到就是剔池。

**揭示**。点「开始这一顿」，舞台上落下一道菜和一行人话理由
（「你的夯」「久别重逢」）。每顿 3 次「换一道」额度；换完还不满意，
三个出口自己选：就吃这个、亲自点一道、这顿不吃了。

**懂我引擎**。每次揭示对全池做四因子乘法加权抽样：

```
分数 = 喜爱 × 新鲜感 × 场合 × 本顿否决
```

喜爱看档位，新鲜感按「距上次吃过隔了几顿」计数（不是日历天），
场合让早餐类只在早晨登台，本顿否决保证一顿之内不重复上菜。
连续四次被换的菜自动降一档，零打扰。全池归零时逐层放宽并在理由行里说明。

**池子外的新尝试**。池子太小、可选太少或你连着换菜时，探索压力上升
（每个信号 +25% 概率，封顶 75%），引擎会从池外按口味画像相似度挑一道
标着「池子外的新尝试」的菜打明牌登台。吃完必须先评价，才能开下一顿——
评到上三档它就顺势入池。

**轻历史**。最近吃过、最近爱吃，一目了然。补评价一菜一口、只认最近，
旧感受没有机会倒灌覆盖现役档位。

**PWA**。手机浏览器里「添加到主屏幕」，装成一个像原生的应用。

## 技术形态

| 层 | 选型 |
|---|---|
| 后端 | Go 1.26 + Gin，单二进制，内嵌有序 schema 迁移 |
| 存储 | SQLite 单文件，无外部依赖 |
| 前端 | React 19 + Vite + TypeScript，Tailwind CSS v4 + shadcn/ui，TanStack Query |
| 验收 | Playwright 全栈规格（移动 + 桌面双端），跑在真实 Go testserver 上 |
| 秘密 | 只有一个：`SESSION_SECRET` |

## 本地开发

需要 Go 1.26+ 和 Node.js 24+。

```bash
export SESSION_SECRET="$(openssl rand -base64 32)"

# 终端 1：API 和 SQLite
go run ./cmd/server

# 终端 2：React 开发服务器
cd frontend
npm install
npm run dev
```

打开 `http://localhost:5173`。用户名须为 3–32 个字符（支持中文），
密码至少 8 个字符且不超过 72 个 UTF-8 字节。

### 导入 HowToCook 菜谱库

```bash
git clone https://github.com/Anduin2017/HowToCook.git
CATALOG_DIR=/path/to/HowToCook/dishes go run ./cmd/server
```

导入幂等：同一路径始终是同一道菜的身份，重跑只更新内容。
未设置 `CATALOG_DIR` 时使用数据库中已有的菜谱库。

### 质量门

```bash
go test ./...         # 后端全量（引擎、生命周期、HTTP 契约）

cd frontend
npm run typecheck     # tsc 项目引用：src、tests、配置一起检查
npm run lint          # ESLint（类型感知 + react-hooks + @tanstack/query）
npm run format:check  # Prettier（tests/ 不参与格式化，保持验收规格逐字节稳定）
npm run test          # Vitest 纯逻辑单测（API client、校验、格式化）
npm run test:browser  # Playwright 全栈验收（自动构建并启动真实 Go testserver）
```

若本机 shell 设置了 HTTP 代理，Playwright 的健康检查可能不识别 `127.*`
通配写法而走代理导致 502。此时用
`NO_PROXY="127.0.0.1,localhost" npm run test:browser` 运行。

## 配置

全部经环境变量，未设置的项走默认值：

| 变量 | 默认 | 说明 |
|---|---|---|
| `SESSION_SECRET` | 必填 | 至少 32 字节的随机值 |
| `DATABASE_PATH` | `data/what-to-eat.db` | SQLite 文件位置 |
| `CATALOG_DIR` | 空 | HowToCook `dishes` 目录，设置则启动时导入 |
| `WEB_DIR` | `frontend/dist` | 前端构建产物目录 |
| `APP_ENV` | 空 | `production` 时启用 Secure Cookie |
| `DISCOVERY_*` | 见下 | 探索阈值，无需重编译即可调 |

探索阈值：`DISCOVERY_ENABLED`、`DISCOVERY_MAX_POOL_SIZE`、
`DISCOVERY_MAX_ELIGIBLE_DISHES`、`DISCOVERY_MIN_REROLLS`、
`DISCOVERY_RECENT_MEAL_WINDOW`、`DISCOVERY_MAX_DISCOVERIES_PER_MEAL`。
嵌入使用时也可从 `server.DefaultDiscoveryConfig()` 取默认值后经
`server.Config.Discovery` 调整。

## 单节点部署（Docker Compose）

```bash
install -m 600 .env.example .env
# 编辑 .env，把 SESSION_SECRET 换成至少 32 字节的随机值：
# openssl rand -base64 32
docker compose up --build -d
docker compose ps
```

镜像固定打包 HowToCook 菜谱库的
`c05758fa661ac4efa0361a987b700a351a22159b` 版本，`/api` 与前端
由同一个非 root 容器在 `8080` 端口提供。SQLite 数据保存在 Compose 命名卷
`what_to_eat_data` 中。生产会话 Cookie 启用 `Secure`、`HttpOnly` 和
`SameSite=Lax`，浏览器访问应由同机反向代理提供 HTTPS。

`SESSION_SECRET` 缺失时 Compose 拒绝启动；会话秘密短于 32 字节或
SQLite 卷不可写时，应用输出明确错误并退出。不要把 `.env` 提交进版本库。

## 备份与恢复

备份前短暂停止应用，确保复制的是一个一致的 SQLite 文件。目录名换成实际时间：

```bash
mkdir -p backups/2026-07-26
docker compose stop app
docker compose run --rm --no-deps \
  --user "$(id -u):$(id -g)" \
  -v "$PWD/backups/2026-07-26:/backup" \
  --entrypoint sh app \
  -c 'cp /app/data/what-to-eat.db /backup/what-to-eat.db'
install -m 600 .env backups/2026-07-26/env
docker compose start app
```

恢复时先还原对应环境配置（尤其 `SESSION_SECRET`），停止应用但不要用
`docker compose down -v`，再覆盖卷内数据库：

```bash
install -m 600 backups/2026-07-26/env .env
docker compose down
docker compose run --rm --no-deps \
  --user root \
  -v "$PWD/backups/2026-07-26:/restore:ro" \
  --entrypoint sh app \
  -c 'cp /restore/what-to-eat.db /app/data/what-to-eat.db && chown app:app /app/data/what-to-eat.db'
docker compose up -d app
docker compose ps
```

数据库备份与 `env` 文件必须作为一组保管，后者包含敏感配置。

## 文档地图

- [`CONTEXT.md`](CONTEXT.md) — 领域词表，产品里每个概念的正名与禁忌
- [`docs/adr/`](docs/adr/) — 全部架构决议，状态行与代码保持一致
- [`docs/copy.md`](docs/copy.md) — 全应用用户可见文案的唯一口径
- [`docs/redesign-brief.md`](docs/redesign-brief.md) — 现行版本的执行规格与红线
- [`frontend/tests/`](frontend/tests/) — Playwright 验收规格，即产品行为的活文档

## 致谢

菜谱数据来自 [HowToCook（程序员做饭指南）](https://github.com/Anduin2017/HowToCook)。

## 许可证

[MIT](LICENSE)
