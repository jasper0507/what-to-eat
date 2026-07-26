// 格式化器提升到模块级，避免在渲染循环里重复构造。
const mealAtFormatter = new Intl.DateTimeFormat("zh-CN", {
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  hour12: false,
});

/** 规格锁定格式：一位小数（如 Preference weight：1.3 / 5.0）。 */
export function formatWeight(weight: number): string {
  return weight.toFixed(1);
}

export function formatMealAt(unixSeconds: number): string {
  return mealAtFormatter.format(new Date(unixSeconds * 1000));
}

/** <time dateTime> 属性用。 */
export function mealAtISO(unixSeconds: number): string {
  return new Date(unixSeconds * 1000).toISOString();
}
