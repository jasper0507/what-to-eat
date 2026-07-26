import { expect, test } from "@playwright/test";

import { addCandidatePoolDish, registerAccount } from "./browser-api";

async function seedPoolAndBegin(
  page: import("@playwright/test").Page,
  username: string,
) {
  await registerAccount(page, username);
  for (const dish of ["vegetable_dish/番茄炒蛋.md", "meat_dish/番茄牛腩.md"]) {
    expect(await addCandidatePoolDish(page, dish, 4)).toBe(true);
  }
  await page.goto("/");
  await page.getByRole("button", { name: "开始这一顿" }).click();
  await expect(page.locator('[aria-live="polite"] h2')).toBeVisible();
}

async function burnAllRerolls(page: import("@playwright/test").Page) {
  for (let remaining = 3; remaining >= 1; remaining -= 1) {
    await page
      .getByRole("button", { name: `换一道 · 剩 ${remaining} 次` })
      .click();
  }
  await expect(
    page.getByText("换菜次数用完了。这顿的结局你来定："),
  ).toBeVisible();
}

test("额度烧完的三出口都站上台", async ({ page }, testInfo) => {
  await seedPoolAndBegin(page, `exits_${testInfo.project.name}`);
  await burnAllRerolls(page);

  await expect(page.getByRole("button", { name: "这顿不吃了" })).toBeVisible();
  await expect(page.getByRole("button", { name: "亲自点一道" })).toBeVisible();
  await expect(page.getByRole("button", { name: "就吃这个" })).toBeVisible();
  await expect(
    page.getByRole("button", { name: /^换一道 · 剩/ }),
  ).toHaveCount(0);

  // 出口一：亲自点一道，只在此刻解锁，点谁吃谁
  await page.getByRole("button", { name: "亲自点一道" }).click();
  const pick = page.getByRole("button", { name: "番茄炒蛋" });
  await expect(pick).toBeVisible();
  await pick.click();
  await expect(page).toHaveURL(/\/recipes\?dish_id=/);
  await expect(page.getByRole("heading", { level: 1 })).toHaveText("番茄炒蛋");
});

test("出口二：这顿不吃了，舞台回到就绪", async ({ page }, testInfo) => {
  await seedPoolAndBegin(page, `abandon_${testInfo.project.name}`);
  await burnAllRerolls(page);

  await page.getByRole("button", { name: "这顿不吃了" }).click();
  await expect(
    page.getByRole("heading", { name: "这一顿，交给池子。" }),
  ).toBeVisible();
});

test("出口三：认了，就吃台上这道", async ({ page }, testInfo) => {
  await seedPoolAndBegin(page, `settle_${testInfo.project.name}`);
  const dishName = page.locator('[aria-live="polite"] h2');
  await burnAllRerolls(page);

  const standing = await dishName.textContent();
  await page.getByRole("button", { name: "就吃这个" }).click();
  await expect(page).toHaveURL(/\/recipes\?dish_id=/);
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(standing!);
});
