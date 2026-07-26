import { Button } from "antd";

import type { Dish, Rating } from "@/api/types";
import { RATING_OPTIONS } from "@/lib/ratings";

// 五个平级 default 大按钮，点按即提交（ADR 0018「极快，不写小作文」）。
// 按钮内容只有标签本身——规格按 exact 可访问名逐字匹配（拉完了/NPC/人上人/顶级/夯）。
// 此屏刻意零 primary：五个选择没有推荐项。
export function RatingPicker({
  dish,
  onRate,
  busyRating,
  disabled,
}: {
  dish: Dish;
  onRate: (rating: Rating) => void;
  busyRating?: Rating;
  disabled?: boolean;
}) {
  return (
    <div
      className="rating-picker"
      role="group"
      aria-label={`${dish.name} Taste rating`}
    >
      {RATING_OPTIONS.map((option) => (
        <Button
          key={option.rating}
          size="large"
          loading={busyRating === option.rating}
          disabled={disabled && busyRating !== option.rating}
          onClick={() => onRate(option.rating)}
        >
          {option.label}
        </Button>
      ))}
    </div>
  );
}
