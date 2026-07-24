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

## 导入 HowToCook Catalog

服务启动时可从 HowToCook 的 `dishes` 目录重复导入 Catalog；同一路径始终得到同一 Dish 身份，已有条目会更新名称、Recipe 和分类信息。

```bash
git clone https://github.com/Anduin2017/HowToCook.git
CATALOG_DIR=/path/to/HowToCook/dishes go run ./cmd/server
```

再次使用相同 `CATALOG_DIR` 启动即可安全重跑导入。未设置时服务使用数据库中已有的 Catalog。

## 单节点运行

```bash
docker compose up --build
```

打开 `http://localhost:8080`。SQLite 数据保存在 Compose 命名卷 `what_to_eat_data` 中；生产环境会为会话 Cookie 启用 `Secure`，因此正式部署应通过 HTTPS 访问。
