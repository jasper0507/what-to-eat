import { expect, test, type Page } from "@playwright/test";

import {
  addCandidatePoolDish,
  registerAccount,
  removeCandidatePoolDish,
} from "./browser-api";

test("还没吃过时，历史页指回主页", async ({ page }, testInfo) => {
  await registerAccount(page, `nohistory_${testInfo.project.name}`);
  await page.getByRole("link", { name: "吃过的" }).click();
  await expect(page).toHaveURL(/\/history$/);
  await expect(page.getByRole("heading", { name: "吃过的" })).toBeVisible();
  await expect(page.getByText("回主页，开一顿。")).toBeVisible();
});

test("不在池里的菜才用「补一句评价」邀请", async ({ page }, testInfo) => {
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

  // 把吃过的这道移出池子：它的记录才会亮「补一句评价」文字邀请
  const acceptedId = decodeURIComponent(
    new URL(page.url()).searchParams.get("dish_id")!,
  );
  expect(await removeCandidatePoolDish(page, acceptedId)).toBe(true);

  await page.getByRole("link", { name: "吃过的" }).click();
  await expect(
    page.getByText("最近的每一顿都记着。想补一句评价，随时。"),
  ).toBeVisible();
  const record = page.locator("li").filter({ hasText: accepted! }).first();
  await expect(record).toBeVisible();

  // 补一句评价 → 五档刻度 → 落「顶尖」徽标
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

// 单菜池吃两顿同一道菜：第二顿靠全零放宽复活，探索揭示用换一道跳过
async function acceptPoolDish(page: Page, name: string) {
  await page.getByRole("button", { name: "开始这一顿" }).click();
  const dishName = page.locator('[aria-live="polite"] h2');
  await expect(dishName).toBeVisible();
  for (let remaining = 3; ; remaining -= 1) {
    if ((await dishName.textContent()) === name) {
      break;
    }
    if (remaining === 0) {
      throw new Error(`额度内没等到池内菜 ${name} 登台`);
    }
    await page
      .getByRole("button", { name: `换一道 · 剩 ${remaining} 次` })
      .click();
    await expect(
      remaining === 1
        ? page.getByText("换菜次数用完了。这顿的结局你来定：")
        : page.getByRole("button", { name: `换一道 · 剩 ${remaining - 1} 次` }),
    ).toBeVisible();
  }
  await page.getByRole("button", { name: "就吃这个" }).click();
  await expect(page).toHaveURL(/\/recipes\?dish_id=/);
  await page.getByRole("link", { name: "开始下一顿" }).click();
  await expect(
    page.getByRole("heading", { name: "这一顿，交给池子。" }),
  ).toBeVisible();
}

test("同菜吃两顿只留一个评价口，池内菜亮现役档位", async ({
  page,
}, testInfo) => {
  await registerAccount(page, `dedup_${testInfo.project.name}`);
  expect(
    await addCandidatePoolDish(page, "vegetable_dish/番茄炒蛋.md", 5),
  ).toBe(true);
  await page.goto("/");
  await acceptPoolDish(page, "番茄炒蛋");
  await acceptPoolDish(page, "番茄炒蛋");

  await page.getByRole("link", { name: "吃过的" }).click();
  const rows = page
    .getByRole("region", { name: "最近吃过" })
    .locator("li")
    .filter({ hasText: "番茄炒蛋" });
  await expect(rows).toHaveCount(2);

  // 一菜一口：零个文字催评，唯一入口是最近一条上的现役档位徽标
  await expect(
    page.getByRole("button", { name: "补一句评价" }),
  ).toHaveCount(0);
  const badge = page.getByRole("button", { name: "改 番茄炒蛋 的评价" });
  await expect(badge).toHaveCount(1);
  await expect(badge).toContainText("夯");

  // 点开五档（下两档在场 = 剔池路径还在），改评「人上人」
  await badge.click();
  await expect(
    page.getByRole("button", { name: "拉完了", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "人上人", exact: true }).click();

  // 最近一条落了实心徽标，入口全部退场；老记录保持留白
  await expect(rows.first()).toContainText("人上人");
  await expect(
    page.getByRole("button", { name: "改 番茄炒蛋 的评价" }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "补一句评价" }),
  ).toHaveCount(0);
});
