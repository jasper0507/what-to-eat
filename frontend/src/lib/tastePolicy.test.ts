import { describe, expect, it } from "vitest";

import {
  canRateHistoryEntry,
  historyModeNote,
  isRecentFavorite,
} from "./tastePolicy";

describe("isRecentFavorite", () => {
  it("starts at 顶尖, not 人上人", () => {
    expect(isRecentFavorite(3)).toBe(false);
    expect(isRecentFavorite(4)).toBe(true);
    expect(isRecentFavorite(5)).toBe(true);
    expect(isRecentFavorite(undefined)).toBe(false);
  });
});

describe("canRateHistoryEntry", () => {
  it("only latest unrated when pool is ready", () => {
    expect(
      canRateHistoryEntry({
        rating: undefined,
        latestOfDish: true,
        poolPending: false,
      }),
    ).toBe(true);
    expect(
      canRateHistoryEntry({
        rating: 4,
        latestOfDish: true,
        poolPending: false,
      }),
    ).toBe(false);
    expect(
      canRateHistoryEntry({
        rating: undefined,
        latestOfDish: false,
        poolPending: false,
      }),
    ).toBe(false);
    expect(
      canRateHistoryEntry({
        rating: undefined,
        latestOfDish: true,
        poolPending: true,
      }),
    ).toBe(false);
  });
});

describe("historyModeNote", () => {
  it("labels discovery and hand_pick only", () => {
    expect(historyModeNote("pool")).toBeNull();
    expect(historyModeNote("discovery")).toBe("新尝试");
    expect(historyModeNote("hand_pick")).toBe("亲自点的");
  });
});
