import type { Rating, Tier } from "@/api/types";

// 情感刻度的唯一出处（CONTEXT.md Taste rating）：标签先行，数字只是实现。
// 入池场景只开上三档；下两档只存在于评分侧，落下即拒绝标记。
export const TIER_LABELS: Record<Rating, string> = {
  1: "拉完了",
  2: "NPC",
  3: "人上人",
  4: "顶尖",
  5: "夯",
};

export const POOL_TIERS: readonly Tier[] = [3, 4, 5];
export const RATING_TIERS: readonly Rating[] = [1, 2, 3, 4, 5];

/** 起步包与手工入池的默认档：上三档取中。 */
export const DEFAULT_POOL_TIER: Tier = 4;
