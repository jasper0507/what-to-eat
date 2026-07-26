import { expect, test, type Page } from "@playwright/test";

import { addCandidatePoolDish, registerAccount } from "./browser-api";

// 探索是按信号数掷概率（单菜池 2 信号 50%，攒够 Reroll 后 3 信号 75%），
// 不是必然首揭。一顿 4 次揭示找不到就弃顿再开，两顿漏率约万分之二。
async function revealDiscovery(page: Page): Promise<string> {
  const badge = page.getByText("池子外的新尝试");
  const dishName = page.locator('[aria-live="polite"] h2');

  for (let meal = 0; meal < 2; meal += 1) {
    await page.getByRole("button", { name: "开始这一顿" }).click();
    await expect(dishName).toBeVisible();
    for (let remaining = 3; remaining >= 0; remaining -= 1) {
      if (await badge.isVisible()) {
        return (await dishName.textContent())!;
      }
      if (remaining === 0) {
        break;
      }
      await page
        .getByRole("button", { name: `换一道 · 剩 ${remaining} 次` })
        .click();
      await expect(
        remaining === 1
          ? page.getByText("换菜次数用完了。这顿的结局你来定：")
          : page.getByRole("button", {
              name: `换一道 · 剩 ${remaining - 1} 次`,
            }),
      ).toBeVisible();
    }
    await page.getByRole("button", { name: "这顿不吃了" }).click();
    await expect(
      page.getByRole("heading", { name: "这一顿，交给池子。" }),
    ).toBeVisible();
  }
  throw new Error("两顿共 8 次揭示都没等来池子外的新尝试");
}

test("池子外的新尝试打明牌，吃完先说说上一顿", async ({
  page,
}, testInfo) => {
  await registerAccount(page, `discovery_${testInfo.project.name}`);
  expect(
    await addCandidatePoolDish(page, "vegetable_dish/番茄炒蛋.md", 5),
  ).toBe(true);
  await page.goto("/");

  const discoveryName = await revealDiscovery(page);
  expect(discoveryName).toBeTruthy();
  expect(discoveryName).not.toBe("番茄炒蛋");

  // 接受 → 菜谱页 → 回来被待评分拦截
  await page.getByRole("button", { name: "就吃这个" }).click();
  await expect(page).toHaveURL(/\/recipes\?dish_id=/);
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(
    discoveryName,
  );
  await page.getByRole("link", { name: "开始下一顿" }).click();
  await expect(
    page.getByRole("heading", { name: "先说说上一顿。" }),
  ).toBeVisible();
  await expect(
    page.getByText("好不好吃，一句话的事；说完就能开新的一顿。"),
  ).toBeVisible();
  // 桌面右栏「最近吃过」也会出现同名（移动端隐藏但仍在 DOM）；
  // 拦截卡在栏之前，first 即卡上的菜名
  await expect(page.getByText(discoveryName).first()).toBeVisible();

  // 五档情感刻度一个不少
  for (const label of ["拉完了", "NPC", "人上人", "顶尖", "夯"]) {
    await expect(
      page.getByRole("button", { name: label, exact: true }),
    ).toBeVisible();
  }

  // 评「夯」→ 拦截解除，回到就绪舞台
  await page.getByRole("button", { name: "夯", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "这一顿，交给池子。" }),
  ).toBeVisible();

  // 夯过的新尝试自动入池，档位如实
  await page.getByRole("link", { name: "池子", exact: true }).click();
  const row = page.locator("li").filter({ hasText: discoveryName }).first();
  await expect(row).toBeVisible();
  await expect(row).toContainText("夯");
});
