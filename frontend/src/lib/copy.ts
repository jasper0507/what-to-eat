// 全部锁定 UI 文案的唯一出处（Playwright 规格逐字匹配 + CONTEXT.md 词汇表）。
// 服务端错误信封的 message 永不复制到这里——一律原样渲染 ApiError.message。
//
// 标题层级政策：每页恰一个 h1；任一屏至多一个 h2——
//   decision 态与菜谱页的 h2 = 菜名；pending 态的 h2 = 先评完上次的 Discovery；
//   池子页的 h2 = 从 Catalog 添加。pending 卡内菜名用 h3。页头 logo 不是 heading。
// 图标一律 lucide-react；界面全程禁止表情符号。
export const copy = {
  auth: {
    title: "今天吃什么？",
    intro: "先进入你的 Account，再决定这一顿。",
    loginTab: "登录",
    registerTab: "注册",
    username: "用户名",
    password: "密码",
    loginSubmit: "登录",
    registerSubmit: "创建 Account",
    sessionExpired: "登录已过期，请重新登录。",
  },
  onboarding: {
    title: "先聊聊你爱吃什么",
    intro: "说出具体 Dish 和你有多喜欢，访谈助手会帮你搭好 Candidate pool。",
    inputLabel: "告诉访谈助手你喜欢的 Dish",
    send: "发送",
    retry: "重试上一条",
    manual: "改用手工 Catalog 编辑",
    seedMessage: "先说一两道你平时真会想吃的具体菜名吧。",
    roleUser: "你",
    roleAssistant: "访谈助手",
    loadingLabel: "正在恢复 Onboarding interview",
  },
  home: {
    title: "准备好决定这一顿了吗？",
    readyBadge: "Meal 已就绪",
    readyIntro: "Candidate pool 中已有可用的 Dish。",
    begin: "开始这一顿",
    managePool: "管理 Candidate pool",
    poolHint: "想先调整选择范围？",
    poolPick: "普通 Pool pick",
    discovery: "Discovery · 候选池外探索",
    reroll: "Reroll",
    accept: "就吃这个（Acceptance）",
    pendingTitle: "先评完上次的 Discovery",
    pendingIntro: "每条只需点一下；全部解决后才能开始新的 Decision。",
    mealAtPrefix: "Meal 时间：",
    emptyText: "Candidate pool 为空，还无法开始 Decision",
    emptyCta: "去 Catalog 添加 Dish",
    ratingRecorded: "已记录 Taste rating",
    loadingLabel: "正在恢复 Meal 状态",
    decisionEyebrow: "这一顿的 Decision",
  },
  pool: {
    title: "Candidate pool",
    intro:
      "这里的 Dish 才会参与普通 Decision；Preference weight 越高越常被选中。",
    back: "返回 Meal readiness",
    empty: "Candidate pool 还是空的",
    addTitle: "从 Catalog 添加",
    addIntro: "按名称查找 HowToCook Dish，加入后即可参与 Decision。",
    searchPlaceholder: "输入 Dish 名称，如 番茄",
    searchButton: "搜索",
    searchHint: "输入名称后，Catalog 只会返回已有 Dish。",
    add: "加入",
    added: "已加入",
    remove: "移出",
    addedToast: "已加入 Candidate pool",
    removedToast: "已移出",
    noResults: "Catalog 中没有匹配的 Dish",
    weightPrefix: "Preference weight：",
    loadingLabel: "正在恢复 Candidate pool",
  },
  recipe: {
    title: "Recipe",
    next: "开始下一顿",
    missing: "Recipe 不存在",
    loadingLabel: "正在读取 Recipe",
  },
  notFound: {
    title: "页面不存在",
    intro: "这个地址没有对应的页面。也许该回去决定这一顿。",
    home: "回到首页",
  },
  common: {
    retry: "重试",
    appName: "今天吃什么",
    sessionLoadingLabel: "正在恢复 Account 会话",
    skipToMain: "跳到主要内容",
  },
  errors: {
    network: "网络异常，请检查连接后重试",
    unexpected: "服务暂时不可用，请稍后重试",
  },
} as const;

export function greeting(username: string): string {
  return `你好，${username}`;
}
