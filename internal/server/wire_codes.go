package server

// HTTP wire 错误码目录：对外契约（frontend/src/api/types.ts 的 union）的唯一
// 后端出处。内部状态枚举（mealStatus、onboardingStatus 等）与这些码各自演化，
// 映射只发生在 handler 层——不要把枚举值直接写上 wire。
const (
	codeInvalidRequest        = "invalid_request"
	codeInvalidQuery          = "invalid_query"
	codeInvalidCredentials    = "invalid_credentials"
	codeUnauthorized          = "unauthorized"
	codeRateLimited           = "rate_limited"
	codeAccountUnavailable    = "account_unavailable"
	codeDishUnavailable       = "dish_unavailable"
	codePoolMemberNotFound    = "candidate_pool_member_not_found"
	codeRecipeNotFound        = "recipe_not_found"
	codeDecisionNotFound      = "decision_not_found"
	codePendingRatingNotFound = "pending_rating_not_found"
	codeRatingConflict        = "rating_conflict"
	codePendingRatings        = "pending_ratings"
	codeCandidatePoolEmpty    = "candidate_pool_empty"
	codeRerollBudgetExhausted = "reroll_budget_exhausted"
	codeHandPickLocked        = "hand_pick_locked"
	codeMealNotFound          = "meal_not_found"
	codeEatingRecordNotFound  = "eating_record_not_found"
	codeNIMUnavailable        = "nim_unavailable"
	codeInterviewLimitReached = "interview_limit_reached"
	codeRetryRequired         = "retry_required"
	codeRetryUnavailable      = "retry_unavailable"
	codeInterviewFinished     = "interview_finished"
	codeNotFound              = "not_found"
	codeInternalError         = "internal_error"
)
