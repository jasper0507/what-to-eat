import { useEffect, useState } from "react";
import {
  Alert,
  Button,
  Card,
  Empty,
  Form,
  Input,
  List,
  Segmented,
  Space,
  Spin,
  Tag,
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

type Dish = {
  id: string;
  name: string;
  category: string;
  recipe_path: string;
  tags: string[];
};

async function requestJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...init?.headers,
    },
  });
  const body = (await response.json()) as T | ErrorResponse;
  if (!response.ok) {
    const error = body as ErrorResponse;
    throw new Error(error.error?.message ?? "服务暂时不可用，请稍后重试");
  }
  return body as T;
}

function AuthPage({ onAuthenticated }: { onAuthenticated: (account: Account) => void }) {
  const [mode, setMode] = useState<"login" | "register">("login");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string>();

  async function submit(values: { username: string; password: string }) {
    setSubmitting(true);
    setError(undefined);
    try {
      const { account } = await requestJSON<AccountResponse>(`/api/auth/${mode}`, {
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

function CatalogPage({ account }: { account: Account }) {
  const [dishes, setDishes] = useState<Dish[] | null>(null);
  const [searching, setSearching] = useState(false);
  const [error, setError] = useState<string>();

  async function search(value: string) {
    const query = value.trim();
    setError(undefined);
    if (!query) {
      setDishes(null);
      setError("请输入要搜索的 Dish 名称");
      return;
    }

    setSearching(true);
    try {
      const result = await requestJSON<{ dishes: Dish[] }>(
        `/api/catalog/dishes?q=${encodeURIComponent(query)}`,
      );
      setDishes(result.dishes);
    } catch (cause) {
      setDishes(null);
      setError(cause instanceof Error ? cause.message : "Catalog 搜索失败，请稍后重试");
    } finally {
      setSearching(false);
    }
  }

  return (
    <main className="catalog-shell">
      <Card className="catalog-card" bordered={false}>
        <Space direction="vertical" size={24} className="full-width">
          <header className="catalog-header">
            <Typography.Text type="secondary">你好，{account.username}</Typography.Text>
            <Typography.Title level={1}>搜索想吃的 Dish</Typography.Title>
            <Typography.Paragraph type="secondary">
              从 HowToCook Catalog 中按名称查找，结果保留来源分类和稳定身份。
            </Typography.Paragraph>
          </header>

          <Input.Search
            aria-label="按名称搜索 Dish"
            allowClear
            enterButton="搜索"
            loading={searching}
            maxLength={100}
            placeholder="例如：番茄、炒蛋、牛腩"
            size="large"
            onSearch={search}
          />

          {error && <Alert type="error" showIcon message={error} />}

          {dishes === null && !error && (
            <Typography.Text type="secondary">输入名称后，Catalog 只会返回已有 Dish。</Typography.Text>
          )}

          {dishes?.length === 0 && (
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="Catalog 中没有匹配的 Dish" />
          )}

          {dishes && dishes.length > 0 && (
            <List
              aria-label="Catalog 搜索结果"
              dataSource={dishes}
              renderItem={(dish) => (
                <List.Item key={dish.id}>
                  <List.Item.Meta
                    title={
                      <Space size={8} wrap>
                        <Typography.Text strong>{dish.name}</Typography.Text>
                        <Tag color="orange">{dish.category}</Tag>
                        {dish.tags.map((tag) => (
                          <Tag key={tag}>{tag}</Tag>
                        ))}
                      </Space>
                    }
                    description={
                      <Typography.Text type="secondary" className="recipe-path">
                        {dish.recipe_path}
                      </Typography.Text>
                    }
                  />
                </List.Item>
              )}
            />
          )}
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
        element={account ? <CatalogPage account={account} /> : <Navigate to="/login" replace />}
      />
    </Routes>
  );
}
