import { expect, test, type Locator } from "@playwright/test";

import { registerAccount } from "./browser-api";

// 现设计的大按钮（shadcn size="lg"）高 40px：按设计现状断言，
// 防回归缩水；是否抬到 44px 触达基线属视觉决策，归用户定。
async function expectUsableTapTarget(locator: Locator) {
  const box = await locator.boundingBox();
  expect(box).not.toBeNull();
  expect(box!.width).toBeGreaterThanOrEqual(40);
  expect(box!.height).toBeGreaterThanOrEqual(40);
}

// 测试 Catalog 里只有起步包的四道：起步包按钮的部分成功语义（缺货照跳、
// 有货入池）在此顺带被验证。凑满四道是为了让「池小」探索信号归零，
// 旅程的每次揭示都确定走池内。
const STARTER_DISHES = ["鱼香肉丝", "蒜蓉西兰花", "宫保鸡丁", "红烧茄子"];
const STARTER_PATTERN = new RegExp(`^(${STARTER_DISHES.join("|")})$`);

test("起步包开局到第一顿收尾的完整旅程", async ({ page }, testInfo) => {
  const desktop = testInfo.project.name === "desktop";
  await registerAccount(page, `journey_${testInfo.project.name}`);

  // 空池欢迎：两条路，不多不少
  await expect(
    page.getByText("放几道你爱吃的进来，这一顿才有得挑。"),
  ).toBeVisible();
  const starter = page.getByRole("button", { name: /^经典起步包 · \d+ 道家常菜入池$/ });
  await expectUsableTapTarget(starter);
  await expect(page.getByRole("link", { name: "自己去挑菜" })).toBeVisible();

  // 起步包入池 → 就绪舞台
  await starter.click();
  await expect(
    page.getByRole("heading", { name: "这一顿，交给池子。" }),
  ).toBeVisible();
  await expect(page.getByText("合适的那道会自己站出来。")).toBeVisible();
  if (desktop) {
    await expect(
      page.getByRole("complementary", { name: "池子概览" }),
    ).toBeVisible();
    await expect(page.getByText(`${STARTER_DISHES.length} 道`)).toBeVisible();
    await expect(
      page.getByRole("complementary", { name: "最近吃过" }),
    ).toContainText("这里会记下你吃过的每一顿。");
  }

  // 就地揭示：这一顿 + 菜名 + 双出口
  const begin = page.getByRole("button", { name: "开始这一顿" });
  await expectUsableTapTarget(begin);
  await begin.click();
  // 揭示菜名住在 aria-live 舞台里（桌面端两条信息栏也有 h2，不能裸抓）
  const dishName = page.locator('[aria-live="polite"] h2');
  await expect(dishName).toHaveText(STARTER_PATTERN);
  await expect(page.getByText("这一顿", { exact: true })).toBeVisible();
  const reroll = page.getByRole("button", { name: /^换一道 · 剩 3 次$/ });
  const accept = page.getByRole("button", { name: "就吃这个" });
  await expectUsableTapTarget(reroll);
  await expectUsableTapTarget(accept);

  // Reroll：换字 + 余量递减
  const firstDish = await dishName.textContent();
  await reroll.click();
  await expect.poll(() => dishName.textContent()).not.toBe(firstDish);
  await expect(dishName).toHaveText(STARTER_PATTERN);
  await expect(
    page.getByRole("button", { name: /^换一道 · 剩 2 次$/ }),
  ).toBeVisible();

  // 接受 → 菜谱页
  const acceptedDish = await dishName.textContent();
  await accept.click();
  await expect(page).toHaveURL(/\/recipes\?dish_id=/);
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(
    acceptedDish!,
  );
  const next = page.getByRole("link", { name: "开始下一顿" });
  await expectUsableTapTarget(next);

  // 池内接受不设拦截：回来就能开下一顿；最近吃过被记下
  await next.click();
  await expect(
    page.getByRole("heading", { name: "这一顿，交给池子。" }),
  ).toBeVisible();
  if (desktop) {
    await expect(
      page.getByRole("complementary", { name: "最近吃过" }),
    ).toContainText(acceptedDish!);
  }

  // 全程无横向滚动
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
});
