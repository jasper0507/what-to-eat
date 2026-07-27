import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";

import { ApiError } from "./client";
import * as api from "./endpoints";
import {
  catalogKey,
  historyKey,
  mealKey,
  poolKey,
  recipeKey,
  sessionKey,
} from "./keys";
import { clearSessionExpired } from "./queryClient";
import type { Rating } from "./types";

// ---- 查询 ----

export function useSession() {
  return useQuery({
    queryKey: sessionKey,
    queryFn: ({ signal }) => api.getSession(signal),
    // 会话只在启动时取一次；登录/过期都走 setQueryData
    staleTime: Infinity,
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

export function useEatingRecords() {
  return useQuery({
    queryKey: historyKey,
    queryFn: ({ signal }) => api.listEatingRecords(signal),
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

// ---- 变更与失效映射 ----

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

export function useLogout() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => api.logout(),
    // 账户边界：先把 session 置 null（removeQueries 不通知活跃观察者，
    // 顺序反了 RequireSession 会攥着已移除的旧 query 永远等不到跳转），
    // 再丢弃这台设备上的其余一切缓存。
    onSuccess: () => {
      queryClient.setQueryData(sessionKey, null);
      queryClient.removeQueries({
        predicate: (query) => query.queryKey[0] !== sessionKey[0],
      });
    },
  });
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
          "reroll_budget_exhausted",
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
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: mealKey });
      void queryClient.invalidateQueries({ queryKey: historyKey });
    },
    onError: (error) => {
      if (hasCode(error, "decision_not_found", "not_found")) {
        void queryClient.invalidateQueries({ queryKey: mealKey });
      }
    },
  });
}

/** 三出口·放弃本顿。 */
export function useAbandonMeal() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => api.abandonMeal(),
    onSuccess: (state) => queryClient.setQueryData(mealKey, state),
    onError: (error) => {
      // 没有进行中的这一顿：界面陈旧，重取即回正确状态
      if (hasCode(error, "meal_not_found", "not_found")) {
        void queryClient.invalidateQueries({ queryKey: mealKey });
      }
    },
  });
}

/** 三出口·亲自点一道（仅额度耗尽时解锁）。 */
export function useHandPick() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (dishId: string) => api.handPickDish(dishId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: mealKey });
      void queryClient.invalidateQueries({ queryKey: historyKey });
    },
    onError: (error) => {
      if (hasCode(error, "meal_not_found", "hand_pick_locked", "not_found")) {
        void queryClient.invalidateQueries({ queryKey: mealKey });
      }
    },
  });
}

/** 轻历史的补评分：可选、绝不拦路。 */
export function useRateRecord() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { recordId: number; rating: Rating }) =>
      api.rateEatingRecord(input.recordId, input.rating),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: historyKey });
      // pool_admission / rejection_mark 都可能改变池子与 readiness
      void queryClient.invalidateQueries({ queryKey: poolKey });
      void queryClient.invalidateQueries({ queryKey: mealKey });
    },
    onError: (error) => {
      if (
        hasCode(
          error,
          "rating_conflict",
          "eating_record_not_found",
          "not_found",
        )
      ) {
        void queryClient.invalidateQueries({ queryKey: historyKey });
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
    mutationFn: (input: { dish_id: string; tier: number }) =>
      api.addPoolDish(input),
    // meal 一并失效：空池 ↔ ready 可能翻转
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: poolKey });
      void queryClient.invalidateQueries({ queryKey: mealKey });
    },
  });
}

/**
 * 经典起步包：十余道国民家常菜逐一入池，默认中档。
 * 单道失败（已在池、暂不可加）跳过不打断——起步包是引导，不是事务。
 */
export function useStarterPack() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (dishes: readonly { id: string; tier: number }[]) => {
      let added = 0;
      for (const dish of dishes) {
        try {
          await api.addPoolDish({ dish_id: dish.id, tier: dish.tier });
          added += 1;
        } catch (error) {
          if (!hasCode(error, "dish_unavailable", "invalid_request")) {
            throw error;
          }
        }
      }
      return added;
    },
    onSettled: () => {
      // 部分成功也要反映到界面：池子与 readiness 一并重取
      void queryClient.invalidateQueries({ queryKey: poolKey });
      void queryClient.invalidateQueries({ queryKey: mealKey });
    },
  });
}

export function useUpdatePoolWeight() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { dish_id: string; tier: number }) =>
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
