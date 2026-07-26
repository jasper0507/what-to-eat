import { describe, expect, it } from "vitest";

import {
  runeCount,
  utf8ByteLength,
  validatePassword,
  validateUsername,
} from "./validation";

describe("runeCount / utf8ByteLength", () => {
  it("按 Unicode 码点计数，而非 UTF-16 单元", () => {
    expect(runeCount("卤肉饭")).toBe(3);
    expect(utf8ByteLength("卤肉饭")).toBe(9);
  });
});

describe("validateUsername", () => {
  it("接受 3–32 个字母/数字/下划线/连字符（含中文）", () => {
    expect(validateUsername("abc")).toBe(undefined);
    expect(validateUsername("吃饭人_2026-a")).toBe(undefined);
    expect(validateUsername("a".repeat(32))).toBe(undefined);
  });

  it("拒绝过短与过长", () => {
    expect(validateUsername("ab")).toBeDefined();
    expect(validateUsername("a".repeat(33))).toBeDefined();
  });

  it("拒绝空白与非法字符", () => {
    expect(validateUsername("a b c")).toBeDefined();
    expect(validateUsername(" abc")).toBeDefined();
    expect(validateUsername("a@bc")).toBeDefined();
  });
});

describe("validatePassword", () => {
  it("接受 ≥8 字符且 ≤72 字节", () => {
    expect(validatePassword("12345678")).toBe(undefined);
    // 24 个三字节汉字 = 恰好 72 字节
    expect(validatePassword("饭".repeat(24))).toBe(undefined);
  });

  it("拒绝少于 8 个字符", () => {
    expect(validatePassword("1234567")).toBeDefined();
    // 8 个汉字 = 24 字节，字符数满足
    expect(validatePassword("饭".repeat(8))).toBe(undefined);
  });

  it("拒绝超过 72 个 UTF-8 字节", () => {
    expect(validatePassword("饭".repeat(25))).toBeDefined();
    expect(validatePassword("a".repeat(73))).toBeDefined();
  });
});
