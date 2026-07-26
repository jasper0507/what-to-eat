import * as React from "react";

import { cn } from "@/lib/utils";

// 白底 + 发丝边框落在暖纸上，自然分层；聚焦环用品牌陶色。
function Input({ className, ...props }: React.ComponentProps<"input">) {
  return (
    <input
      data-slot="input"
      className={cn(
        "h-10 w-full min-w-0 rounded-md border border-input bg-card px-3 text-[1rem]/[1.6] text-foreground transition-[border-color,box-shadow] duration-150 outline-none placeholder:text-muted-foreground md:text-base",
        "focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/25",
        "aria-invalid:border-destructive/50 aria-invalid:ring-destructive/20",
        "disabled:cursor-not-allowed disabled:opacity-50",
        className,
      )}
      {...props}
    />
  );
}

export { Input };
