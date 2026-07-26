import { expect, test } from "@playwright/test";

import { addCandidatePoolDish, registerAccount } from "./browser-api";

test("还没吃过时，历史页指回主页", async ({ page }, testInfo) => {
  await registerAccount(page, `nohistory_${testInfo.project.name}`);
  await page.getByRole("link", { name: "吃过的" }).click();
  await expect(page).toHaveURL(/\/history$/);
  await expect(page.getByRole("heading", { name: "吃过的" })).toBeVisible();
  await expect(page.getByText("回主页，开一顿。")).toBeVisible();
});

test("吃过的被记下，随时补一句评价", async ({ page }, testInfo) => {
  await registerAccount(page, `history_${testInfo.project.name}`);
  // 池满四道，「池小」探索信号归零：接受的必是池内菜，评分路径确定
  for (const dish of [
    "vegetable_dish/番茄炒蛋.md",
    "meat_dish/番茄牛腩.md",
    "vegetable_dish/番茄豆腐.md",
    "vegetable_dish/番茄土豆.md",
  ]) {
    expect(await addCandidatePoolDish(page, dish, 4)).toBe(true);
  }
  await page.goto("/");
  await page.getByRole("button", { name: "开始这一顿" }).click();
  const dishName = page.locator('[aria-live="polite"] h2');
  const accepted = await dishName.textContent();
  await page.getByRole("button", { name: "就吃这个" }).click();
  await expect(page).toHaveURL(/\/recipes\?dish_id=/);

  await page.getByRole("link", { name: "吃过的" }).click();
  await expect(
    page.getByText("最近的每一顿都记着。想补一句评价，随时。"),
  ).toBeVisible();
  const record = page.locator("li").filter({ hasText: accepted! }).first();
  await expect(record).toBeVisible();

  // 池内接受未评分：补一句评价 → 五档刻度 → 落「顶尖」徽标
  await record.getByRole("button", { name: "补一句评价" }).click();
  await record.getByRole("button", { name: "顶尖", exact: true }).click();
  await expect(record).toContainText("顶尖");
  await expect(
    record.getByRole("button", { name: "补一句评价" }),
  ).toHaveCount(0);

  // 记录行点菜名可回菜谱页
  await record.getByRole("link", { name: accepted! }).click();
  await expect(page).toHaveURL(/\/recipes\?dish_id=/);
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(accepted!);
});
