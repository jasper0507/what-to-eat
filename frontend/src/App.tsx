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
  Slider,
  Space,
  Spin,
  Tag,
  Typography,
} from "antd";
import { Link, Navigate, Route, Routes } from "react-router-dom";

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

type CandidateDish = Dish & {
  preference_weight: number;
};

type MealResume = {
  status: "candidate_pool_empty" | "ready";
};

async function requestJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...init?.headers,
    },
  });
  const text = await response.text();
  const body = text ? (JSON.parse(text) as T | ErrorResponse) : undefined;
  if (!response.ok) {
    const error = body as ErrorResponse | undefined;
    throw new Error(error?.error?.message ?? "服务暂时不可用，请稍后重试");
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

function HomePage({ account }: { account: Account }) {
  const [resume, setResume] = useState<MealResume>();
  const [error, setError] = useState<string>();

  useEffect(() => {
    requestJSON<MealResume>("/api/meals/resume")
      .then(setResume)
      .catch((cause) => {
        setError(cause instanceof Error ? cause.message : "无法恢复 Meal 状态");
      });
  }, []);

  return (
    <main className="page-shell">
      <Card className="page-card" bordered={false}>
        <Space direction="vertical" size={24} className="full-width">
          <header className="page-header">
            <Typography.Text type="secondary">你好，{account.username}</Typography.Text>
            <Typography.Title level={1}>准备好决定这一顿了吗？</Typography.Title>
            <Typography.Paragraph type="secondary">
              Meal readiness 来自你的真实 Candidate pool，不会从 Catalog 隐藏兜底。
            </Typography.Paragraph>
          </header>

          {error && <Alert type="error" showIcon message={error} />}
          {!resume && !error && (
            <div className="inline-loading" aria-label="正在恢复 Meal 状态">
              <Spin />
            </div>
          )}

          {resume?.status === "candidate_pool_empty" && (
            <>
              <Alert
                type="warning"
                showIcon
                message="Candidate pool 为空"
                description="当前无法创建 Decision。先从 Catalog 添加至少一个你愿意考虑的 Dish。"
              />
              <Link to="/candidate-pool">
                <Button block type="primary" size="large">
                  搜索 Catalog 添加 Dish
                </Button>
              </Link>
            </>
          )}

          {resume?.status === "ready" && (
            <>
              <Alert
                type="success"
                showIcon
                message="Meal 已就绪"
                description="Candidate pool 中已有可用于下一次 Decision 的 Dish。"
              />
              <Link to="/candidate-pool">
                <Button block type="primary" size="large">
                  管理 Candidate pool
                </Button>
              </Link>
            </>
          )}
        </Space>
      </Card>
    </main>
  );
}

function CandidatePoolPage({ account }: { account: Account }) {
  const [pool, setPool] = useState<CandidateDish[]>();
  const [catalogDishes, setCatalogDishes] = useState<Dish[] | null>(null);
  const [searching, setSearching] = useState(false);
  const [busyDishID, setBusyDishID] = useState<string>();
  const [poolError, setPoolError] = useState<string>();
  const [searchError, setSearchError] = useState<string>();

  async function loadPool() {
    try {
      const result = await requestJSON<{ dishes: CandidateDish[] }>(
        "/api/candidate-pool/dishes",
      );
      setPool(result.dishes);
      setPoolError(undefined);
    } catch (cause) {
      setPoolError(cause instanceof Error ? cause.message : "无法读取 Candidate pool");
    }
  }

  useEffect(() => {
    void loadPool();
  }, []);

  async function search(value: string) {
    const query = value.trim();
    setSearchError(undefined);
    if (!query) {
      setCatalogDishes(null);
      setSearchError("请输入要搜索的 Dish 名称");
      return;
    }

    setSearching(true);
    try {
      const result = await requestJSON<{ dishes: Dish[] }>(
        `/api/catalog/dishes?q=${encodeURIComponent(query)}`,
      );
      setCatalogDishes(result.dishes);
    } catch (cause) {
      setCatalogDishes(null);
      setSearchError(
        cause instanceof Error ? cause.message : "Catalog 搜索失败，请稍后重试",
      );
    } finally {
      setSearching(false);
    }
  }

  async function addDish(dish: Dish) {
    setBusyDishID(dish.id);
    setPoolError(undefined);
    try {
      await requestJSON<void>("/api/candidate-pool/dishes", {
        method: "POST",
        body: JSON.stringify({ dish_id: dish.id, preference_weight: 1 }),
      });
      setPool((current) =>
        [...(current ?? []), { ...dish, preference_weight: 1 }].sort((left, right) =>
          left.name.localeCompare(right.name, "zh-CN"),
        ),
      );
    } catch (cause) {
      setPoolError(cause instanceof Error ? cause.message : "无法加入 Candidate pool");
    } finally {
      setBusyDishID(undefined);
    }
  }

  async function saveWeight(dishID: string, preferenceWeight: number) {
    setBusyDishID(dishID);
    setPoolError(undefined);
    try {
      await requestJSON<void>("/api/candidate-pool/dishes", {
        method: "PATCH",
        body: JSON.stringify({
          dish_id: dishID,
          preference_weight: preferenceWeight,
        }),
      });
    } catch (cause) {
      const message =
        cause instanceof Error ? cause.message : "无法保存 Preference weight";
      await loadPool();
      setPoolError(message);
    } finally {
      setBusyDishID(undefined);
    }
  }

  async function removeDish(dishID: string) {
    setBusyDishID(dishID);
    setPoolError(undefined);
    try {
      await requestJSON<void>(
        `/api/candidate-pool/dishes?dish_id=${encodeURIComponent(dishID)}`,
        { method: "DELETE" },
      );
      setPool((current) => current?.filter((dish) => dish.id !== dishID));
    } catch (cause) {
      setPoolError(cause instanceof Error ? cause.message : "无法移出 Candidate pool");
    } finally {
      setBusyDishID(undefined);
    }
  }

  return (
    <main className="page-shell">
      <Card className="page-card pool-card" bordered={false}>
        <Space direction="vertical" size={24} className="full-width">
          <header className="page-header">
            <div className="page-header-row">
              <Typography.Text type="secondary">你好，{account.username}</Typography.Text>
              <Link to="/">返回 Meal readiness</Link>
            </div>
            <Typography.Title level={1}>Candidate pool</Typography.Title>
            <Typography.Paragraph type="secondary">
              这里的 Dish 才会参与普通 Decision；Preference weight 越高，基础偏好越强。
            </Typography.Paragraph>
          </header>

          {poolError && <Alert type="error" showIcon message={poolError} />}

          {pool === undefined && !poolError && (
            <div className="inline-loading" aria-label="正在读取 Candidate pool">
              <Spin />
            </div>
          )}

          {pool?.length === 0 && (
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="Candidate pool 还是空的" />
          )}

          {pool && pool.length > 0 && (
            <List
              className="pool-list"
              aria-label="Candidate pool"
              dataSource={pool}
              renderItem={(dish) => (
                <List.Item key={dish.id}>
                  <div className="pool-item">
                    <div>
                      <Space size={8} wrap>
                        <Typography.Text strong>{dish.name}</Typography.Text>
                        <Tag color="orange">{dish.category}</Tag>
                      </Space>
                      <Typography.Paragraph type="secondary" className="recipe-path">
                        {dish.recipe_path}
                      </Typography.Paragraph>
                    </div>
                    <div className="weight-control">
                      <Typography.Text>
                        Preference weight：{dish.preference_weight.toFixed(1)}
                      </Typography.Text>
                      <Slider
                        aria-label={`${dish.name} Preference weight`}
                        min={0.1}
                        max={5}
                        step={0.1}
                        value={dish.preference_weight}
                        disabled={busyDishID === dish.id}
                        onChange={(value) => {
                          setPool((current) =>
                            current?.map((member) =>
                              member.id === dish.id
                                ? { ...member, preference_weight: value }
                                : member,
                            ),
                          );
                        }}
                        onChangeComplete={(value) => {
                          void saveWeight(dish.id, value);
                        }}
                      />
                    </div>
                    <Button
                      danger
                      loading={busyDishID === dish.id}
                      onClick={() => {
                        void removeDish(dish.id);
                      }}
                    >
                      移出
                    </Button>
                  </div>
                </List.Item>
              )}
            />
          )}

          <section aria-labelledby="catalog-search-heading">
            <Typography.Title id="catalog-search-heading" level={2}>
              从 Catalog 添加
            </Typography.Title>
            <Typography.Paragraph type="secondary">
              按名称查找 HowToCook Dish，不会把自由文本创建成新 Dish。
            </Typography.Paragraph>
          </section>

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

          {searchError && <Alert type="error" showIcon message={searchError} />}

          {catalogDishes === null && !searchError && (
            <Typography.Text type="secondary">输入名称后，Catalog 只会返回已有 Dish。</Typography.Text>
          )}

          {catalogDishes?.length === 0 && (
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="Catalog 中没有匹配的 Dish" />
          )}

          {catalogDishes && catalogDishes.length > 0 && (
            <List
              aria-label="Catalog 搜索结果"
              dataSource={catalogDishes}
              renderItem={(dish) => (
                <List.Item key={dish.id}>
                  <div className="catalog-result">
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
                    <Button
                      type="primary"
                      disabled={pool?.some((member) => member.id === dish.id)}
                      loading={busyDishID === dish.id}
                      onClick={() => {
                        void addDish(dish);
                      }}
                    >
                      {pool?.some((member) => member.id === dish.id) ? "已加入" : "加入"}
                    </Button>
                  </div>
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
        path="/candidate-pool"
        element={
          account ? <CandidatePoolPage account={account} /> : <Navigate to="/login" replace />
        }
      />
      <Route
        path="*"
        element={account ? <HomePage account={account} /> : <Navigate to="/login" replace />}
      />
    </Routes>
  );
}
