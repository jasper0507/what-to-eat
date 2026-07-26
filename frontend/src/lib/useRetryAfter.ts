import { useMemo, useSyncExternalStore } from "react";

import { ApiError } from "@/api/client";

const noopSubscribe = () => () => {};

const tickSubscribe = (onTick: () => void) => {
  const timer = setInterval(onTick, 1000);
  return () => clearInterval(timer);
};

/**
 * 从 ApiError.retryAtMs（构造时定格的截止时刻）倒数到 0。
 * 时间是外部系统：useSyncExternalStore 每秒取一次快照；无 Retry-After 时恒为 0。
 * 调用方用「> 0」禁用触发按钮。
 */
export function useRetryAfter(error: unknown): number {
  const deadline =
    error instanceof ApiError && error.retryAtMs ? error.retryAtMs : 0;

  const { subscribe, getSnapshot } = useMemo(() => {
    if (deadline === 0) {
      return { subscribe: noopSubscribe, getSnapshot: () => 0 };
    }
    return {
      subscribe: tickSubscribe,
      getSnapshot: () => Math.max(0, Math.ceil((deadline - Date.now()) / 1000)),
    };
  }, [deadline]);

  return useSyncExternalStore(subscribe, getSnapshot);
}
