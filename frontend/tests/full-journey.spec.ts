import { expect, test, type Locator } from "@playwright/test";

async function expectUsableTapTarget(locator: Locator) {
  const box = await locator.boundingBox();
  expect(box).not.toBeNull();
  expect(box!.width).toBeGreaterThanOrEqual(44);
  expect(box!.height).toBeGreaterThanOrEqual(44);
}

test("Eater completes the full v1 journey with a clear primary action", async ({
  page,
}, testInfo) => {
  await page.goto("/");
  await page.getByText("注册", { exact: true }).click();
  await page.getByLabel("用户名").fill(`journey_${testInfo.project.name}`);
  await page.getByLabel("密码").fill("browser-pass-1");
  const registration = page.waitForResponse(
    (response) => response.url().endsWith("/api/auth/register"),
  );
  await page.getByRole("button", { name: "创建 Account" }).click();
  const sessionCookie = await (await registration).headerValue("set-cookie");
  expect(sessionCookie).toContain("HttpOnly");
  expect(sessionCookie).toContain("Secure");
  expect(sessionCookie).toContain("SameSite=Lax");

  await page.getByLabel("告诉访谈助手你喜欢的 Dish").fill("运行完整旅程");
  await page.getByRole("button", { name: "发送" }).click();
  await expect(page).toHaveURL(/\/candidate-pool$/);
  await expect(page.getByText("番茄炒蛋", { exact: true })).toBeVisible();
  await expect(page.getByText("番茄牛腩", { exact: true })).toBeVisible();

  await page.getByRole("link", { name: "返回 Meal readiness" }).click();
  const begin = page.getByRole("button", { name: "开始这一顿" });
  await expectUsableTapTarget(begin);
  await expect(page.locator("main .ant-btn-primary")).toHaveCount(1);
  await begin.click();

  await expect(page.getByText("普通 Pool pick", { exact: true })).toBeVisible();
  const decision = page.getByRole("heading", { level: 2 });
  const firstDish = await decision.textContent();
  const reroll = page.getByRole("button", { name: "Reroll" });
  const accept = page.getByRole("button", { name: "就吃这个（Acceptance）" });
  await expectUsableTapTarget(reroll);
  await expectUsableTapTarget(accept);
  await expect(page.locator("main .ant-btn-primary")).toHaveCount(1);

  await reroll.click();
  await expect.poll(() => decision.textContent()).not.toBe(firstDish);
  const acceptedPoolDish = await decision.textContent();
  await accept.click();
  await expect(page).toHaveURL(/\/recipes\?dish_id=/);
  await expect(page.getByRole("heading", { level: 2 })).toHaveText(acceptedPoolDish!);

  await page.getByRole("link", { name: "开始下一顿" }).click();
  await page.getByRole("button", { name: "开始这一顿" }).click();
  await expect(page.getByText("Discovery · 候选池外探索", { exact: true })).toBeVisible();
  const discoveryDish = await page.getByRole("heading", { level: 2 }).textContent();
  await page.getByRole("button", { name: "就吃这个（Acceptance）" }).click();
  await expect(page.getByRole("heading", { level: 2 })).toHaveText(discoveryDish!);

  await page.getByRole("link", { name: "开始下一顿" }).click();
  await expect(page.getByRole("heading", { name: "先评完上次的 Discovery" })).toBeVisible();
  const rating = page.getByRole("button", { name: "夯", exact: true });
  await expectUsableTapTarget(rating);
  await rating.click();
  await expect(page.getByText("Meal 已就绪", { exact: true })).toBeVisible();

  await page.getByRole("link", { name: "管理 Candidate pool" }).click();
  await expect(page.getByText(discoveryDish!, { exact: true })).toBeVisible();
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
  ).toBe(true);
});
