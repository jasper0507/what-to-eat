import { LoaderCircle } from "lucide-react";
import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";

import { ApiError } from "@/api/client";
import {
  useAbandonMeal,
  useAccept,
  useBeginMeal,
  useCandidatePool,
  useEatingRecords,
  useHandPick,
  useMealState,
  useRatePending,
  useReroll,
  useStarterPack,
} from "@/api/hooks";
import type { Decision, Dish, PendingRating } from "@/api/types";
import { Notice } from "@/components/Notice";
import { TierBadge, TierScale } from "@/components/TierBadge";
import { Button, buttonVariants } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { formatMealAt, mealAtISO } from "@/lib/format";
import { STARTER_PACK } from "@/lib/starterPack";
import { DEFAULT_POOL_TIER, RATING_TIERS, TIER_LABELS } from "@/lib/tiers";

// 主界面 = Decision stage 的舞台位：三态互斥（空池引导 / 待评分拦截 / 就绪开饭），
// 揭示就地完成，无路由跳转；接受才离场（去菜谱页）。
export default function HomePage() {
  const navigate = useNavigate();
  const meal = useMealState();
  const begin = useBeginMeal();
  const reroll = useReroll();
  const accept = useAccept();

  if (meal.isPending) {
    return (
      <div className="mx-auto max-w-3xl">
        <Stage>
          <div
            role="status"
            aria-label="正在看这一顿的状态"
            className="flex min-h-72 items-center justify-center"
          >
            <LoaderCircle
              aria-hidden="true"
              className="size-5 animate-spin text-muted-foreground"
            />
          </div>
        </Stage>
      </div>
    );
  }
  if (meal.isError) {
    return (
      <div className="mx-auto max-w-3xl">
        <Stage>
          <div className="flex min-h-72 items-center justify-center px-6">
            <Notice tone="error" onRetry={() => void meal.refetch()}>
              {meal.error.message}
            </Notice>
          </div>
        </Stage>
      </div>
    );
  }
  if (!meal.data) {
    return null;
  }
  const state = meal.data;

  if (state.status === "candidate_pool_empty") {
    return <EmptyPoolWelcome />;
  }
  if (state.status === "pending_ratings") {
    return (
      <Cockpit>
        <PendingRatingsGate pendingRatings={state.pending_ratings} />
      </Cockpit>
    );
  }

  const decision = state.status === "active_decision" ? state.decision : null;
  const mutationError = visibleError(
    accept.error ?? reroll.error ?? begin.error,
  );

  return (
    <Cockpit>
      {mutationError ? (
        <Notice tone="error">{(mutationError as Error).message}</Notice>
      ) : null}

      {decision ? (
        <Reveal
          decision={decision}
          rerollsRemaining={
            state.status === "active_decision" ? state.rerolls_remaining : 0
          }
          rerolling={reroll.isPending}
          accepting={accept.isPending}
          onReroll={() => reroll.mutate(decision.id)}
          onAccept={() =>
            accept.mutate(decision.id, {
              onSuccess: (result) => {
                void navigate(
                  `/recipes?dish_id=${encodeURIComponent(result.recipe.dish.id)}`,
                );
              },
            })
          }
        />
      ) : (
        <Stage>
          <div className="flex min-h-72 flex-col items-center justify-center gap-6 px-6 py-12 text-center lg:min-h-96">
            <span
              aria-hidden="true"
              className="font-serif text-6xl leading-none text-brand"
            >
              ？
            </span>
            <div className="space-y-1.5">
              <h1 className="font-serif text-2xl font-medium">
                这一顿，交给池子。
              </h1>
              <p className="text-sm text-muted-foreground">
                合适的那道会自己站出来。
              </p>
            </div>
            <Button
              size="lg"
              className="min-w-44"
              aria-busy={begin.isPending}
              disabled={begin.isPending}
              onClick={() => begin.mutate()}
            >
              {begin.isPending ? (
                <LoaderCircle
                  aria-hidden="true"
                  className="size-4 animate-spin"
                />
              ) : null}
              开始这一顿
            </Button>
          </div>
        </Stage>
      )}
    </Cockpit>
  );
}

/** 舞台容器：白底面 + 发丝边框，主界面唯一的分层。 */
function Stage({ children }: { children: React.ReactNode }) {
  return (
    <section className="rounded-xl border border-border bg-card">
      {children}
    </section>
  );
}

