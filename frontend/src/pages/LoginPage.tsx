import { Alert, Button, Card, Form, Input, Segmented } from "antd";
import { Eye, EyeOff } from "lucide-react";
import { m } from "motion/react";
import { useState } from "react";
import { Navigate } from "react-router-dom";

import { useLogin, useRegister, useSession } from "@/api/hooks";
import { peekSessionExpired } from "@/api/queryClient";
import type { Credentials } from "@/api/types";
import { ErrorAlert } from "@/components/ErrorAlert";
import { PageShell } from "@/components/PageShell";
import { copy } from "@/lib/copy";
import { pageEnter } from "@/lib/motion";
import { useRetryAfter } from "@/lib/useRetryAfter";
import { passwordRules, usernameRules } from "@/lib/validation";

type Mode = "login" | "register";

export default function LoginPage() {
  const session = useSession();
  const [mode, setMode] = useState<Mode>("login");
  const loginMutation = useLogin();
  const registerMutation = useRegister();
  const active = mode === "login" ? loginMutation : registerMutation;
  const retryRemaining = useRetryAfter(active.error);
  // 只读不消费（幂等）；下次认证成功时由 useAuthMutation 清除
  const expired = peekSessionExpired();

  if (session.isPending) {
    return <PageShell width="sm">{null}</PageShell>;
  }
  if (session.data) {
    // 注册成功直达 Onboarding；登录成功回主页（主页门控仍会按服务端状态兜底）
    return <Navigate to={mode === "register" ? "/onboarding" : "/"} replace />;
  }

  const switchMode = (next: Mode) => {
    setMode(next);
    loginMutation.reset();
    registerMutation.reset();
  };

  return (
    <PageShell width="sm">
      <m.div {...pageEnter}>
        <Card variant="borderless" className="login-card">
          <div className="page-stack page-stack-tight">
            <h1 className="page-title">{copy.auth.title}</h1>
            <p className="page-intro">{copy.auth.intro}</p>
            <Segmented
              block
              size="large"
              value={mode}
              onChange={(value) => switchMode(value as Mode)}
              options={[
                { label: copy.auth.loginTab, value: "login" },
                { label: copy.auth.registerTab, value: "register" },
              ]}
            />
            {expired && !active.error ? (
              <Alert
                type="warning"
                showIcon
                message={copy.auth.sessionExpired}
              />
            ) : null}
            <ErrorAlert error={active.error} retryRemaining={retryRemaining} />
            <Form<Credentials>
              layout="vertical"
              requiredMark={false}
              validateTrigger={["onBlur", "onSubmit"]}
              onFinish={(values) => active.mutate(values)}
            >
              <Form.Item
                label={copy.auth.username}
                name="username"
                rules={usernameRules}
              >
                <Input size="large" autoComplete="username" />
              </Form.Item>
              <Form.Item
                label={copy.auth.password}
                name="password"
                rules={passwordRules}
              >
                <Input.Password
                  size="large"
                  autoComplete={
                    mode === "login" ? "current-password" : "new-password"
                  }
                  iconRender={(visible) =>
                    visible ? <Eye size={16} /> : <EyeOff size={16} />
                  }
                />
              </Form.Item>
              <Button
                block
                type="primary"
                size="large"
                htmlType="submit"
                loading={active.isPending}
                disabled={retryRemaining > 0}
              >
                {mode === "login"
                  ? copy.auth.loginSubmit
                  : copy.auth.registerSubmit}
              </Button>
            </Form>
          </div>
        </Card>
      </m.div>
    </PageShell>
  );
}
