# What to Eat

一个帮助单个 Eater 决定一顿 Meal 吃什么的响应式 Web 应用。

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

打开 `http://localhost:5173`。用户名须为 3–32 个字母、数字、下划线或连字符（支持中文），密码须至少 8 个字符且不超过 72 个 UTF-8 字节。

### 前端质量门

```bash
cd frontend
npm run typecheck     # tsc 项目引用：src、tests、配置一起检查
npm run lint          # ESLint（类型感知 + react-hooks + @tanstack/query）
npm run format:check  # Prettier（tests/ 不参与格式化，保持验收规格逐字节稳定）
npm run test          # Vitest 纯逻辑单测（API client、校验、格式化）
npm run test:browser  # Playwright 全栈验收（自动构建并启动真实 Go testserver）
```

若本机 shell 设置了 HTTP 代理（如 `http_proxy=http://127.0.0.1:7897`），Playwright 的
健康检查可能不识别 `127.*` 通配写法而走代理导致 502。此时用
`NO_PROXY="127.0.0.1,localhost" npm run test:browser` 运行。

## 导入 HowToCook Catalog

服务启动时可从 HowToCook 的 `dishes` 目录重复导入 Catalog；同一路径始终得到同一 Dish 身份，已有条目会更新名称、Recipe 和分类信息。

```bash
git clone https://github.com/Anduin2017/HowToCook.git
CATALOG_DIR=/path/to/HowToCook/dishes go run ./cmd/server
```

再次使用相同 `CATALOG_DIR` 启动即可安全重跑导入。未设置时服务使用数据库中已有的 Catalog。

## Discovery 触发条件

Discovery pressure 的三个透明信号分别是 Candidate pool 不超过 3 道、完整 Cooldown 后不超过 1 道可选，以及当前或最近 3 个已接受 Meal 中至少 2 次 Reroll。默认满足其中 2 项即触发，每个 Meal 最多展示 2 个 Discovery。

嵌入应用时可从 `server.DefaultDiscoveryConfig()` 取得默认值，通过 `server.Config.Discovery` 调整阈值或关闭 Discovery。

部署时同一组阈值可经环境变量覆盖，无需重编译：`DISCOVERY_ENABLED`、`DISCOVERY_MAX_POOL_SIZE`、`DISCOVERY_MAX_ELIGIBLE_DISHES`、`DISCOVERY_MIN_REROLLS`、`DISCOVERY_REQUIRED_SIGNALS`、`DISCOVERY_RECENT_MEAL_WINDOW`、`DISCOVERY_MAX_DISCOVERIES_PER_MEAL`。

## 单节点运行

```bash
install -m 600 .env.example .env
# 编辑 .env，把 SESSION_SECRET 换成至少 32 字节的随机值：
# openssl rand -base64 32
docker compose up --build -d
docker compose ps
```

镜像固定打包 HowToCook Catalog 的
`c05758fa661ac4efa0361a987b700a351a22159b` 版本，`/api` 与 React SPA
由同一个非 root 容器在 `8080` 端口提供。SQLite 数据保存在 Compose 命名卷
`what_to_eat_data` 中。生产会话 Cookie 启用 `Secure`、`HttpOnly` 和
`SameSite=Lax`，因此浏览器访问应由同机反向代理提供 HTTPS。

`SESSION_SECRET` 缺失时 Compose 会直接拒绝启动；
会话秘密短于 32 字节或 SQLite 卷不可写时，应用会输出明确错误并退出。
不要把 `.env` 加入镜像或版本库。

## 备份与恢复

备份前短暂停止应用，确保复制的是一个一致的 SQLite 文件。下面的目录名可换成实际时间：

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

恢复时先还原对应环境配置（尤其是 `SESSION_SECRET`），停止应用但不要使用
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

恢复后用原 Account 登录，确认 Candidate pool、Meal/Pending rating 和 Taste rating
状态仍可访问。数据库备份与 `env` 文件必须作为一组保管；后者包含敏感配置。
