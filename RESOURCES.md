# What2Eat v1 验收资源

## Knowledge

- [仓库 README：运行、备份与恢复](./README.md)
  项目的权威操作说明。用于核对环境变量、Compose 启动方式和 SQLite 备份恢复步骤。
- [Docker Compose `up` 官方文档](https://docs.docker.com/reference/cli/docker/compose/up/)
  解释后台启动、强制重建和等待健康状态。用于验证容器生命周期行为。
- [Docker Compose 应用模型](https://docs.docker.com/compose/intro/compose-application-model/)
  解释 `up`、`down`、`logs` 与 `ps`。用于定位部署验收中的常见故障。
- [Docker Compose volumes 官方参考](https://docs.docker.com/reference/compose-file/volumes/)
  解释命名卷的创建和复用。用于理解为什么重建容器后 SQLite 数据应继续存在。
- [Chrome Device Mode 官方文档](https://developer.chrome.com/docs/devtools/device-mode)
  说明移动视口模拟及其局限。用于桌面预检，不能替代最后的真机验收。
- [NVIDIA NIM LLM API 参考](https://docs.nvidia.com/nim/large-language-models/latest/reference/api-reference.html)
  NIM 的 OpenAI 兼容聊天接口参考。用于判断 Onboarding 是否真正穿过服务端集成边界。
- [MDN `Set-Cookie` 参考](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Set-Cookie)
  解释 `Secure`、`HttpOnly` 和 `SameSite`。用于检查生产 Cookie 及 HTTPS 前置条件。

## Wisdom (Communities)

- 本仓库 GitHub Issues
  用于记录可复现的验收失败；包含步骤、预期、实际结果、截图或日志，避免只留下口头结论。

## Gaps

- 当前没有独立的预发布域名和真机矩阵；正式验收时至少需要一个 HTTPS URL 和一台实际手机。
