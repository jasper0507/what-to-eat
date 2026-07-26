import { ApiError, apiFetch } from "./client";
import type {
  AcceptanceResponse,
  Account,
  Credentials,
  Dish,
  EatingRecordEntry,
  MealState,
  OnboardingState,
  PoolDish,
  Rating,
  RecipeResponse,
  TasteRatingResponse,
} from "./types";

/** 场合因子的客户端上报（ADR-0022）：揭示按本机小时理解「这一顿」。 */
function localHour(): number {
  return new Date().getHours();
}

export function register(
  credentials: Credentials,
): Promise<{ account: Account }> {
  return apiFetch("POST", "/api/auth/register", { body: credentials });
}

export function login(credentials: Credentials): Promise<{ account: Account }> {
  return apiFetch("POST", "/api/auth/login", { body: credentials });
}

/** 幂等：没 cookie、token 已失效都 204。 */
export function logout(): Promise<void> {
  return apiFetch("POST", "/api/auth/logout");
}

/** 匿名（unauthorized）返回 null——首屏未登录不是错误；其余错误照抛。 */
export async function getSession(
  signal?: AbortSignal,
): Promise<Account | null> {
  try {
    const response = await apiFetch<{ account: Account }>(
      "GET",
      "/api/auth/session",
      { signal },
    );
    return response.account;
  } catch (cause) {
    if (cause instanceof ApiError && cause.code === "unauthorized") {
      return null;
    }
    throw cause;
  }
}

export async function searchCatalog(
  query: string,
  signal?: AbortSignal,
): Promise<Dish[]> {
  const response = await apiFetch<{ dishes: Dish[] }>(
    "GET",
    `/api/catalog/dishes?q=${encodeURIComponent(query)}`,
    { signal },
  );
  return response.dishes;
}

export function getRecipe(
  dishId: string,
  signal?: AbortSignal,
): Promise<RecipeResponse> {
  return apiFetch(
    "GET",
    `/api/catalog/recipes?dish_id=${encodeURIComponent(dishId)}`,
    { signal },
  );
}

export async function listCandidatePool(
  signal?: AbortSignal,
): Promise<PoolDish[]> {
  const response = await apiFetch<{ dishes: PoolDish[] }>(
    "GET",
    "/api/candidate-pool/dishes",
    {
      signal,
    },
  );
  return response.dishes;
}

export function addPoolDish(input: {
  dish_id: string;
  tier: number;
}): Promise<void> {
  return apiFetch("POST", "/api/candidate-pool/dishes", { body: input });
}

export function updatePoolDish(input: {
  dish_id: string;
  tier: number;
}): Promise<void> {
  return apiFetch("PATCH", "/api/candidate-pool/dishes", { body: input });
}

export function removePoolDish(dishId: string): Promise<void> {
  return apiFetch(
    "DELETE",
    `/api/candidate-pool/dishes?dish_id=${encodeURIComponent(dishId)}`,
  );
}

export function getOnboarding(signal?: AbortSignal): Promise<OnboardingState> {
  return apiFetch("GET", "/api/onboarding/interview", { signal });
}

export function sendOnboardingMessage(
  message: string,
): Promise<OnboardingState> {
  return apiFetch("POST", "/api/onboarding/interview/messages", {
    body: { message },
  });
}

export function retryOnboarding(): Promise<OnboardingState> {
  return apiFetch("POST", "/api/onboarding/interview/retry");
}

export function manualOnboarding(): Promise<OnboardingState> {
  return apiFetch("POST", "/api/onboarding/interview/manual");
}

export function resumeMeal(signal?: AbortSignal): Promise<MealState> {
  return apiFetch("GET", "/api/meals/resume", { signal });
}

/** 已有 active Decision 时后端幂等返回 200 + 同一状态。 */
export function beginMeal(): Promise<MealState> {
  return apiFetch("POST", "/api/meals", { body: { local_hour: localHour() } });
}

export function rerollDecision(decisionId: number): Promise<MealState> {
  return apiFetch("POST", `/api/decisions/${decisionId}/reroll`, {
    body: { local_hour: localHour() },
  });
}

export function acceptDecision(
  decisionId: number,
): Promise<AcceptanceResponse> {
  return apiFetch("POST", `/api/decisions/${decisionId}/accept`);
}

/** 三出口·放弃本顿：Meal → abandoned，无吃饭记录，不进冷却。 */
export function abandonMeal(): Promise<MealState> {
  return apiFetch("POST", "/api/meals/abandon");
}

/** 三出口·亲自点一道：仅 Reroll budget 耗尽时解锁（否则 hand_pick_locked）。 */
export function handPickDish(dishId: string): Promise<AcceptanceResponse> {
  return apiFetch("POST", "/api/meals/hand-pick", {
    body: { dish_id: dishId, local_hour: localHour() },
  });
}

/** 轻历史：最近吃过、评过几档、现在池里几档。 */
export async function listEatingRecords(
  limit: number,
  signal?: AbortSignal,
): Promise<EatingRecordEntry[]> {
  const response = await apiFetch<{ records: EatingRecordEntry[] }>(
    "GET",
    `/api/eating-records?limit=${limit}`,
    { signal },
  );
  return response.records;
}

/** 轻历史的补评分：可选、绝不拦路。 */
export function rateEatingRecord(
  recordId: number,
  rating: Rating,
): Promise<TasteRatingResponse> {
  return apiFetch("POST", `/api/eating-records/${recordId}/rate`, {
    body: { rating },
  });
}

export function ratePending(
  pendingRatingId: number,
  rating: Rating,
): Promise<TasteRatingResponse> {
  return apiFetch("POST", `/api/pending-ratings/${pendingRatingId}/rate`, {
    body: { rating },
  });
}
