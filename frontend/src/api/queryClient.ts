import { MutationCache, QueryCache, QueryClient } from "@tanstack/react-query";

import { ApiError } from "./client";
import { sessionKey } from "./keys";

// 会话过期的提示标志。不进 Web Storage——服务端是唯一事实源，
// 这里只是把「过期跳转」与「登录页提示」解耦的内存信号。
// 登录页只读取（幂等，StrictMode 安全）；下次认证成功时清除。
let sessionExpired = false;

export function markSessionExpired(): void {
  sessionExpired = true;
}

export function peekSessionExpired(): boolean {
  return sessionExpired;
}

export function clearSessionExpired(): void {
  sessionExpired = false;
}

// 全局 401 漏斗：只认 code === "unauthorized"（登录失败的 invalid_credentials 也是 401，
// 不能按裸状态码判断）。置 session 为 null 后，RequireSession 会声明式跳转 /login。
function handleUnauthorized(error: unknown): void {
  if (error instanceof ApiError && error.code === "unauthorized") {
    markSessionExpired();
    queryClient.setQueryData(sessionKey, null);
  }
}

export const queryClient = new QueryClient({
  queryCache: new QueryCache({ onError: handleUnauthorized }),
  mutationCache: new MutationCache({ onError: handleUnauthorized }),
  defaultOptions: {
    queries: {
      // 本地 Go 服务，确定性 409 不该重试三遍；刷新语义 = 挂载时重取
      retry: false,
      refetchOnWindowFocus: false,
      refetchOnReconnect: false,
      staleTime: 0,
    },
  },
});
