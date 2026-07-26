import { describe, expect, it } from "vitest";

import { formatMealAt, mealAtISO } from "./format";

describe("formatMealAt", () => {
  it("unix 秒渲染为 zh-CN 年月日时分", () => {
    // 时区随运行环境，断言结构而非具体值
    expect(formatMealAt(1_753_500_000)).toMatch(
      /^\d{4}\/\d{2}\/\d{2} \d{2}:\d{2}$/,
    );
  });
});

describe("mealAtISO", () => {
  it("unix 秒转 ISO 字符串（time 元素 dateTime 属性）", () => {
    expect(mealAtISO(0)).toBe("1970-01-01T00:00:00.000Z");
  });
});
