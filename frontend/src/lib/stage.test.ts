import { describe, expect, it } from "vitest";

import {
  isDiscoveryMode,
  isRerollBudgetExhausted,
  shouldSuppressStageError,
  toStage,
} from "./stage";

const dish = {
  id: "meat_dish/a.md",
  name: "宫保鸡丁",
  category: "meat_dish",
};

const decision = {
  id: 1,
  meal_id: 2,
  mode: "pool" as const,
  reason: "你的夯。",
  dish,
};

describe("toStage", () => {
  it("maps empty pool and pending ratings", () => {
    expect(toStage({ status: "candidate_pool_empty" })).toEqual({
      kind: "empty_pool",
    });
    expect(
      toStage({
        status: "pending_ratings",
        pending_ratings: [
          { id: 1, meal_id: 1, meal_at: 0, dish },
        ],
      }),
    ).toEqual({
      kind: "pending_ratings",
      pendingRatings: [{ id: 1, meal_id: 1, meal_at: 0, dish }],
    });
  });

  it("maps ready and reveal", () => {
    expect(toStage({ status: "ready" })).toEqual({ kind: "ready" });
    expect(
      toStage({
        status: "active_decision",
        decision,
        rerolls_remaining: 2,
      }),
    ).toEqual({
      kind: "reveal",
      decision,
      rerollsRemaining: 2,
    });
  });

  it("splits exhausted when reroll budget is zero", () => {
    expect(
      toStage({
        status: "active_decision",
        decision,
        rerolls_remaining: 0,
      }),
    ).toEqual({
      kind: "exhausted",
      decision,
      rerollsRemaining: 0,
    });
  });
});

describe("stage helpers", () => {
  it("detects budget exhaustion and discovery mode", () => {
    expect(isRerollBudgetExhausted(0)).toBe(true);
    expect(isRerollBudgetExhausted(1)).toBe(false);
    expect(isDiscoveryMode("discovery")).toBe(true);
    expect(isDiscoveryMode("pool")).toBe(false);
  });

  it("suppresses stale stage error codes", () => {
    expect(
      shouldSuppressStageError({ code: "pending_ratings", message: "x" }),
    ).toBe(true);
    expect(
      shouldSuppressStageError({
        code: "reroll_budget_exhausted",
        message: "x",
      }),
    ).toBe(true);
    expect(shouldSuppressStageError({ code: "network_error", message: "x" })).toBe(
      false,
    );
    expect(shouldSuppressStageError(new Error("boom"))).toBe(false);
  });
});
