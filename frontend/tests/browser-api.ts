import { expect, type Page } from "@playwright/test";

/** 经 UI 注册一个新账号并等待落到主界面（空池欢迎）。 */
export async function registerAccount(page: Page, username: string) {
  await page.goto("/login");
  await page.getByRole("button", { name: "注册" }).click();
  await page.getByRole("textbox", { name: "用户名" }).fill(username);
  await page.getByRole("textbox", { name: "密码" }).fill("browser-pass-1");
  await page.getByRole("button", { name: "注册" }).click();
  await expect(
    page.getByRole("heading", { name: "池子还空着。" }),
  ).toBeVisible();
}

/** 绕过 UI 把菜移出池子（造局用）。 */
export function removeCandidatePoolDish(page: Page, dishID: string) {
  return page.evaluate(
    async (dish) =>
      (
        await fetch(
          `/api/candidate-pool/dishes?dish_id=${encodeURIComponent(dish)}`,
          { method: "DELETE" },
        )
      ).ok,
    dishID,
  );
}

/** 绕过 UI 往池子塞菜（造局用）；tier 只认上三档 3/4/5。 */
export function addCandidatePoolDish(
  page: Page,
  dishID: string,
  tier: number,
) {
  return page.evaluate(
    async (dish) =>
      (
        await fetch("/api/candidate-pool/dishes", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(dish),
        })
      ).ok,
    { dish_id: dishID, tier },
  );
}