/**
 * 驾驶舱布局：舞台居中吸睛，桌面端左右各一条安静的信息栏（左·池子概览，
 * 右·最近吃过），对称的发丝竖线把视线夹向中央。栏是纯文字，永不抢戏。
 */
function Cockpit({ children }: { children: React.ReactNode }) {
  return (
    <div className="lg:grid lg:grid-cols-[minmax(0,13rem)_minmax(0,1fr)_minmax(0,13rem)] lg:items-stretch lg:gap-10 xl:gap-14">
      <PoolRail />
      <div className="space-y-4 lg:min-w-0">{children}</div>
      <RecentRail />
    </div>
  );
}

const railLink =
  "inline-block rounded-sm text-brand-ink underline decoration-brand-ink/40 underline-offset-4 outline-none hover:decoration-brand-ink focus-visible:ring-2 focus-visible:ring-ring";

function PoolRail() {
  const pool = useCandidatePool();

  return (
    <aside
      aria-label="池子概览"
      className="hidden space-y-3 border-r border-border pr-8 pt-1 text-sm lg:block"
    >
      <div className="flex items-baseline justify-between gap-3">
        <h2 className="font-medium">池子</h2>
        {pool.data ? (
          <span className="text-muted-foreground">{pool.data.length} 道</span>
        ) : null}
      </div>
      {pool.data && pool.data.length > 0 ? (
        <ul className="space-y-2">
          {pool.data.slice(0, 8).map((dish) => (
            <li
              key={dish.id}
              className="flex items-baseline justify-between gap-3"
            >
              <span className="truncate">{dish.name}</span>
              <span className="shrink-0 text-muted-foreground">
                {TIER_LABELS[dish.tier]}
              </span>
            </li>
          ))}
        </ul>
      ) : null}
      <Link to="/candidate-pool" className={railLink}>
        去管池子
      </Link>
    </aside>
  );
}

function RecentRail() {
  const records = useEatingRecords();
  const recent = (records.data ?? []).slice(0, 8);

  return (
    <aside
      aria-label="最近吃过"
      className="hidden space-y-3 border-l border-border pl-8 pt-1 text-sm lg:block"
    >
      <h2 className="font-medium">最近吃过</h2>
      {recent.length > 0 ? (
        <ul className="space-y-2">
          {recent.map((record) => (
            <li
              key={record.id}
              className="flex items-baseline justify-between gap-3"
            >
              <span className="truncate">{record.dish.name}</span>
              <span className="shrink-0 text-muted-foreground">
                {shortDate(record.accepted_at)}
              </span>
            </li>
          ))}
        </ul>
      ) : (
        <p className="text-muted-foreground">这里会记下你吃过的每一顿。</p>
      )}
      <Link to="/history" className={railLink}>
        看全部
      </Link>
    </aside>
  );
}

const shortDateFormatter = new Intl.DateTimeFormat("zh-CN", {
  month: "long",
  day: "numeric",
});

function shortDate(unixSeconds: number): string {
  return shortDateFormatter.format(new Date(unixSeconds * 1000));
}

// 这两个 409 语义是「界面陈旧」而非失败：hooks 已 invalidate，
// 界面会自己变成正确状态，不该再弹错误。
function visibleError(error: unknown): unknown {
  if (
    error instanceof ApiError &&
    (error.code === "pending_ratings" ||
      error.code === "candidate_pool_empty" ||
      error.code === "reroll_budget_exhausted")
  ) {
    return null;
  }
  return error;
}

