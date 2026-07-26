import { describe, expect, it } from "vitest";

import { formatMealAt, formatWeight, mealAtISO } from "./format";

describe("formatWeight", () => {
  it("恒为一位小数（规格锁定的 Preference weight：X.X 格式）", () => {
    expect(formatWeight(5)).toBe("5.0");
    expect(formatWeight(1)).toBe("1.0");
    expect(formatWeight(1.3)).toBe("1.3");
    expect(formatWeight(0.7)).toBe("0.7");
    expect(formatWeight(4.5)).toBe("4.5");
  });
});

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
