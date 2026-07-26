export const sessionKey = ["session"] as const;
export const onboardingKey = ["onboarding"] as const;
export const mealKey = ["meal"] as const;
export const poolKey = ["pool"] as const;
export const historyKey = ["history"] as const;
export const catalogKey = (query: string) => ["catalog", query] as const;
export const recipeKey = (dishId: string) => ["recipe", dishId] as const;
