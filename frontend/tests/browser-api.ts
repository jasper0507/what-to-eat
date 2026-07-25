import type { Page } from "@playwright/test";

export function addCandidatePoolDish(
  page: Page,
  dishID: string,
  preferenceWeight: number,
) {
  return page.evaluate(
    async (dish) =>
      (
        await fetch("/api/candidate-pool/dishes", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(dish),
        })
      ).ok,
    { dish_id: dishID, preference_weight: preferenceWeight },
  );
}
