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
import {
  Link,
  Navigate,
  Route,
  Routes,
  useNavigate,
  useSearchParams,
} from "react-router-dom";

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

type OnboardingMessage = {
  role: "user" | "assistant";
  content: string;
};

type OnboardingState = {
  status: "not_started" | "in_progress" | "failed" | "completed" | "manual";
  messages: OnboardingMessage[];
  can_retry: boolean;
};

type MealDecision = {
  id: number;
  meal_id: number;
  mode: "pool" | "discovery";
  reason?: string;
  dish: Dish;
};

type PendingRating = {
  id: number;
  meal_id: number;
  meal_at: number;
  dish: Dish;
};

type MealResume =
  | { status: "candidate_pool_empty" | "ready" }
  | { status: "active_decision"; decision: MealDecision }
  | { status: "pending_ratings"; pending_ratings: PendingRating[] };

type Recipe = {
  dish: Dish;
  content: string;
};

type AcceptanceResult = {
  recipe: {
    dish: Dish;
  };
};

const tasteRatings = [
  { rating: 1, label: "拉完了" },
  { rating: 2, label: "NPC" },
  { rating: 3, label: "人上人" },
  { rating: 4, label: "顶级" },
  { rating: 5, label: "夯" },
] as const;

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

function OnboardingEntry({ account }: { account: Account }) {
  const [state, setState] = useState<OnboardingState>();
  const [error, setError] = useState<string>();

  useEffect(() => {
    requestJSON<OnboardingState>("/api/onboarding/interview")
      .then(setState)
      .catch((cause) => {
        setError(cause instanceof Error ? cause.message : "无法恢复 Onboarding interview");
      });
  }, []);

  if (error) {
    return (
      <main className="centered-state">
        <Alert
          type="error"
          showIcon
          message={error}
          action={<Button onClick={() => window.location.reload()}>重试</Button>}
        />
      </main>
    );
  }
  if (!state) {
    return (
      <main className="centered-state" aria-label="正在恢复 Onboarding interview">
        <Spin size="large" />
      </main>
    );
  }
  if (
    state.status === "not_started" ||
    state.status === "in_progress" ||
    state.status === "failed"
  ) {
    return <Navigate to="/onboarding" replace />;
  }
  return <HomePage account={account} />;
}

function OnboardingPage({ account }: { account: Account }) {
  const [state, setState] = useState<OnboardingState>();
  const [message, setMessage] = useState("");
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState<"send" | "retry" | "manual">();
  const navigate = useNavigate();

  async function loadState(clearError = true) {
    try {
      setState(await requestJSON<OnboardingState>("/api/onboarding/interview"));
      if (clearError) {
        setError(undefined);
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "无法恢复 Onboarding interview");
    }
  }

  useEffect(() => {
    void loadState();
  }, []);

  async function sendMessage() {
    const content = message.trim();
    if (!content) {
      return;
    }
    setBusy("send");
    setError(undefined);
    try {
      setState(
        await requestJSON<OnboardingState>("/api/onboarding/interview/messages", {
          method: "POST",
          body: JSON.stringify({ message: content }),
        }),
      );
      setMessage("");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "无法发送访谈消息");
      await loadState(false);
    } finally {
      setBusy(undefined);
    }
  }

  async function retry() {
    setBusy("retry");
    setError(undefined);
    try {
      setState(
        await requestJSON<OnboardingState>("/api/onboarding/interview/retry", {
          method: "POST",
        }),
      );
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "无法重试访谈消息");
      await loadState(false);
    } finally {
      setBusy(undefined);
    }
  }

  async function useManualPool() {
    setBusy("manual");
    setError(undefined);
    try {
      await requestJSON<OnboardingState>("/api/onboarding/interview/manual", {
        method: "POST",
      });
      navigate("/candidate-pool", { replace: true });
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "无法切换到手工 Catalog 编辑");
      setBusy(undefined);
    }
  }

  if (state?.status === "completed" || state?.status === "manual") {
    return <Navigate to="/candidate-pool" replace />;
  }

  return (
    <main className="page-shell">
      <Card className="page-card onboarding-card" bordered={false}>
        <Space direction="vertical" size={24} className="full-width">
          <header className="page-header">
            <Typography.Text type="secondary">你好，{account.username}</Typography.Text>
            <Typography.Title level={1}>先聊聊你爱吃什么</Typography.Title>
            <Typography.Paragraph type="secondary">
              说具体 Dish 名称和喜欢程度；NIM 只在服务端参与这次访谈。
            </Typography.Paragraph>
          </header>

          {!state && !error && (
            <div className="inline-loading" aria-label="正在恢复 Onboarding interview">
              <Spin />
            </div>
          )}
          {error && <Alert type="error" showIcon message={error} />}

          {state && (
            <>
              <List
                className="interview-messages"
                aria-label="Onboarding interview 对话"
                dataSource={
                  state.messages.length > 0
                    ? state.messages
                    : [
                        {
                          role: "assistant" as const,
                          content: "先说一两道你平时真会想吃的具体菜名吧。",
                        },
                      ]
                }
                renderItem={(item, index) => (
                  <List.Item key={`${item.role}-${index}`}>
                    <article className={`interview-message ${item.role}`}>
                      <Typography.Text strong>
                        {item.role === "user" ? "你" : "访谈助手"}
                      </Typography.Text>
                      <Typography.Paragraph>{item.content}</Typography.Paragraph>
                    </article>
                  </List.Item>
                )}
              />

              <Input.TextArea
                aria-label="告诉访谈助手你喜欢的 Dish"
                autoSize={{ minRows: 3, maxRows: 6 }}
                disabled={busy !== undefined || state.can_retry}
                maxLength={600}
                placeholder="例如：我很喜欢番茄炒蛋，牛腩也常吃，但没那么偏爱"
                value={message}
                onChange={(event) => setMessage(event.target.value)}
              />

              {state.can_retry ? (
                <Button
                  block
                  type="primary"
                  size="large"
                  loading={busy === "retry"}
                  disabled={busy !== undefined && busy !== "retry"}
                  onClick={() => {
                    void retry();
                  }}
                >
                  重试上一条
                </Button>
              ) : (
                <Button
                  block
                  type="primary"
                  size="large"
                  autoInsertSpace={false}
                  loading={busy === "send"}
                  disabled={!message.trim() || busy !== undefined}
                  onClick={() => {
                    void sendMessage();
                  }}
                >
                  发送
                </Button>
              )}

              <Button
                block
                size="large"
                loading={busy === "manual"}
                disabled={busy !== undefined && busy !== "manual"}
                onClick={() => {
                  void useManualPool();
                }}
              >
                改用手工 Catalog 编辑
              </Button>
            </>
          )}
        </Space>
      </Card>
    </main>
  );
}

