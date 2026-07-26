import { ApiError, apiFetch } from "./client";
import type {
  AcceptanceResponse,
  Account,
  Credentials,
  Dish,
  MealState,
  OnboardingState,
  PoolDish,
  Rating,
  RecipeResponse,
  TasteRatingResponse,
} from "./types";

export function register(
  credentials: Credentials,
): Promise<{ account: Account }> {
  return apiFetch("POST", "/api/auth/register", { body: credentials });
}

export function login(credentials: Credentials): Promise<{ account: Account }> {
  return apiFetch("POST", "/api/auth/login", { body: credentials });
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
  return apiFetch("POST", "/api/meals");
}

export function rerollDecision(decisionId: number): Promise<MealState> {
  return apiFetch("POST", `/api/decisions/${decisionId}/reroll`);
}

export function acceptDecision(
  decisionId: number,
): Promise<AcceptanceResponse> {
  return apiFetch("POST", `/api/decisions/${decisionId}/accept`);
}

export function ratePending(
  pendingRatingId: number,
  rating: Rating,
): Promise<TasteRatingResponse> {
  return apiFetch("POST", `/api/pending-ratings/${pendingRatingId}/rate`, {
    body: { rating },
  });
}
