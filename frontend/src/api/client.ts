import { copy } from "@/lib/copy";

import type { ApiErrorCode } from "./types";

// 全应用唯一的 fetch 调用点。职责：把任意响应形态归一为「typed 值或 ApiError」。
// 401 的会话处理不在这里做（属于 queryClient 的全局漏斗），保持本模块纯净可单测。
export class ApiError extends Error {
  readonly status: number;
  readonly code: ApiErrorCode;

  constructor(status: number, code: ApiErrorCode, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }
}

interface ErrorEnvelope {
  error?: { code?: string; message?: string };
}

export async function apiFetch<T>(
  method: "GET" | "POST" | "PATCH" | "DELETE",
  path: string,
  options: { body?: unknown; signal?: AbortSignal } = {},
): Promise<T> {
  let response: Response;
  try {
    response = await fetch(path, {
      method,
      signal: options.signal,
      headers:
        options.body === undefined
          ? undefined
          : { "Content-Type": "application/json" },
      body:
        options.body === undefined ? undefined : JSON.stringify(options.body),
    });
  } catch (cause) {
    if (cause instanceof DOMException && cause.name === "AbortError") {
      throw cause;
    }
    throw new ApiError(0, "network_error", copy.errors.network);
  }

  const text = await response.text();

  if (response.ok) {
    // 空体成功（池子 add 的 201、PATCH/DELETE 的 204）
    if (text === "") {
      return undefined as unknown as T;
    }
    try {
      return JSON.parse(text) as T;
    } catch {
      // 2xx 但不是 JSON：多半是 SPA fallback 的 index.html
      throw new ApiError(
        response.status,
        "unexpected_response",
        copy.errors.unexpected,
      );
    }
  }

  let envelope: ErrorEnvelope | undefined;
  try {
    envelope = JSON.parse(text) as ErrorEnvelope;
  } catch {
    envelope = undefined;
  }
  const code = envelope?.error?.code;
  const message = envelope?.error?.message;
  if (!code || !message) {
    throw new ApiError(
      response.status,
      "unexpected_response",
      copy.errors.unexpected,
    );
  }
  throw new ApiError(response.status, code, message);
}
