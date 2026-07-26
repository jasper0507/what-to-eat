import type { Rating } from "@/api/types";

// ADR 0008：标签先行的 1–5 Taste rating；按钮可访问名必须恰为标签本身。
export const RATING_OPTIONS: ReadonlyArray<{ rating: Rating; label: string }> =
  [
    { rating: 1, label: "拉完了" },
    { rating: 2, label: "NPC" },
    { rating: 3, label: "人上人" },
    { rating: 4, label: "顶级" },
    { rating: 5, label: "夯" },
  ];

export const WEIGHT_MIN = 0.1;
export const WEIGHT_MAX = 5;
export const WEIGHT_STEP = 0.1;
export const DEFAULT_ADD_WEIGHT = 1;
