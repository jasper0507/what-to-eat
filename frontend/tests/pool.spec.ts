import { expect, test } from "@playwright/test";

import { registerAccount } from "./browser-api";

test("手工挑菜：搜、入池、改档、移出", async ({ page }, testInfo) => {
  await registerAccount(page, `pool_${testInfo.project.name}`);
  await page.getByRole("link", { name: "自己去挑菜" }).click();
  await expect(page).toHaveURL(/\/candidate-pool$/);
  await expect(
    page.getByRole("heading", { name: "池子", exact: true }),
  ).toBeVisible();
  await expect(page.getByText("这一顿只会从池子里挑。")).toBeVisible();
  await expect(
    page.getByText("在下面搜你爱吃的，一道道放进来。"),
  ).toBeVisible();

  // 搜索无果：给出改写法的指路
  const search = page.getByRole("searchbox", { name: "搜索菜谱库" });
  await search.fill("不存在的自由文本");
  await page.getByRole("button", { name: "搜索" }).click();
  await expect(
    page.getByText("菜谱库里没有这道。换个写法试试，比如只搜两个字。"),
  ).toBeVisible();

  // 搜到 → 入池，默认档位顶尖
  await search.fill("番茄");
  await page.getByRole("button", { name: "搜索" }).click();
  // 入池后池子列表也会出现同名行，结果行必须锚在加菜 region 里
  const resultRow = page
    .getByRole("region", { name: "往池子加菜" })
    .locator("li")
    .filter({ hasText: "番茄炒蛋" });
  await resultRow.getByRole("button", { name: "入池" }).click();
  await expect(
    resultRow.getByRole("button", { name: "已在池里" }),
  ).toBeDisabled();
  const poolRow = page.locator("li").filter({
    has: page.getByRole("button", { name: "改 番茄炒蛋 的档位" }),
  });
  await expect(poolRow).toContainText("顶尖");

  // 点徽标改档：上三档刻度，落「夯」
  await page.getByRole("button", { name: "改 番茄炒蛋 的档位" }).click();
  await expect(
    page.getByRole("button", { name: "拉完了", exact: true }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "夯", exact: true }).click();
  await expect(poolRow).toContainText("夯");

  // 移出 → 回到空池文案
  await page.getByRole("button", { name: "把 番茄炒蛋 移出池子" }).click();
  await expect(
    page.getByText("在下面搜你爱吃的，一道道放进来。"),
  ).toBeVisible();
});
