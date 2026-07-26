import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query";

import { ApiError } from "./client";
import * as api from "./endpoints";
import {
  catalogKey,
  mealKey,
  onboardingKey,
  poolKey,
  recipeKey,
  sessionKey,
} from "./keys";
import { clearSessionExpired } from "./queryClient";
import type { OnboardingState, Rating } from "./types";

// ---- 查询（6 个）----

export function useSession() {
  return useQuery({
    queryKey: sessionKey,
    queryFn: ({ signal }) => api.getSession(signal),
    // 会话只在启动时取一次；登录/过期都走 setQueryData
    staleTime: Infinity,
  });
}

export function useOnboardingState() {
  return useQuery({
    queryKey: onboardingKey,
    queryFn: ({ signal }) => api.getOnboarding(signal),
  });
}

export function useMealState() {
  return useQuery({
    queryKey: mealKey,
    queryFn: ({ signal }) => api.resumeMeal(signal),
  });
}

export function useCandidatePool() {
  return useQuery({
    queryKey: poolKey,
    queryFn: ({ signal }) => api.listCandidatePool(signal),
  });
}

export function useCatalogSearch(query: string) {
  return useQuery({
    queryKey: catalogKey(query),
    queryFn: ({ signal }) => api.searchCatalog(query, signal),
    enabled: query !== "",
    placeholderData: keepPreviousData,
    staleTime: 30_000,
  });
}

export function useRecipe(dishId: string) {
  return useQuery({
    queryKey: recipeKey(dishId),
    queryFn: ({ signal }) => api.getRecipe(dishId, signal),
    // dishId 变化即换 key，react-query 自动中止旧请求——旧版竞态在结构上不存在
    enabled: dishId !== "",
    staleTime: 5 * 60_000,
  });
}

// ---- 变更（12 个）与失效映射 ----

function hasCode(error: unknown, ...codes: string[]): boolean {
  return error instanceof ApiError && codes.includes(error.code);
}

function useAuthMutation(authenticate: typeof api.login) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: authenticate,
    onSuccess: (response) => {
      // 账户边界：丢弃上一账户的所有缓存，再写入新会话
      queryClient.removeQueries();
      queryClient.setQueryData(sessionKey, response.account);
      clearSessionExpired();
    },
  });
}

export function useLogin() {
  return useAuthMutation(api.login);
}

export function useRegister() {
  return useAuthMutation(api.register);
}

export function useBeginMeal() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => api.beginMeal(),
    onSuccess: (state) => queryClient.setQueryData(mealKey, state),
    onError: (error) => {
      // 409 表示界面陈旧（已有待评分/池子已空）——重取让界面变成正确状态
      if (hasCode(error, "pending_ratings", "candidate_pool_empty")) {
        void queryClient.invalidateQueries({ queryKey: mealKey });
      }
    },
  });
}

export function useReroll() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (decisionId: number) => api.rerollDecision(decisionId),
    onSuccess: (state) => queryClient.setQueryData(mealKey, state),
    onError: (error) => {
      if (
        hasCode(
          error,
          "candidate_pool_empty",
          "decision_not_found",
          "not_found",
        )
      ) {
        void queryClient.invalidateQueries({ queryKey: mealKey });
      }
    },
  });
}

export function useAccept() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (decisionId: number) => api.acceptDecision(decisionId),
    // 响应是 AcceptanceResponse 而非 MealState：失效重取；页面在自己的
    // onSuccess 里用 response.recipe.dish.id 导航，无需额外请求
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: mealKey }),
    onError: (error) => {
      if (hasCode(error, "decision_not_found", "not_found")) {
        void queryClient.invalidateQueries({ queryKey: mealKey });
      }
    },
  });
}

export function useRatePending() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { pendingRatingId: number; rating: Rating }) =>
      api.ratePending(input.pendingRatingId, input.rating),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: mealKey });
      // pool_admission 会往 Candidate pool 加菜
      void queryClient.invalidateQueries({ queryKey: poolKey });
    },
    onError: (error) => {
      if (
        hasCode(
          error,
          "rating_conflict",
          "pending_rating_not_found",
          "not_found",
        )
      ) {
        void queryClient.invalidateQueries({ queryKey: mealKey });
      }
    },
  });
}

export function useAddPoolDish() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { dish_id: string; preference_weight: number }) =>
      api.addPoolDish(input),
    // meal 一并失效：空池 ↔ ready 可能翻转
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: poolKey });
      void queryClient.invalidateQueries({ queryKey: mealKey });
    },
  });
}

export function useUpdatePoolWeight() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { dish_id: string; preference_weight: number }) =>
      api.updatePoolDish(input),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: poolKey }),
    onError: (error) => {
      if (hasCode(error, "candidate_pool_member_not_found", "not_found")) {
        void queryClient.invalidateQueries({ queryKey: poolKey });
      }
    },
  });
}

export function useRemovePoolDish() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (dishId: string) => api.removePoolDish(dishId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: poolKey });
      void queryClient.invalidateQueries({ queryKey: mealKey });
    },
    onError: (error) => {
      if (hasCode(error, "candidate_pool_member_not_found", "not_found")) {
        void queryClient.invalidateQueries({ queryKey: poolKey });
      }
    },
  });
}

function applyOnboardingResult(
  queryClient: QueryClient,
  state: OnboardingState,
): void {
  queryClient.setQueryData(onboardingKey, state);
  if (state.status === "completed" || state.status === "manual") {
    // 访谈落库了带权重的 Candidate pool；readiness 也随之改变
    void queryClient.invalidateQueries({ queryKey: poolKey });
    void queryClient.invalidateQueries({ queryKey: mealKey });
  }
}

function invalidateOnboardingOnConflict(
  queryClient: QueryClient,
  error: unknown,
): void {
  // nim_unavailable / rate_limited 时服务端已把 failed + can_retry 落库；
  // 重取让「重试上一条 / 改用手工」按钮从 query 状态渲染，刷新后依然在。
  if (
    hasCode(
      error,
      "nim_unavailable",
      "rate_limited",
      "retry_required",
      "retry_unavailable",
      "interview_finished",
      "interview_limit_reached",
    )
  ) {
    void queryClient.invalidateQueries({ queryKey: onboardingKey });
  }
}

export function useSendOnboardingMessage() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (message: string) => api.sendOnboardingMessage(message),
    onSuccess: (state) => applyOnboardingResult(queryClient, state),
    onError: (error) => invalidateOnboardingOnConflict(queryClient, error),
  });
}

export function useRetryOnboarding() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => api.retryOnboarding(),
    onSuccess: (state) => applyOnboardingResult(queryClient, state),
    onError: (error) => invalidateOnboardingOnConflict(queryClient, error),
  });
}

export function useManualOnboarding() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => api.manualOnboarding(),
    onSuccess: (state) => applyOnboardingResult(queryClient, state),
    onError: (error) => invalidateOnboardingOnConflict(queryClient, error),
  });
}
