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
  /** 导入富化元数据：仅揭示与菜谱页路径填充（信息小字：耗时·难度） */
  difficulty?: number;
  cook_minutes?: number;
  calories?: number;
}

/** 档位（Taste rating 上三档）：3 人上人 / 4 顶尖 / 5 夯 */
export type Tier = 3 | 4 | 5;

export type PoolDish = Dish & { tier: Tier };

export interface Decision {
  id: number;
  meal_id: number;
  mode: "pool" | "discovery" | "hand_pick";
  /** ADR-0022：每次揭示必有理由行 */
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
  | {
      status: "active_decision";
      decision: Decision;
      /** Reroll budget（每顿 3 次）的余量 */
      rerolls_remaining: number;
    }
  | { status: "pending_ratings"; pending_ratings: PendingRating[] };

export type Rating = 1 | 2 | 3 | 4 | 5;

/** 无损镜像 accept / hand-pick 响应：eating_record 与 pending_rating 不许丢弃。 */
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
  tier?: Tier;
  dish: Dish;
}

/** 轻历史条目：最近吃过、评过几档、现在池里几档。 */
export interface EatingRecordEntry {
  id: number;
  sequence: number;
  dish: Dish;
  mode: "pool" | "discovery" | "hand_pick";
  /** unix 秒 */
  accepted_at: number;
  rating?: Rating;
  pool_tier?: Tier;
}

export interface RecipeResponse {
  dish: Dish;
  content: string;
  /** Catalog 相对路径（拼 /api/catalog/assets/ 前缀）或外链 URL */
  images: string[];
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
  | "pending_ratings"
  | "candidate_pool_empty"
  | "reroll_budget_exhausted"
  | "hand_pick_locked"
  | "meal_not_found"
  | "eating_record_not_found"
  | "decision_not_found"
  | "pending_rating_not_found"
  | "rating_conflict"
  | "not_found"
  | "network_error"
  | "unexpected_response"
  | (string & {});
