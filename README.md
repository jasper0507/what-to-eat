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

打开 `http://localhost:5173`。用户名须为 3–32 个字母、数字、下划线或连字符（支持中文），密码须为 8–72 个字符。

## 单节点运行

```bash
docker compose up --build
```

打开 `http://localhost:8080`。SQLite 数据保存在 Compose 命名卷 `what_to_eat_data` 中；生产环境会为会话 Cookie 启用 `Secure`，因此正式部署应通过 HTTPS 访问。