function HomePage({ account }: { account: Account }) {
  const [resume, setResume] = useState<MealResume>();
  const [error, setError] = useState<string>();
  const [submitting, setSubmitting] = useState(false);
  const [rerolling, setRerolling] = useState(false);
  const [submittingRating, setSubmittingRating] = useState<string>();
  const navigate = useNavigate();

  async function loadMeal() {
    try {
      setResume(await requestJSON<MealResume>("/api/meals/resume"));
      setError(undefined);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "无法恢复 Meal 状态");
    }
  }

  useEffect(() => {
    void loadMeal();
  }, []);

  async function beginMeal() {
    setSubmitting(true);
    setError(undefined);
    try {
      setResume(await requestJSON<MealResume>("/api/meals", { method: "POST" }));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "无法开始 Meal");
    } finally {
      setSubmitting(false);
    }
  }

  async function acceptDecision(decisionID: number) {
    setSubmitting(true);
    setError(undefined);
    try {
      const accepted = await requestJSON<AcceptanceResult>(
        `/api/decisions/${decisionID}/accept`,
        { method: "POST" },
      );
      navigate(
        `/recipes?dish_id=${encodeURIComponent(accepted.recipe.dish.recipe_path)}`,
      );
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "无法完成 Acceptance");
      setSubmitting(false);
    }
  }

  async function rerollDecision(decisionID: number) {
    setRerolling(true);
    setError(undefined);
    try {
      setResume(
        await requestJSON<MealResume>(`/api/decisions/${decisionID}/reroll`, {
          method: "POST",
        }),
      );
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "无法 Reroll Decision");
    } finally {
      setRerolling(false);
    }
  }

  async function ratePending(pendingRatingID: number, rating: number) {
    setSubmittingRating(`${pendingRatingID}:${rating}`);
    setError(undefined);
    try {
      await requestJSON(`/api/pending-ratings/${pendingRatingID}/rate`, {
        method: "POST",
        body: JSON.stringify({ rating }),
      });
      await loadMeal();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "无法提交 Taste rating");
    } finally {
      setSubmittingRating(undefined);
    }
  }

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

          {resume?.status === "pending_ratings" && (
            <section aria-labelledby="pending-ratings-heading">
              <Space direction="vertical" size={20} className="full-width">
                <div>
                  <Typography.Title id="pending-ratings-heading" level={2}>
                    先评完上次的 Discovery
                  </Typography.Title>
                  <Typography.Paragraph type="secondary">
                    每条只需点一下；全部解决后才能开始新的 Decision。
                  </Typography.Paragraph>
                </div>
                <List
                  className="pending-rating-list"
                  aria-label="Pending ratings"
                  dataSource={resume.pending_ratings}
                  renderItem={(pending) => (
                    <List.Item key={pending.id}>
                      <article className="pending-rating">
                        <Typography.Text type="secondary">
                          Meal 时间：
                          <time dateTime={new Date(pending.meal_at * 1000).toISOString()}>
                            {new Intl.DateTimeFormat("zh-CN", {
                              dateStyle: "medium",
                              timeStyle: "short",
                            }).format(new Date(pending.meal_at * 1000))}
                          </time>
                        </Typography.Text>
                        <Typography.Title level={3}>{pending.dish.name}</Typography.Title>
                        <div
                          className="taste-rating-options"
                          aria-label={`${pending.dish.name} Taste rating`}
                        >
                          {tasteRatings.map(({ rating, label }) => {
                            const submissionKey = `${pending.id}:${rating}`;
                            return (
                              <Button
                                key={rating}
                                size="large"
                                autoInsertSpace={false}
                                loading={submittingRating === submissionKey}
                                disabled={
                                  submittingRating !== undefined &&
                                  submittingRating !== submissionKey
                                }
                                onClick={() => {
                                  void ratePending(pending.id, rating);
                                }}
                              >
                                {label}
                              </Button>
                            );
                          })}
                        </div>
                      </article>
                    </List.Item>
                  )}
                />
              </Space>
            </section>
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
              <Button
                block
                type="primary"
                size="large"
                loading={submitting}
                onClick={() => {
                  void beginMeal();
                }}
              >
                开始这一顿
              </Button>
              <Typography.Text type="secondary">
                想先调整选择范围？<Link to="/candidate-pool">管理 Candidate pool</Link>
              </Typography.Text>
            </>
          )}

          {resume?.status === "active_decision" && (
            <section className="decision" aria-labelledby="decision-heading">
              <Space direction="vertical" size={20} className="full-width">
                {resume.decision.mode === "discovery" ? (
                  <>
                    <Tag color="blue">Discovery · 候选池外探索</Tag>
                    {resume.decision.reason && (
                      <Alert type="info" showIcon message={resume.decision.reason} />
                    )}
                  </>
                ) : (
                  <Tag color="orange">普通 Pool pick</Tag>
                )}
                <div>
                  <Typography.Text type="secondary">{resume.decision.dish.category}</Typography.Text>
                  <Typography.Title id="decision-heading" level={2}>
                    {resume.decision.dish.name}
                  </Typography.Title>
                </div>
                <div className="decision-actions" aria-label="Decision 操作">
                  <Button
                    block
                    size="large"
                    loading={rerolling}
                    disabled={submitting}
                    onClick={() => {
                      void rerollDecision(resume.decision.id);
                    }}
                  >
                    Reroll
                  </Button>
                  <Button
                    block
                    type="primary"
                    size="large"
                    loading={submitting}
                    disabled={rerolling}
                    onClick={() => {
                      void acceptDecision(resume.decision.id);
                    }}
                  >
                    就吃这个（Acceptance）
                  </Button>
                </div>
              </Space>
            </section>
          )}
        </Space>
      </Card>
    </main>
  );
}

