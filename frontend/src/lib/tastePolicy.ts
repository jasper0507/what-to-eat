import type { EatingRecordEntry, Rating } from "@/api/types";

/**
 * Taste / 轻历史政策：刻度形状在 tiers.ts；阈值与可评条件住这里，
 * 避免 History/Home 魔法数字漂移。
 *
 * 注意：UI「最近爱吃」阈值（顶尖+）≠ 服务端 Pool admission（≥ 人上人 / 3）。
 */

/** 最近爱吃：顶尖与夯（≥4）。人上人仍在池内，但不进「爱吃」横条。 */
export const RECENT_FAVORITE_MIN_RATING: Rating = 4;

export function isRecentFavorite(rating: Rating | undefined | null): boolean {
  return rating != null && rating >= RECENT_FAVORITE_MIN_RATING;
}

/** 未评的入口只给每道菜最近一条；池子未就绪前不放按钮，避免闪变。 */
export function canRateHistoryEntry(input: {
  rating?: Rating;
  latestOfDish: boolean;
  poolPending: boolean;
}): boolean {
  return !input.rating && input.latestOfDish && !input.poolPending;
}

/** 轻历史 mode 旁注。 */
export function historyModeNote(
  mode: EatingRecordEntry["mode"],
): string | null {
  switch (mode) {
    case "discovery":
      return "新尝试";
    case "hand_pick":
      return "亲自点的";
    default:
      return null;
  }
}

