// 客户端归一化错误的兜底文案（apiFetch 专用）。
// 服务端错误信封的 message 永不复制到这里——一律原样渲染 ApiError.message。
export const copy = {
  errors: {
    network: "网络不通，检查一下再试",
    unexpected: "服务暂时不可用，稍后再试",
  },
} as const;
