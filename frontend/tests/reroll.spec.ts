import { expect, test } from "@playwright/test";

import { addCandidatePoolDish } from "./browser-api";

test("Eater can Reroll and accept the replacement Decision", async ({
  page,
}, testInfo) => {
  await page.goto("/");
  await page.getByText("注册", { exact: true }).click();
  await page.getByLabel("用户名").fill(`reroll_${testInfo.project.name}`);
  await page.getByLabel("密码").fill("browser-pass-1");
  await page.getByRole("button", { name: "创建 Account" }).click();
  await expect(page.getByRole("heading", { name: "先聊聊你爱吃什么" })).toBeVisible();

  for (const dish of [
    "vegetable_dish/番茄炒蛋.md",
    "meat_dish/番茄牛腩.md",
  ]) {
    expect(await addCandidatePoolDish(page, dish, 1)).toBe(true);
  }

  await page.goto("/");
  await page.getByRole("button", { name: "开始这一顿" }).click();

  const decisionHeading = page.getByRole("heading", { level: 2 });
  const firstDish = await decisionHeading.textContent();
  const reroll = page.getByRole("button", { name: "Reroll" });
  const accept = page.getByRole("button", { name: /就吃这个/ });
  const rerollBox = await reroll.boundingBox();
  const acceptBox = await accept.boundingBox();
  expect(rerollBox).not.toBeNull();
  expect(acceptBox).not.toBeNull();
  if (testInfo.project.name === "mobile") {
    expect(rerollBox!.y).toBeLessThan(acceptBox!.y);
  } else {
    expect(Math.abs(rerollBox!.y - acceptBox!.y)).toBeLessThan(2);
    expect(rerollBox!.x).toBeLessThan(acceptBox!.x);
  }

  await reroll.click();
  await expect.poll(() => decisionHeading.textContent()).not.toBe(firstDish);
  const replacementDish = await decisionHeading.textContent();

  await accept.click();
  await expect(page).toHaveURL(/\/recipes\?dish_id=/);
  await expect(page.getByRole("heading", { level: 2 })).toHaveText(replacementDish!);
});
