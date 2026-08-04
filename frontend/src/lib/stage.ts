import type { ApiError } from "@/api/client";
import type { Decision, MealState, PendingRating } from "@/api/types";

/**
 * Decision stage 纯模型（CONTEXT.md）：把 MealState + Reroll budget
 * 解释成舞台变体。HomePage 只做 adapter（model → JSX）。
 */
export type StageView =
  | { kind: "empty_pool" }
  | { kind: "pending_ratings"; pendingRatings: PendingRating[] }
  | { kind: "ready" }
  | {
      kind: "reveal";
      decision: Decision;
      rerollsRemaining: number;
    }
  | {
      kind: "exhausted";
      decision: Decision;
      rerollsRemaining: number;
    };

/** MealState → 舞台变体。额度用尽时仍在 active_decision，由 remaining 分叉。 */
export function toStage(state: MealState): StageView {
  switch (state.status) {
    case "candidate_pool_empty":
      return { kind: "empty_pool" };
    case "pending_ratings":
      return {
        kind: "pending_ratings",
        pendingRatings: state.pending_ratings,
      };
    case "ready":
      return { kind: "ready" };
    case "active_decision": {
      const remaining = state.rerolls_remaining;
      if (isRerollBudgetExhausted(remaining)) {
        return {
          kind: "exhausted",
          decision: state.decision,
          rerollsRemaining: remaining,
        };
      }
      return {
        kind: "reveal",
        decision: state.decision,
        rerollsRemaining: remaining,
      };
    }
  }
}

/** Reroll budget 用尽：三出口解锁。 */
export function isRerollBudgetExhausted(remaining: number): boolean {
  return remaining <= 0;
}

export function isDiscoveryMode(mode: Decision["mode"]): boolean {
  return mode === "discovery";
}

/** Decision stage 上 Discovery 揭示的徽章文案。 */
export const DISCOVERY_STAGE_BADGE = "池子外的新尝试";

/** 这些 409 语义是「界面陈旧」而非失败：hooks 已 invalidate，不当错误展示。 */
const SUPPRESSED_STAGE_CODES = new Set([
  "pending_ratings",
  "candidate_pool_empty",
  "reroll_budget_exhausted",
]);

export function shouldSuppressStageError(error: unknown): boolean {
  if (
    error !== null &&
    typeof error === "object" &&
    "code" in error &&
    typeof (error as ApiError).code === "string"
  ) {
    return SUPPRESSED_STAGE_CODES.has((error as ApiError).code);
  }
  return false;
}