/** 揭示：一拍期待 → 菜名衬线大字落定 → 理由行 + 信息小字 → 出口。 */
function Reveal({
  decision,
  rerollsRemaining,
  rerolling,
  accepting,
  onReroll,
  onAccept,
}: {
  decision: Decision;
  rerollsRemaining: number;
  rerolling: boolean;
  accepting: boolean;
  onReroll: () => void;
  onAccept: () => void;
}) {
  const exhausted = rerollsRemaining <= 0;
  const busy = rerolling || accepting;
  const meta = dishMeta(decision.dish);

  return (
    <Stage>
      <div className="flex min-h-72 flex-col px-6 py-8 sm:px-10 lg:min-h-96 lg:px-12 lg:py-10">
        <p className="text-sm text-muted-foreground">
          这一顿
          {decision.mode === "discovery" ? (
            <span className="ml-2 inline-flex h-6 items-center rounded-full border border-brand/30 bg-brand/10 px-2.5 text-sm text-brand-ink">
              池子外的新尝试
            </span>
          ) : null}
        </p>

        {/* key=decision.id：换字即重放揭示序列，连续 Reroll 随时打断；
            aria-live 挂在稳定父节点上，读屏才能听到换字 */}
        <div aria-live="polite" className="flex flex-1 flex-col">
          <div
            key={decision.id}
            className="relative flex flex-1 flex-col justify-center py-8"
          >
            <span
              aria-hidden="true"
              className="animate-tease absolute font-serif text-5xl text-brand"
            >
              ？
            </span>
            <h2 className="animate-reveal-name font-serif text-4xl leading-tight font-medium tracking-wide sm:text-5xl">
              {decision.dish.name}
            </h2>
            <div className="animate-reveal-reason mt-3 space-y-1">
              {decision.reason ? (
                <p className="text-foreground/80">{decision.reason}</p>
              ) : null}
              {meta ? (
                <p className="text-sm text-muted-foreground">{meta}</p>
              ) : null}
            </div>
          </div>
        </div>

        {exhausted ? (
          <ExhaustedExits accepting={accepting} onAccept={onAccept} />
        ) : (
          <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
            <Button
              variant="outline"
              size="lg"
              disabled={busy}
              aria-busy={rerolling}
              onClick={onReroll}
            >
              {rerolling ? (
                <LoaderCircle
                  aria-hidden="true"
                  className="size-4 animate-spin"
                />
              ) : null}
              换一道 · 剩 {rerollsRemaining} 次
            </Button>
            <Button
              size="lg"
              disabled={busy}
              aria-busy={accepting}
              onClick={onAccept}
            >
              {accepting ? (
                <LoaderCircle
                  aria-hidden="true"
                  className="size-4 animate-spin"
                />
              ) : null}
              就吃这个
            </Button>
          </div>
        )}
      </div>
    </Stage>
  );
}

