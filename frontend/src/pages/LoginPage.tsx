import { Eye, EyeOff, LoaderCircle } from "lucide-react";
import { useState, type FormEvent } from "react";
import { Navigate } from "react-router-dom";

import { useLogin, useRegister, useSession } from "@/api/hooks";
import { peekSessionExpired } from "@/api/queryClient";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { validatePassword, validateUsername } from "@/lib/validation";

type Mode = "login" | "register";

const text = {
  login: { submit: "登录", switchHint: "还没有账号？", switchAction: "注册" },
  register: { submit: "注册", switchHint: "已有账号？", switchAction: "登录" },
} as const;

interface FieldErrors {
  username?: string;
  password?: string;
}

export default function LoginPage() {
  const session = useSession();
  const [mode, setMode] = useState<Mode>("login");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({});
  const loginMutation = useLogin();
  const registerMutation = useRegister();
  const active = mode === "login" ? loginMutation : registerMutation;
  // 只读不消费（幂等）；下次认证成功时由 useAuthMutation 清除
  const expired = peekSessionExpired();

  if (session.isPending) {
    return (
      <div
        role="status"
        aria-label="正在加载"
        className="flex min-h-dvh items-center justify-center bg-background"
      >
        <LoaderCircle
          aria-hidden="true"
          className="size-5 animate-spin text-muted-foreground"
        />
      </div>
    );
  }
  if (session.data) {
    // 注册与登录一律回主页：空池态即欢迎引导（brief §三，无弹窗无强制访谈）
    return <Navigate to="/" replace />;
  }

  const t = text[mode];

  const switchMode = () => {
    setMode(mode === "login" ? "register" : "login");
    setFieldErrors({});
    loginMutation.reset();
    registerMutation.reset();
  };

  const handleSubmit = (event: FormEvent) => {
    event.preventDefault();
    // 空值给指路文案而非念规则；焦点移到第一个出错字段，读屏随 label+describedby 自然播报
    const errors: FieldErrors = {
      username: username === "" ? "输入用户名" : validateUsername(username),
      password: password === "" ? "输入密码" : validatePassword(password),
    };
    setFieldErrors(errors);
    if (errors.username || errors.password) {
      document
        .getElementById(errors.username ? "username" : "password")
        ?.focus();
      return;
    }
    active.mutate({ username, password });
  };

  return (
    <main className="flex min-h-dvh flex-col items-center justify-center bg-background px-6 pb-[10vh] font-sans text-base text-foreground antialiased">
      <div className="w-full max-w-[21.25rem]">
        <header className="animate-rise">
          <h1 className="font-serif text-4xl leading-snug font-medium tracking-wide">
            今天吃什么<span className="text-brand">？</span>
          </h1>
          <p className="mt-3 text-muted-foreground">进来，定这一顿。</p>
        </header>

        <form
          className="animate-rise mt-9 space-y-5 [animation-delay:60ms]"
          noValidate
          onSubmit={handleSubmit}
        >
          {expired && !active.error ? (
            <div className="rounded-md border border-border bg-secondary px-3 py-2 text-sm">
              登录过期了，再进来一次。
            </div>
          ) : null}
          {active.error ? (
            <div
              role="alert"
              className="rounded-md border border-destructive/25 bg-destructive/5 px-3 py-2 text-sm text-destructive"
            >
              {active.error.message}
            </div>
          ) : null}

          <div className="space-y-2">
            <Label htmlFor="username">用户名</Label>
            <Input
              id="username"
              name="username"
              autoComplete="username"
              autoCapitalize="none"
              autoCorrect="off"
              spellCheck={false}
              value={username}
              onChange={(event) => {
                setUsername(event.target.value);
                setFieldErrors((prev) => ({ ...prev, username: undefined }));
              }}
              aria-invalid={fieldErrors.username ? true : undefined}
              aria-describedby={
                fieldErrors.username ? "username-error" : undefined
              }
            />
            {fieldErrors.username ? (
              <p id="username-error" className="text-sm text-destructive">
                {fieldErrors.username}
              </p>
            ) : null}
          </div>

          <div className="space-y-2">
            <Label htmlFor="password">密码</Label>
            <div className="relative">
              <Input
                id="password"
                name="password"
                type={showPassword ? "text" : "password"}
                autoComplete={
                  mode === "login" ? "current-password" : "new-password"
                }
                className="pr-10"
                value={password}
                onChange={(event) => {
                  setPassword(event.target.value);
                  setFieldErrors((prev) => ({ ...prev, password: undefined }));
                }}
                aria-invalid={fieldErrors.password ? true : undefined}
                aria-describedby={
                  fieldErrors.password
                    ? "password-error"
                    : mode === "register"
                      ? "password-hint"
                      : undefined
                }
              />
              <button
                type="button"
                className="absolute inset-y-0 right-0 flex w-10 cursor-pointer items-center justify-center rounded-md text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
                aria-label={showPassword ? "隐藏密码" : "显示密码"}
                onClick={() => setShowPassword((visible) => !visible)}
              >
                {showPassword ? (
                  <Eye aria-hidden="true" className="size-4" />
                ) : (
                  <EyeOff aria-hidden="true" className="size-4" />
                )}
              </button>
            </div>
            {fieldErrors.password ? (
              <p id="password-error" className="text-sm text-destructive">
                {fieldErrors.password}
              </p>
            ) : mode === "register" ? (
              <p id="password-hint" className="text-sm text-muted-foreground">
                至少 8 个字符。
              </p>
            ) : null}
          </div>

          <Button
            type="submit"
            size="lg"
            className="mt-2 w-full"
            aria-busy={active.isPending}
            disabled={active.isPending}
          >
            {active.isPending ? (
              <LoaderCircle
                aria-hidden="true"
                className="size-4 animate-spin"
              />
            ) : null}
            {t.submit}
          </Button>
        </form>

        <p className="animate-rise mt-7 text-sm text-muted-foreground [animation-delay:120ms]">
          {t.switchHint}
          <button
            type="button"
            className="-mx-1 -my-2 cursor-pointer rounded-sm px-1 py-2 text-brand-ink underline decoration-brand-ink/40 underline-offset-4 outline-none hover:decoration-brand-ink focus-visible:ring-2 focus-visible:ring-ring"
            onClick={switchMode}
          >
            {t.switchAction}
          </button>
        </p>
      </div>
    </main>
  );
}
