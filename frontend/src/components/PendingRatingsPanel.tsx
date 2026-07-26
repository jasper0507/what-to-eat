import { m } from "motion/react";

import type { PendingRating, Rating } from "@/api/types";
import { copy } from "@/lib/copy";
import { formatMealAt, mealAtISO } from "@/lib/format";
import { springSnappy } from "@/lib/motion";

import { RatingPicker } from "./RatingPicker";

// 评分门禁（ADR 0018）：h2 标题是本态唯一的 h2；卡内菜名用 h3
//（规格按 heading name 匹配这两级）。Meal 时间用 <time> + tabular-nums。
export function PendingRatingsPanel({
  pendingRatings,
  onRate,
  busy,
}: {
  pendingRatings: PendingRating[];
  onRate: (pendingRatingId: number, rating: Rating) => void;
  busy?: { pendingRatingId: number; rating: Rating };
}) {
  return (
    <section className="page-stack page-stack-tight">
      <h2 className="section-title">{copy.home.pendingTitle}</h2>
      <p className="page-intro">{copy.home.pendingIntro}</p>
      {pendingRatings.map((pending, index) => (
        <m.div
          key={pending.id}
          className="pending-card"
          initial={{ opacity: 0, y: 12 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ ...springSnappy, delay: Math.min(index * 0.05, 0.3) }}
        >
          <span className="pending-meta num">
            {copy.home.mealAtPrefix}
            <time dateTime={mealAtISO(pending.meal_at)}>
              {formatMealAt(pending.meal_at)}
            </time>
          </span>
          <h3 className="pending-dish">{pending.dish.name}</h3>
          <RatingPicker
            dish={pending.dish}
            busyRating={
              busy?.pendingRatingId === pending.id ? busy.rating : undefined
            }
            disabled={busy !== undefined}
            onRate={(rating) => onRate(pending.id, rating)}
          />
        </m.div>
      ))}
    </section>
  );
}
