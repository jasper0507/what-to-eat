# What to Eat

一个帮助单个 Eater 决定一顿 Meal 吃什么的响应式 Web 应用。

## 本地开发

需要 Go 1.26+ 和 Node.js 24+。

```bash
# 终端 1：API 和 SQLite
go run ./cmd/server

# 终端 2：React 开发服务器
cd frontend
npm install
npm run dev
```

打开 `http://localhost:5173`。用户名须为 3–32 个字母、数字、下划线或连字符（支持中文），密码须至少 8 个字符且不超过 72 个 UTF-8 字节。

## NVIDIA NIM Onboarding

Onboarding interview 只由 Go 服务调用 NVIDIA NIM；API key 不会进入前端构建。启动 API 前设置：

```bash
export NVIDIA_API_KEY="..."
go run ./cmd/server
```

默认使用 `https://integrate.api.nvidia.com/v1` 和 `meta/llama-3.1-8b-instruct`。自托管 NIM 可通过 `NIM_BASE_URL` 和 `NIM_MODEL` 覆盖。未配置或 NIM 暂时失败时，访谈页面允许重试，或转到手工 Catalog 搜索与 Candidate pool 编辑。

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

## 单节点运行

```bash
docker compose up --build
```

打开 `http://localhost:8080`。SQLite 数据保存在 Compose 命名卷 `what_to_eat_data` 中；生产环境会为会话 Cookie 启用 `Secure`，因此正式部署应通过 HTTPS 访问。
