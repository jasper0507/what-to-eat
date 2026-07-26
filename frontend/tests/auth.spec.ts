import { expect, test } from "@playwright/test";

import { registerAccount } from "./browser-api";

test("注册开户：会话 Cookie 齐全，落在空池欢迎", async ({
  page,
}, testInfo) => {
  await page.goto("/login");
  await expect(
    page.getByRole("heading", { name: "今天吃什么？" }),
  ).toBeVisible();
  await expect(page.getByText("进来，定这一顿。")).toBeVisible();
  await expect(page.locator('link[rel="manifest"]')).toHaveAttribute(
    "href",
    /manifest/,
  );

  await page.getByRole("button", { name: "注册" }).click();
  await expect(page.getByText("已有账号？")).toBeVisible();
  await expect(page.getByText("至少 8 个字符。")).toBeVisible();

  await page
    .getByRole("textbox", { name: "用户名" })
    .fill(`auth_${testInfo.project.name}`);
  await page.getByRole("textbox", { name: "密码" }).fill("browser-pass-1");
  const registration = page.waitForResponse((response) =>
    response.url().endsWith("/api/auth/register"),
  );
  await page.getByRole("button", { name: "注册" }).click();
  const sessionCookie = await (await registration).headerValue("set-cookie");
  expect(sessionCookie).toContain("HttpOnly");
  expect(sessionCookie).toContain("Secure");
  expect(sessionCookie).toContain("SameSite=Lax");

  await expect(
    page.getByRole("heading", { name: "池子还空着。" }),
  ).toBeVisible();
});

test("客户端预校验先把话说清楚", async ({ page }) => {
  await page.goto("/login");
  await page.getByRole("button", { name: "登录" }).click();
  await expect(page.getByText("输入用户名")).toBeVisible();

  await page.getByRole("textbox", { name: "用户名" }).fill("某个人");
  await page.getByRole("button", { name: "登录" }).click();
  await expect(page.getByText("输入密码")).toBeVisible();

  await page.getByRole("button", { name: "注册" }).click();
  await page.getByRole("textbox", { name: "用户名" }).fill("某个人");
  await page.getByRole("textbox", { name: "密码" }).fill("短密码");
  await page.getByRole("button", { name: "注册" }).click();
  await expect(page.getByText("密码至少需要 8 个字符")).toBeVisible();
});

test("密码可见性切换", async ({ page }) => {
  await page.goto("/login");
  const password = page.getByRole("textbox", { name: "密码" });
  await password.fill("browser-pass-1");
  await expect(password).toHaveAttribute("type", "password");
  await page.getByRole("button", { name: "显示密码" }).click();
  await expect(password).toHaveAttribute("type", "text");
  await page.getByRole("button", { name: "隐藏密码" }).click();
  await expect(password).toHaveAttribute("type", "password");
});

test("登出回到登录页，同一账号能再进来", async ({ page }, testInfo) => {
  const username = `logout_${testInfo.project.name}`;
  await registerAccount(page, username);

  await page.getByRole("button", { name: "登出" }).click();
  await expect(page).toHaveURL(/\/login$/);

  await page.getByRole("textbox", { name: "用户名" }).fill(username);
  await page.getByRole("textbox", { name: "密码" }).fill("browser-pass-1");
  await page.getByRole("button", { name: "登录" }).click();
  await expect(
    page.getByRole("heading", { name: "池子还空着。" }),
  ).toBeVisible();
});

test("登录后的乱路径给一个回主页的出口", async ({ page }, testInfo) => {
  await registerAccount(page, `lost_${testInfo.project.name}`);
  await page.goto("/不存在的角落");
  await expect(
    page.getByRole("heading", { name: "这里什么都没有。" }),
  ).toBeVisible();
  await expect(page.getByText("地址不对。回主页，接着定这一顿。")).toBeVisible();
  await page.getByRole("link", { name: "回主页" }).click();
  await expect(
    page.getByRole("heading", { name: "池子还空着。" }),
  ).toBeVisible();
});
