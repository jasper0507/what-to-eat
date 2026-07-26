# 0024. UI 地基换血：Tailwind v4 + shadcn/ui，antd 全退役

日期：2026-07-27
状态：已接受（取代 ADR-0012 第 3 条「Ant Design v5 为主组件库」；
redesign-brief 段 0–2 的既成事实追认）

## 背景

v1 用 antd v5 快速拼装界面，代价在重设计时集中爆发：默认蓝的组件观感与
「陶色 + 衬线中文」的品牌方向硬冲；主题定制要跟 antd token 体系搏斗；
Form/Modal/message 的交互范式（弹窗注册、全局 toast）多条已被用户
列入已毙清单。redesign 段 0 用「登录页一屏样板」验证了新地基，段 2 铺开
全部页面后 antd（连同 motion 动效库）依赖已从 package.json 卸载。

## 决定

1. **样式地基**：Tailwind CSS v4，design token 唯一来源
   `frontend/src/styles/globals.css`（品牌陶色、衬线/无衬线字组、
   动效节拍全在其中；深色 token 预留不出货）。
2. **组件**：shadcn/ui 按需落地到 `frontend/src/components/ui/`
   （代码进仓库、可改可删，不是依赖）；自研件（TierBadge/TierScale、
   Notice、AppShell、舞台/揭示）直接用 Tailwind 写。
3. **动效**：CSS keyframes（globals.css 注册，`animate-*` 工具类挂用），
   不引 JS 动效库；揭示序列以 `key={decision.id}` 重放，可被连续
   Reroll 一拍打断。
4. **图标**：Lucide，全应用零 emoji。
5. antd、@ant-design/*、motion 不得回归 package.json。

## 后果

- 视觉决策全部落在自己手里，token 改一处全应用生效；代价是失去 antd
  的现成复杂组件（Table/DatePicker 等）——当前产品面不需要，若未来需要
  按 shadcn 方式逐件落地。
- 组件代码进仓库意味着升级靠手，不靠 npm；shadcn 生成件视同自家代码维护。
- ADR-0012 其余决定（Go/Gin、Vite+React+TS、SQLite、React Router）不受
  影响，继续有效。
