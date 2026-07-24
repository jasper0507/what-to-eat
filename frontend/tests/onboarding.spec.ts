import { expect, test, type Page } from "@playwright/test";

async function register(page: Page, username: string) {
  await page.goto("/");
  await page.getByText("注册", { exact: true }).click();
  await page.getByLabel("用户名").fill(username);
  await page.getByLabel("密码").fill("browser-pass-1");
  await page.getByRole("button", { name: "创建 Account" }).click();
  await expect(page.getByRole("heading", { name: "先聊聊你爱吃什么" })).toBeVisible();
}

test("NIM Onboarding builds a weighted Candidate pool without exposing its key", async ({
  page,
}, testInfo) => {
  await register(page, `onboarding_success_${testInfo.project.name}`);

  await page
    .getByLabel("告诉访谈助手你喜欢的 Dish")
    .fill(`我最喜欢番茄炒蛋，浏览器成功_${testInfo.project.name}`);
  await page.getByRole("button", { name: "发送" }).click();

  await expect(page).toHaveURL(/\/candidate-pool$/);
  await expect(page.getByText("番茄炒蛋", { exact: true })).toBeVisible();
  await expect(page.getByText("Preference weight：5.0", { exact: true })).toBeVisible();
  await expect(page.locator("body")).not.toContainText("browser-test-secret");
  expect(
    await page.evaluate(() => JSON.stringify({ ...localStorage, ...sessionStorage })),
  ).not.toContain("browser-test-secret");
});

test("Onboarding conversation survives a refresh", async ({ page }, testInfo) => {
  await register(page, `onboarding_resume_${testInfo.project.name}`);

  const firstMessage = `我喜欢番茄牛腩，浏览器恢复_${testInfo.project.name}`;
  await page.getByLabel("告诉访谈助手你喜欢的 Dish").fill(firstMessage);
  await page.getByRole("button", { name: "发送" }).click();
  await expect(page.getByText("番茄牛腩记下了，再确认一下它有多喜欢？")).toBeVisible();

  await page.reload();
  await expect(page.getByText(firstMessage, { exact: true })).toBeVisible();
  await expect(page.getByText("番茄牛腩记下了，再确认一下它有多喜欢？")).toBeVisible();

  await page.getByLabel("告诉访谈助手你喜欢的 Dish").fill("继续完成恢复测试");
  await page.getByRole("button", { name: "发送" }).click();
  await expect(page).toHaveURL(/\/candidate-pool$/);
  await expect(page.getByText("番茄牛腩", { exact: true })).toBeVisible();
  await expect(page.getByText("Preference weight：4.5", { exact: true })).toBeVisible();
});

test("NIM failure offers retry and manual Catalog fallback", async ({ page }, testInfo) => {
  await register(page, `onboarding_failure_${testInfo.project.name}`);

  await page
    .getByLabel("告诉访谈助手你喜欢的 Dish")
    .fill(`我喜欢柠檬水，浏览器失败_${testInfo.project.name}`);
  await page.getByRole("button", { name: "发送" }).click();

  await expect(page.getByText(/NIM 暂时不可用/)).toBeVisible();
  await expect(page.getByRole("button", { name: "重试上一条" })).toBeVisible();
  await page.getByRole("button", { name: "重试上一条" }).click();
  await expect(page.getByText(/NIM 暂时不可用/)).toBeVisible();

  await page.getByRole("button", { name: "改用手工 Catalog 编辑" }).click();
  await expect(page).toHaveURL(/\/candidate-pool$/);
  await expect(page.getByText("Candidate pool 还是空的", { exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "从 Catalog 添加" })).toBeVisible();
});
