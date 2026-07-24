import { expect, test } from "@playwright/test";

test("Discovery recipe handoff leads to labeled Pending rating before the next Decision", async ({
  page,
}, testInfo) => {
  await page.goto("/");
  await page.getByText("注册", { exact: true }).click();
  await page.getByLabel("用户名").fill(`pending_${testInfo.project.name}`);
  await page.getByLabel("密码").fill("browser-pass-1");
  await page.getByRole("button", { name: "创建 Account" }).click();
  await expect(page.getByRole("heading", { name: "先聊聊你爱吃什么" })).toBeVisible();

  const addResponse = await page.request.post("/api/candidate-pool/dishes", {
    data: {
      dish_id: "vegetable_dish/番茄炒蛋.md",
      preference_weight: 5,
    },
  });
  expect(addResponse.ok()).toBe(true);

  await page.goto("/");
  await page.getByRole("button", { name: "开始这一顿" }).click();
  await expect(page.getByText("Discovery · 候选池外探索", { exact: true })).toBeVisible();
  const discoveryName = await page.getByRole("heading", { level: 2 }).textContent();
  expect(discoveryName).toBeTruthy();

  await page.getByRole("button", { name: "就吃这个（Acceptance）" }).click();
  await expect(page).toHaveURL(/\/recipes\?dish_id=/);
  await expect(page.getByRole("heading", { level: 2 })).toHaveText(discoveryName!);

  await page.getByRole("link", { name: "开始下一顿" }).click();
  await expect(page.getByRole("heading", { name: "先评完上次的 Discovery" })).toBeVisible();
  await expect(page.getByRole("heading", { name: discoveryName! })).toBeVisible();
  await expect(page.getByText(/Meal 时间：/)).toBeVisible();
  for (const label of ["拉完了", "NPC", "人上人", "顶级", "夯"]) {
    await expect(page.getByRole("button", { name: label, exact: true })).toBeVisible();
  }

  await page.getByRole("button", { name: "夯", exact: true }).click();
  await expect(page.getByText("Meal 已就绪", { exact: true })).toBeVisible();
  await page.getByRole("link", { name: "管理 Candidate pool" }).click();
  await expect(page.getByText(discoveryName!, { exact: true })).toBeVisible();
  await expect(page.getByText("Preference weight：1.3", { exact: true })).toBeVisible();
});
