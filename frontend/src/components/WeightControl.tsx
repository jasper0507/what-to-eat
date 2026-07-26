import { Segmented } from "antd";

import type { PoolDish, Tier } from "@/api/types";

// 过渡件：数字滑杆已按 ADR-0022 处决，wire 只说档位语言。本组件只为让旧
// 池子页在第 2 段重建前保持可用，随 antd 迁移一并退役。
const tierLabels: Record<Tier, string> = {
  3: "人上人",
  4: "顶尖",
  5: "夯",
};

export function WeightControl({
  dish,
  disabled,
  onCommit,
}: {
  dish: PoolDish;
  disabled?: boolean;
  onCommit: (value: Tier) => void;
}) {
  return (
    <Segmented
      value={dish.tier}
      disabled={disabled}
      options={([3, 4, 5] as const).map((tier) => ({
        label: tierLabels[tier],
        value: tier,
      }))}
      onChange={(value) => {
        if (value !== dish.tier) {
          onCommit(value);
        }
      }}
    />
  );
}