/** 额度用尽的三出口：接受 / 亲自点一道（仅此刻解锁）/ 放弃本顿。 */
function ExhaustedExits({
  accepting,
  onAccept,
}: {
  accepting: boolean;
  onAccept: () => void;
}) {
  const navigate = useNavigate();
  const [picking, setPicking] = useState(false);
  const pool = useCandidatePool();
  const handPick = useHandPick();
  const abandon = useAbandonMeal();
  const busy = accepting || handPick.isPending || abandon.isPending;

  return (
    <div className="space-y-4">
      <p className="border-t border-border pt-4 text-sm text-muted-foreground">
        换菜次数用完了。这顿的结局你来定：
      </p>
      {handPick.error ? (
        <Notice tone="error">{handPick.error.message}</Notice>
      ) : null}
      {abandon.error ? (
        <Notice tone="error">{abandon.error.message}</Notice>
      ) : null}
      <div className="flex flex-col gap-2 sm:flex-row sm:justify-end">
        <Button
          variant="ghost"
          size="lg"
          className="text-muted-foreground"
          disabled={busy}
          aria-busy={abandon.isPending}
          onClick={() => abandon.mutate()}
        >
          这顿不吃了
        </Button>
        <Button
          variant="outline"
          size="lg"
          disabled={busy}
          aria-expanded={picking}
          onClick={() => setPicking((open) => !open)}
        >
          亲自点一道
        </Button>
        <Button
          size="lg"
          disabled={busy}
          aria-busy={accepting}
          onClick={onAccept}
        >
          {accepting ? (
            <LoaderCircle aria-hidden="true" className="size-4 animate-spin" />
          ) : null}
          就吃这个
        </Button>
      </div>

      {picking ? (
        <div className="rounded-md border border-border">
          {pool.isPending ? (
            <p className="px-4 py-3 text-sm text-muted-foreground">
              正在翻池子……
            </p>
          ) : null}
          {pool.isError ? (
            <div className="p-3">
              <Notice tone="error" onRetry={() => void pool.refetch()}>
                {pool.error.message}
              </Notice>
            </div>
          ) : null}
          <ul className="max-h-72 divide-y divide-border overflow-y-auto">
            {(pool.data ?? []).map((dish) => (
              <li key={dish.id}>
                <button
                  type="button"
                  disabled={busy}
                  onClick={() =>
                    handPick.mutate(dish.id, {
                      onSuccess: (result) => {
                        void navigate(
                          `/recipes?dish_id=${encodeURIComponent(result.recipe.dish.id)}`,
                        );
                      },
                    })
                  }
                  className="flex w-full cursor-pointer items-center justify-between gap-3 px-4 py-2.5 text-left outline-none transition-colors duration-150 hover:bg-accent focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset disabled:pointer-events-none disabled:opacity-50"
                >
                  <span className="flex items-center gap-2.5">
                    {handPick.isPending && handPick.variables === dish.id ? (
                      <LoaderCircle
                        aria-hidden="true"
                        className="size-4 animate-spin text-muted-foreground"
                      />
                    ) : null}
                    {dish.name}
                  </span>
                  <TierBadge tier={dish.tier} />
                </button>
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </div>
  );
}

/** 空池态即欢迎引导（注册后无弹窗）：起步包 / 自己挑，两条路入池。 */
function EmptyPoolWelcome() {
  const starter = useStarterPack();

  return (
    <div className="mx-auto max-w-3xl">
      <Stage>
        <div className="flex min-h-72 flex-col items-center justify-center gap-7 px-6 py-14 text-center lg:min-h-96">
          <span
            aria-hidden="true"
            className="font-serif text-6xl leading-none text-brand"
          >
            ？
          </span>
          <div className="max-w-md space-y-1.5">
            <h1 className="font-serif text-2xl font-medium">池子还空着。</h1>
            <p className="text-sm text-muted-foreground">
              放几道你爱吃的进来，这一顿才有得挑。
            </p>
          </div>
          {starter.error ? (
            <Notice tone="error">{starter.error.message}</Notice>
          ) : null}
          <div className="flex w-full max-w-xs flex-col gap-2">
            <Button
              size="lg"
              aria-busy={starter.isPending}
              disabled={starter.isPending}
              onClick={() =>
                starter.mutate(
                  STARTER_PACK.map((dish) => ({
                    id: dish.id,
                    tier: DEFAULT_POOL_TIER,
                  })),
                )
              }
            >
              {starter.isPending ? (
                <LoaderCircle
                  aria-hidden="true"
                  className="size-4 animate-spin"
                />
              ) : null}
              经典起步包 · {STARTER_PACK.length} 道家常菜入池
            </Button>
            <Link
              to="/candidate-pool"
              className={cn(buttonVariants({ variant: "link" }), "w-full")}
            >
              自己去挑菜
            </Link>
          </div>
        </div>
      </Stage>
    </div>
  );
}

/** 待评分拦截：先说说上一顿，才好开始新的一顿。 */
function PendingRatingsGate({
  pendingRatings,
}: {
  pendingRatings: PendingRating[];
}) {
  const rate = useRatePending();

  return (
    <Stage>
      <div className="space-y-5 px-6 py-8 sm:px-10">
        <div className="space-y-1.5">
          <h1 className="font-serif text-2xl font-medium">先说说上一顿。</h1>
          <p className="text-sm text-muted-foreground">
            好不好吃，一句话的事；说完就能开新的一顿。
          </p>
        </div>
        {rate.error ? <Notice tone="error">{rate.error.message}</Notice> : null}
        <ul className="space-y-4">
          {pendingRatings.map((pending) => (
            <li
              key={pending.id}
              className="space-y-3 rounded-md border border-border px-4 py-3.5"
            >
              <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
                <span className="font-serif text-lg font-medium">
                  {pending.dish.name}
                </span>
                <time
                  dateTime={mealAtISO(pending.meal_at)}
                  className="text-sm text-muted-foreground"
                >
                  {formatMealAt(pending.meal_at)}
                </time>
              </div>
              <TierScale
                tiers={RATING_TIERS}
                disabled={rate.isPending}
                onSelect={(rating) =>
                  rate.mutate({ pendingRatingId: pending.id, rating })
                }
              />
            </li>
          ))}
        </ul>
      </div>
    </Stage>
  );
}

/** 信息小字：耗时 · 难度（缺谁略谁，全缺整行不出）。 */
function dishMeta(dish: Dish): string | null {
  const parts: string[] = [];
  if (dish.cook_minutes) {
    parts.push(`约 ${dish.cook_minutes} 分钟`);
  }
  if (dish.difficulty) {
    const stars = ["一", "二", "三", "四", "五"][dish.difficulty - 1];
    if (stars) {
      parts.push(`难度${stars}星`);
    }
  }
  return parts.length > 0 ? parts.join(" · ") : null;
}
