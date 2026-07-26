// 旧世界文案的残馀（段 3 访谈换脑时一并退役）：只剩 Onboarding 页与
// 错误兜底还在引用。新旅程各页文案就地内联——锁定成册是第 4 段的事。
export const copy = {
  onboarding: {
    title: "先聊聊你爱吃什么",
    intro: "说出具体的菜和你有多喜欢，访谈助手会帮你把池子搭起来。",
    inputLabel: "告诉访谈助手你喜欢的菜",
    send: "发送",
    retry: "重试上一条",
    manual: "改用手工挑菜",
    seedMessage: "先说一两道你平时真会想吃的具体菜名吧。",
    roleUser: "你",
    roleAssistant: "访谈助手",
    loadingLabel: "正在恢复访谈",
  },
  common: {
    retry: "重试",
  },
  errors: {
    network: "网络不通，检查一下再试",
    unexpected: "服务暂时不可用，稍后再试",
  },
} as const;
