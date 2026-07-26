import { Compass, Soup } from "lucide-react";

import { copy } from "@/lib/copy";

// Decision 模式标签：Discovery 用全系统唯一的青蓝实底（ADR 0005「一眼可辨」），
// pool pick 用暖底。文本与规格逐字一致；图标不产生可访问文本。
export function StatusTag({ mode }: { mode: "pool" | "discovery" }) {
  if (mode === "discovery") {
    return (
      <span className="status-tag status-tag-discovery">
        <Compass size={14} strokeWidth={2.2} aria-hidden="true" />
        {copy.home.discovery}
      </span>
    );
  }
  return (
    <span className="status-tag status-tag-pool">
      <Soup size={14} strokeWidth={2.2} aria-hidden="true" />
      {copy.home.poolPick}
    </span>
  );
}
