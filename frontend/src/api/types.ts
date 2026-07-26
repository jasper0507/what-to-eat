// 与 Go 后端 wire 格式一一对应的 DTO（字段名 = JSON 序列化名）。
// 身份规则：导航与变更只允许使用 dish.id；recipe_path 仅用于展示。
// （今日两者恰好同值——都是 Catalog 源路径——但这是实现巧合，绝不依赖。）

export interface Account {
  id: number;
  username: string;
}

export interface Credentials {
  username: string;
  password: string;
}

export interface Dish {
  id: string;
  name: string;
  category: string;
  recipe_path: string;
  tags: string[];
}

export type PoolDish = Dish & { preference_weight: number };

export interface Decision {
  id: number;
  meal_id: number;
  mode: "pool" | "discovery";
  reason?: string;
  dish: Dish;
}

export interface PendingRating {
  id: number;
  meal_id: number;
  /** unix 秒（前端展示时 ×1000） */
  meal_at: number;
  dish: Dish;
}

export type MealState =
  | { status: "candidate_pool_empty" }
  | { status: "ready" }
  | { status: "active_decision"; decision: Decision }
  | { status: "pending_ratings"; pending_ratings: PendingRating[] };

export type Rating = 1 | 2 | 3 | 4 | 5;

/** 无损镜像 accept 响应：eating_record 与 pending_rating 不许丢弃。 */
export interface AcceptanceResponse {
  eating_record: { sequence: number };
  recipe: { dish: Dish };
  /** 仅 discovery Decision 被接受时出现 */
  pending_rating?: PendingRating;
}

export interface TasteRatingResponse {
  pending_rating_id: number;
  rating: Rating;
  outcome: "pool_admission" | "rejection_mark";
  /** 评 1–2（rejection_mark）时缺省 */
  preference_weight?: number;
  dish: Dish;
}

export type OnboardingStatus =
  "not_started" | "in_progress" | "failed" | "completed" | "manual";

export interface OnboardingMessage {
  role: "user" | "assistant";
  content: string;
}

export interface OnboardingState {
  status: OnboardingStatus;
  messages: OnboardingMessage[];
  can_retry: boolean;
}

export interface RecipeResponse {
  dish: Dish;
  content: string;
}

/** 服务端错误信封里的 code，外加两个客户端归一化 code。 */
export type ApiErrorCode =
  | "unauthorized"
  | "invalid_request"
  | "invalid_credentials"
  | "account_unavailable"
  | "rate_limited"
  | "invalid_query"
  | "recipe_not_found"
  | "dish_unavailable"
  | "candidate_pool_member_not_found"
  | "nim_unavailable"
  | "interview_limit_reached"
  | "retry_required"
  | "retry_unavailable"
  | "interview_finished"
  | "pending_ratings"
  | "candidate_pool_empty"
  | "decision_not_found"
  | "pending_rating_not_found"
  | "rating_conflict"
  | "not_found"
  | "network_error"
  | "unexpected_response"
  | (string & {});
