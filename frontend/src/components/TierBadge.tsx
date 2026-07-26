import type { Rating } from "@/api/types";
import { TIER_LABELS } from "@/lib/tiers";
import { cn } from "@/lib/utils";

// 情感刻度徽标：标签先行，无数字。夯用陶色点睛（全应用唯一常驻的品牌色块），
// 顶尖沉稳、人上人安静，下两档只出现在评分回显里。
const TONE: Record<Rating, string> = {
  5: "border-brand/30 bg-brand/10 text-brand-ink",
  4: "border-border bg-secondary text-foreground",
  3: "border-border bg-transparent text-muted-foreground",
  2: "border-border bg-transparent text-muted-foreground",
  1: "border-destructive/25 bg-destructive/5 text-destructive",
};

export function TierBadge({
  tier,
  className,
}: {
  tier: Rating;
  className?: string;
}) {
  return (
    <span
      className={cn(
        "inline-flex h-6 shrink-0 items-center rounded-full border px-2.5 text-sm",
        TONE[tier],
        className,
      )}
    >
      {TIER_LABELS[tier]}
    </span>
  );
}

/** 刻度选择：一排档位丸，选中者落成炭墨。入池场景传上三档，评分场景传五档。 */
export function TierScale<T extends Rating>({
  tiers,
  value,
  onSelect,
  disabled,
  size = "default",
}: {
  tiers: readonly T[];
  value?: Rating;
  onSelect: (tier: T) => void;
  disabled?: boolean;
  size?: "default" | "sm";
}) {
  return (
    <div className="flex flex-wrap gap-1.5">
      {tiers.map((tier) => {
        const selected = value === tier;
        return (
          <button
            key={tier}
            type="button"
            disabled={disabled}
            aria-pressed={selected}
            onClick={() => onSelect(tier)}
            className={cn(
              "cursor-pointer rounded-full border px-3 font-sans outline-none transition-colors duration-150",
              "focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background",
              "disabled:pointer-events-none disabled:opacity-50",
              size === "sm" ? "h-7 text-sm" : "h-8 text-sm",
              selected
                ? "border-primary bg-primary text-primary-foreground"
                : "border-border bg-transparent text-foreground hover:bg-accent",
            )}
          >
            {TIER_LABELS[tier]}
          </button>
        );
      })}
    </div>
  );
}
