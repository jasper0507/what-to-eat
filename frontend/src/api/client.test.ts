import { afterEach, describe, expect, it, vi } from "vitest";

import { ApiError, apiFetch } from "./client";

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), { status });
}

function stubFetch(result: Response | Error): ReturnType<typeof vi.fn> {
  const fetchMock =
    result instanceof Error
      ? vi.fn().mockRejectedValue(result)
      : vi.fn().mockResolvedValue(result);
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

async function caughtError(promise: Promise<unknown>): Promise<unknown> {
  try {
    await promise;
    throw new Error("expected apiFetch to reject");
  } catch (cause) {
    return cause;
  }
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("apiFetch", () => {
  it("解析成功的 JSON 响应", async () => {
    stubFetch(jsonResponse(200, { account: { id: 1, username: "eater" } }));
    await expect(apiFetch("GET", "/api/auth/session")).resolves.toEqual({
      account: { id: 1, username: "eater" },
    });
  });

  it("空体 2xx（201/204）解析为 undefined", async () => {
    stubFetch(new Response(null, { status: 204 }));
    await expect(
      apiFetch<void>("DELETE", "/api/candidate-pool/dishes?dish_id=x"),
    ).resolves.toBe(undefined);
  });

  it("错误信封转成带 code 与服务端原文的 ApiError", async () => {
    stubFetch(
      jsonResponse(401, {
        error: { code: "unauthorized", message: "需要登录" },
      }),
    );
    const error = await caughtError(apiFetch("GET", "/api/meals/resume"));
    expect(error).toBeInstanceOf(ApiError);
    const apiError = error as ApiError;
    expect(apiError.status).toBe(401);
    expect(apiError.code).toBe("unauthorized");
    expect(apiError.message).toBe("需要登录");
  });

  it("非 JSON 错误体归一为 unexpected_response", async () => {
    stubFetch(new Response("<html>panic</html>", { status: 500 }));
    const error = (await caughtError(
      apiFetch("GET", "/api/meals/resume"),
    )) as ApiError;
    expect(error.code).toBe("unexpected_response");
    expect(error.status).toBe(500);
  });

  it("2xx 但非 JSON（SPA fallback HTML）归一为 unexpected_response", async () => {
    stubFetch(new Response("<!doctype html><html></html>", { status: 200 }));
    const error = (await caughtError(apiFetch("GET", "/api/typo"))) as ApiError;
    expect(error.code).toBe("unexpected_response");
  });

  it("网络失败归一为 network_error（status 0）", async () => {
    stubFetch(new TypeError("fetch failed"));
    const error = (await caughtError(
      apiFetch("GET", "/api/meals/resume"),
    )) as ApiError;
    expect(error.code).toBe("network_error");
    expect(error.status).toBe(0);
  });

  it("中止的请求原样抛出 AbortError（不包装成 ApiError）", async () => {
    stubFetch(new DOMException("The operation was aborted.", "AbortError"));
    const error = await caughtError(apiFetch("GET", "/api/meals/resume"));
    expect(error).toBeInstanceOf(DOMException);
    expect((error as DOMException).name).toBe("AbortError");
  });

  it("带 body 的请求序列化 JSON 并设置 Content-Type；无 body 不设", async () => {
    const fetchMock = stubFetch(jsonResponse(200, {}));
    await apiFetch("POST", "/api/auth/login", {
      body: { username: "u", password: "p" },
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/auth/login",
      expect.objectContaining({
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username: "u", password: "p" }),
      }),
    );

    fetchMock.mockResolvedValue(jsonResponse(200, {}));
    await apiFetch("GET", "/api/meals/resume");
    const [, init] = fetchMock.mock.calls[1] as [string, RequestInit];
    expect(init.headers).toBe(undefined);
    expect(init.body).toBe(undefined);
  });
});
