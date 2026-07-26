import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

// 非理想态的统一容器：一个安静的短句盒子，错误也不喧哗。
// 服务端错误信封的 message 原样渲染，永不在前端复写。
export function Notice({
  tone = "neutral",
  children,
  onRetry,
  className,
}: {
  tone?: "neutral" | "error";
  children: ReactNode;
  onRetry?: () => void;
  className?: string;
}) {
  return (
    <div
      role={tone === "error" ? "alert" : undefined}
      className={cn(
        "flex flex-wrap items-center justify-between gap-x-4 gap-y-2 rounded-md border px-3 py-2 text-sm",
        tone === "error"
          ? "border-destructive/25 bg-destructive/5 text-destructive"
          : "border-border bg-secondary text-foreground",
        className,
      )}
    >
      <span>{children}</span>
      {onRetry ? (
        <Button
          size="sm"
          variant="outline"
          className="h-7 bg-transparent"
          onClick={onRetry}
        >
          再试一次
        </Button>
      ) : null}
    </div>
  );
}
