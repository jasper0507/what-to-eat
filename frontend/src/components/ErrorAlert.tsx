import { Alert, Button } from "antd";
import { CircleAlert, Hourglass } from "lucide-react";

import { ApiError } from "@/api/client";
import { copy } from "@/lib/copy";

// 唯一错误约定：内联 Alert、服务端原文、永不 toast。
// rate_limited 渲染为 warning（限流不是失败）；倒计时秒数由调用方经
// useRetryAfter 传入（调用方同时用它禁用触发按钮）。
export function ErrorAlert({
  error,
  onRetry,
  retryRemaining = 0,
}: {
  error: unknown;
  onRetry?: () => void;
  retryRemaining?: number;
}) {
  if (!error) {
    return null;
  }
  const apiError = error instanceof ApiError ? error : undefined;
  const throttled = apiError?.code === "rate_limited";
  const baseMessage =
    error instanceof Error ? error.message : copy.errors.unexpected;
  const message =
    throttled && retryRemaining > 0
      ? `${baseMessage}（${retryRemaining} 秒后可重试）`
      : baseMessage;

  return (
    <Alert
      type={throttled ? "warning" : "error"}
      showIcon
      icon={throttled ? <Hourglass size={18} /> : <CircleAlert size={18} />}
      message={message}
      action={
        onRetry ? (
          <Button size="small" onClick={onRetry}>
            {copy.common.retry}
          </Button>
        ) : undefined
      }
    />
  );
}
