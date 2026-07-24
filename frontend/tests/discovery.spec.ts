import { expect, test } from "@playwright/test";

test("Discovery stays explicit and respects similarity, Reroll penalty, and Meal limit", async ({
  page,
}, testInfo) => {
  await page.goto("/");
  await page.getByText("注册", { exact: true }).click();
  await page.getByLabel("用户名").fill(`discovery_${testInfo.project.name}`);
  await page.getByLabel("密码").fill("browser-pass-1");
  await page.getByRole("button", { name: "创建 Account" }).click();
  await expect(page.getByRole("heading", { name: "先聊聊你爱吃什么" })).toBeVisible();

  const poolDish = "vegetable_dish/番茄炒蛋.md";
  const addResponse = await page.request.post("/api/candidate-pool/dishes", {
    data: { dish_id: poolDish, preference_weight: 5 },
  });
  expect(addResponse.ok()).toBe(true);

  await page.goto("/");
  await page.getByRole("button", { name: "开始这一顿" }).click();

  const discoveryLabel = page.getByText("Discovery · 候选池外探索", {
    exact: true,
  });
  const reason = page.getByText(/Candidate pool 较小.*Cooldown 后可选 Dish 较少/);
  const heading = page.getByRole("heading", { level: 2 });
  const similarDishes = ["番茄豆腐", "番茄土豆"];

  await expect(discoveryLabel).toBeVisible();
  await expect(reason).toBeVisible();
  await expect(heading).toHaveText(new RegExp(`^(${similarDishes.join("|")})$`));
  const firstDiscovery = await heading.textContent();

  await page.getByRole("button", { name: "Reroll" }).click();
  await expect(discoveryLabel).toBeVisible();
  await expect.poll(() => heading.textContent()).not.toBe(firstDiscovery);
  await expect(heading).toHaveText(new RegExp(`^(${similarDishes.join("|")})$`));

  await page.getByRole("button", { name: "Reroll" }).click();
  await expect(page.getByText("普通 Pool pick", { exact: true })).toBeVisible();
  await expect(discoveryLabel).toHaveCount(0);
  await expect(heading).toHaveText("番茄炒蛋");
});
