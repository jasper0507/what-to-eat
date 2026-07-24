import { useEffect, useState } from "react";
import {
  Alert,
  Button,
  Card,
  Form,
  Input,
  Segmented,
  Space,
  Spin,
  Typography,
} from "antd";
import { Navigate, Route, Routes } from "react-router-dom";

type Account = {
  id: number;
  username: string;
};

type AccountResponse = {
  account: Account;
};

type ErrorResponse = {
  error?: {
    message?: string;
  };
};

async function requestAccount(path: string, init?: RequestInit): Promise<Account> {
  const response = await fetch(path, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...init?.headers,
    },
  });
  const body = (await response.json()) as AccountResponse | ErrorResponse;
  if (!response.ok) {
    const error = body as ErrorResponse;
    throw new Error(error.error?.message ?? "服务暂时不可用，请稍后重试");
  }
  return (body as AccountResponse).account;
}

function AuthPage({ onAuthenticated }: { onAuthenticated: (account: Account) => void }) {
  const [mode, setMode] = useState<"login" | "register">("login");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string>();

  async function submit(values: { username: string; password: string }) {
    setSubmitting(true);
    setError(undefined);
    try {
      const account = await requestAccount(`/api/auth/${mode}`, {
        method: "POST",
        body: JSON.stringify(values),
      });
      onAuthenticated(account);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "服务暂时不可用，请稍后重试");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="auth-shell">
      <Card className="auth-card" bordered={false}>
        <Space direction="vertical" size={24} className="full-width">
          <header>
            <Typography.Title level={1}>今天吃什么？</Typography.Title>
            <Typography.Text type="secondary">先进入你的 Account，让选择和记录只属于你。</Typography.Text>
          </header>

          <Segmented
            block
            size="large"
            value={mode}
            options={[
              { label: "登录", value: "login" },
              { label: "注册", value: "register" },
            ]}
            onChange={(value) => {
              if (value === "login" || value === "register") {
                setMode(value);
              }
              setError(undefined);
            }}
          />

          {error && <Alert type="error" showIcon message={error} />}

          <Form layout="vertical" requiredMark={false} onFinish={submit}>
            <Form.Item
              label="用户名"
              name="username"
              rules={[
                { required: true, message: "请输入用户名" },
                { min: 3, max: 32, message: "用户名须为 3–32 个字符" },
                {
                  pattern: /^[\p{L}\p{N}_-]+$/u,
                  message: "用户名只能包含字母、数字、下划线或连字符",
                },
              ]}
            >
              <Input
                size="large"
                autoComplete="username"
                maxLength={32}
                placeholder="你的用户名"
              />
            </Form.Item>
            <Form.Item
              label="密码"
              name="password"
              rules={[
                { required: true, message: "请输入密码" },
                {
                  validator: async (_, value?: string) => {
                    if (
                      value &&
                      ([...value].length < 8 || new TextEncoder().encode(value).length > 72)
                    ) {
                      throw new Error("密码须至少 8 个字符且不超过 72 字节");
                    }
                  },
                },
              ]}
            >
              <Input.Password
                size="large"
                autoComplete={mode === "login" ? "current-password" : "new-password"}
                placeholder="至少 8 个字符，最多 72 字节"
              />
            </Form.Item>
            <Button
              block
              type="primary"
              size="large"
              htmlType="submit"
              loading={submitting}
            >
              {mode === "login" ? "登录" : "创建 Account"}
            </Button>
          </Form>
        </Space>
      </Card>
    </main>
  );
}

function AccountHome({ account }: { account: Account }) {
  return (
    <main className="auth-shell">
      <Card className="auth-card" bordered={false}>
        <Space direction="vertical" size={16}>
          <Typography.Text type="secondary">Account 会话已恢复</Typography.Text>
          <Typography.Title level={1}>你好，{account.username}</Typography.Title>
          <Typography.Paragraph>
            你的 Account 已安全连接。下一张工单会从这里继续建立候选池和每日 Decision。
          </Typography.Paragraph>
        </Space>
      </Card>
    </main>
  );
}

export function WhatToEatApp() {
  const [account, setAccount] = useState<Account | null>();
  const [loadError, setLoadError] = useState<string>();

  useEffect(() => {
    fetch("/api/auth/session")
      .then(async (response) => {
        if (response.status === 401) {
          setAccount(null);
          return;
        }
        if (!response.ok) {
          throw new Error("无法恢复 Account 会话");
        }
        const body = (await response.json()) as AccountResponse;
        setAccount(body.account);
      })
      .catch((cause) => {
        setLoadError(cause instanceof Error ? cause.message : "无法恢复 Account 会话");
      });
  }, []);

  if (loadError) {
    return (
      <main className="centered-state">
        <Alert
          type="error"
          showIcon
          message={loadError}
          action={<Button onClick={() => window.location.reload()}>重试</Button>}
        />
      </main>
    );
  }

  if (account === undefined) {
    return (
      <main className="centered-state" aria-label="正在恢复 Account 会话">
        <Spin size="large" />
      </main>
    );
  }

  return (
    <Routes>
      <Route
        path="/login"
        element={
          account ? <Navigate to="/" replace /> : <AuthPage onAuthenticated={setAccount} />
        }
      />
      <Route
        path="*"
        element={account ? <AccountHome account={account} /> : <Navigate to="/login" replace />}
      />
    </Routes>
  );
}