function RecipePage({ account }: { account: Account }) {
  const [searchParams] = useSearchParams();
  const dishID = searchParams.get("dish_id");
  const [recipe, setRecipe] = useState<Recipe>();
  const [error, setError] = useState<string>();

  useEffect(() => {
    if (!dishID) {
      setError("Recipe 不存在");
      return;
    }
    requestJSON<Recipe>(`/api/catalog/recipes?dish_id=${encodeURIComponent(dishID)}`)
      .then(setRecipe)
      .catch((cause) => {
        setError(cause instanceof Error ? cause.message : "无法读取 Recipe");
      });
  }, [dishID]);

  return (
    <main className="page-shell">
      <Card className="page-card" bordered={false}>
        <Space direction="vertical" size={24} className="full-width">
          <header className="page-header">
            <div className="page-header-row">
              <Typography.Text type="secondary">你好，{account.username}</Typography.Text>
              <Link to="/">开始下一顿</Link>
            </div>
            <Typography.Title level={1}>Recipe</Typography.Title>
          </header>

          {error && <Alert type="error" showIcon message={error} />}
          {!recipe && !error && (
            <div className="inline-loading" aria-label="正在读取 Recipe">
              <Spin />
            </div>
          )}
          {recipe && (
            <article aria-labelledby="recipe-heading">
              <Space direction="vertical" size={16} className="full-width">
                <div>
                  <Tag color="orange">{recipe.dish.category}</Tag>
                  <Typography.Title id="recipe-heading" level={2}>
                    {recipe.dish.name}
                  </Typography.Title>
                  <Typography.Text type="secondary" className="recipe-path">
                    {recipe.dish.id}
                  </Typography.Text>
                </div>
                <Typography.Paragraph className="recipe-content">
                  {recipe.content}
                </Typography.Paragraph>
              </Space>
            </article>
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
        path="/onboarding"
        element={
          account ? <OnboardingPage account={account} /> : <Navigate to="/login" replace />
        }
      />
      <Route
        path="/candidate-pool"
        element={
          account ? <CandidatePoolPage account={account} /> : <Navigate to="/login" replace />
        }
      />
      <Route
        path="/recipes"
        element={
          account ? <RecipePage account={account} /> : <Navigate to="/login" replace />
        }
      />
      <Route
        path="*"
        element={
          account ? <OnboardingEntry account={account} /> : <Navigate to="/login" replace />
        }
      />
    </Routes>
  );
}
