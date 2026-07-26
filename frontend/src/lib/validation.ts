import type { FormRule } from "antd";

// 与 Go 端 credentialErrors 完全一致的客户端预校验（服务端仍是权威）。
export const USERNAME_MIN_RUNES = 3;
export const USERNAME_MAX_RUNES = 32;
export const USERNAME_PATTERN = /^[\p{L}\p{N}_-]+$/u;
export const PASSWORD_MIN_RUNES = 8;
export const PASSWORD_MAX_BYTES = 72;
export const MESSAGE_MAX_RUNES = 600;

const encoder = new TextEncoder();

export function runeCount(value: string): number {
  return [...value].length;
}

export function utf8ByteLength(value: string): number {
  return encoder.encode(value).length;
}

export function validateUsername(value: string): string | undefined {
  const runes = runeCount(value);
  if (runes < USERNAME_MIN_RUNES || runes > USERNAME_MAX_RUNES) {
    return `用户名须为 ${USERNAME_MIN_RUNES}–${USERNAME_MAX_RUNES} 个字符`;
  }
  if (!USERNAME_PATTERN.test(value)) {
    return "用户名只能包含字母、数字、下划线或连字符";
  }
  return undefined;
}

export function validatePassword(value: string): string | undefined {
  if (runeCount(value) < PASSWORD_MIN_RUNES) {
    return `密码至少需要 ${PASSWORD_MIN_RUNES} 个字符`;
  }
  if (utf8ByteLength(value) > PASSWORD_MAX_BYTES) {
    return `密码不能超过 ${PASSWORD_MAX_BYTES} 个 UTF-8 字节`;
  }
  return undefined;
}

function toRule(validate: (value: string) => string | undefined): FormRule {
  return {
    validator: (_rule, value: unknown) => {
      const message = validate(typeof value === "string" ? value : "");
      return message ? Promise.reject(new Error(message)) : Promise.resolve();
    },
  };
}

export const usernameRules: FormRule[] = [toRule(validateUsername)];
export const passwordRules: FormRule[] = [toRule(validatePassword)];
