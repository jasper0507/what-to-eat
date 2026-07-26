import { Alert, App as AntApp, Button, Empty } from "antd";
import { CircleCheck } from "lucide-react";
import { m } from "motion/react";
import { Link, Navigate, useNavigate } from "react-router-dom";

import { ApiError } from "@/api/client";
import {
  useAccept,
  useBeginMeal,
  useMealState,
  useOnboardingState,
  useRatePending,
  useReroll,
} from "@/api/hooks";
import { DecisionPanel } from "@/components/DecisionPanel";
import { ErrorAlert } from "@/components/ErrorAlert";
import { LoadingBlock } from "@/components/LoadingBlock";
import { PageHeader } from "@/components/PageHeader";
import { PendingRatingsPanel } from "@/components/PendingRatingsPanel";
import { copy } from "@/lib/copy";
import { pageEnter } from "@/lib/motion";

// 这两个 409 语义是「界面陈旧」而非失败：hooks 已 invalidate，
// 界面会自己变成正确状态，不该再弹错误。
function visibleError(error: unknown): unknown {
  if (
    error instanceof ApiError &&
    (error.code === "pending_ratings" || error.code === "candidate_pool_empty")
  ) {
    return null;
  }
  return error;
}

export default function HomePage() {
  const { message } = AntApp.useApp();
  const navigate = useNavigate();
  // 门控与 Meal 状态同 tick 并行发起——旧版的请求瀑布在结构上不存在
  const onboarding = useOnboardingState();
  const meal = useMealState();
  const begin = useBeginMeal();
  const reroll = useReroll();
  const accept = useAccept();
  const rate = useRatePending();

  if (onboarding.isPending || meal.isPending) {
    return (
      <div className="container page-stack">
        <PageHeader title={copy.home.title} />
        <LoadingBlock preset="home" label={copy.home.loadingLabel} />
      </div>
    );
  }

  // 门控完全信任服务端状态（池子非空且未访谈时后端自动报 manual）
  if (
    onboarding.data &&
    onboarding.data.status !== "completed" &&
    onboarding.data.status !== "manual"
  ) {
    return <Navigate to="/onboarding" replace />;
  }

  if (onboarding.isError || meal.isError) {
    const failed = onboarding.isError ? onboarding : meal;
    return (
      <div className="container page-stack">
        <PageHeader title={copy.home.title} />
        <ErrorAlert
          error={failed.error}
          onRetry={() => void failed.refetch()}
        />
      </div>
    );
  }
  if (!meal.data) {
    return null;
  }

  const state = meal.data;
  const decision = state.status === "active_decision" ? state.decision : null;
  const pendingRatings =
    state.status === "pending_ratings" ? state.pending_ratings : null;
  const mutationError = visibleError(
    accept.error ?? reroll.error ?? begin.error ?? rate.error,
  );

  return (
    <m.div {...pageEnter} className="container page-stack">
      <PageHeader title={copy.home.title} />
      <ErrorAlert error={mutationError} />

      {state.status === "ready" ? (
        <div className="page-stack page-stack-tight">
          <Alert
            type="success"
            showIcon
            icon={<CircleCheck size={20} />}
            message={copy.home.readyBadge}
            description={copy.home.readyIntro}
          />
          <Button
            block
            type="primary"
            size="large"
            loading={begin.isPending}
            onClick={() => begin.mutate()}
          >
            {copy.home.begin}
          </Button>
          <p className="ready-hint">
            {copy.home.poolHint}{" "}
            <Link to="/candidate-pool">{copy.home.managePool}</Link>
          </p>
        </div>
      ) : null}

      {state.status === "candidate_pool_empty" ? (
        <div className="page-stack page-stack-tight">
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            description={copy.home.emptyText}
          />
          <Button
            block
            type="primary"
            size="large"
            href="/candidate-pool"
            onClick={(event) => {
              event.preventDefault();
              void navigate("/candidate-pool");
            }}
          >
            {copy.home.emptyCta}
          </Button>
        </div>
      ) : null}

      {decision ? (
        <DecisionPanel
          decision={decision}
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
      ) : null}

      {pendingRatings ? (
        <PendingRatingsPanel
          pendingRatings={pendingRatings}
          busy={rate.isPending ? rate.variables : undefined}
          onRate={(pendingRatingId, rating) =>
            rate.mutate(
              { pendingRatingId, rating },
              {
                onSuccess: () => {
                  void message.success(copy.home.ratingRecorded);
                },
              },
            )
          }
        />
      ) : null}
    </m.div>
  );
}
