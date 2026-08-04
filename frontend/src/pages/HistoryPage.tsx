import { LoaderCircle } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";

import { useCandidatePool, useEatingRecords, useRateRecord } from "@/api/hooks";
import type { EatingRecordEntry, Rating } from "@/api/types";
import { Notice } from "@/components/Notice";
import { TierBadge, TierScale } from "@/components/TierBadge";
import { Button } from "@/components/ui/button";
import { mealAtISO } from "@/lib/format";
import {
  canRateHistoryEntry,
  historyModeNote,
  isRecentFavorite,
} from "@/lib/tastePolicy";
import { RATING_TIERS } from "@/lib/tiers";

// 轻历史：最近吃过 / 最近爱吃。补评分是邀请，不是义务——绝不拦路。
// 一菜一口，只认最近：评价入口只出现在每道菜最近的一条记录上，
// 老记录留白，旧感受永远没有机会倒灌覆盖现役档位。
export default function HistoryPage() {
  const records = useEatingRecords();
  const pool = useCandidatePool();

  return (
    <div className="animate-rise mx-auto max-w-3xl space-y-10">
      <header className="space-y-1">
        <h1 className="font-serif text-2xl font-medium">吃过的</h1>
        <p className="text-sm text-muted-foreground">
          最近的每一顿都记着。想补一句评价，随时。
        </p>
      </header>

      {records.isPending ? (
        <div
          role="status"
          aria-label="正在翻记录"
          className="flex justify-center py-10"
        >
          <LoaderCircle
            aria-hidden="true"
            className="size-5 animate-spin text-muted-foreground"
          />
        </div>
      ) : null}
      {records.isError ? (
        <Notice onRetry={() => void records.refetch()}>
          {records.error.message}
        </Notice>
      ) : null}

      {records.data && records.data.length === 0 ? (
        <div className="space-y-4 rounded-md border border-border px-4 py-8 text-center">
          <p className="text-sm text-muted-foreground">
            还一顿都没吃过<span className="text-brand">？</span>
            回主页，开一顿。
          </p>
        </div>
      ) : null}

      {records.data && records.data.length > 0 ? (
        <>
          <Favorites records={records.data} />
          <section className="space-y-4" aria-label="最近吃过">
            <h2 className="font-serif text-xl font-medium">最近吃过</h2>
            <ul className="divide-y divide-border rounded-md border border-border">
              {markLatest(records.data).map(({ record, latest }) => (
                <HistoryRow
                  key={record.id}
                  record={record}
                  latestOfDish={latest}
                  poolTier={
                    pool.data?.find((dish) => dish.id === record.dish.id)?.tier
                  }
                  poolPending={pool.isPending}
                />
              ))}
            </ul>
          </section>
        </>
      ) : null}
    </div>
  );
}

/** 最近爱吃：近期评到顶尖与夯的菜，安静的一排。没有就整段不出。 */
function Favorites({ records }: { records: EatingRecordEntry[] }) {
  const seen = new Set<string>();
  const loved = records.filter((record) => {
    if (!isRecentFavorite(record.rating) || seen.has(record.dish.id)) {
      return false;
    }
    seen.add(record.dish.id);
    return true;
  });
  if (loved.length === 0) {
    return null;
  }
  return (
    <section className="space-y-3" aria-label="最近爱吃">
      <h2 className="font-serif text-xl font-medium">最近爱吃</h2>
      <ul className="flex flex-wrap gap-2">
        {loved.slice(0, 6).map((record) => (
          <li key={record.dish.id}>
            <Link
              to={`/recipes?dish_id=${encodeURIComponent(record.dish.id)}`}
              className="flex items-center gap-2 rounded-full border border-border bg-card py-1.5 pr-1.5 pl-3.5 outline-none transition-colors duration-150 hover:bg-accent focus-visible:ring-2 focus-visible:ring-ring"
            >
              {record.dish.name}
              <TierBadge tier={record.rating as Rating} />
            </Link>
          </li>
        ))}
      </ul>
    </section>
  );
}

const dateFormatter = new Intl.DateTimeFormat("zh-CN", {
  month: "long",
  day: "numeric",
  hour: "2-digit",
  minute: "2-digit",
  hour12: false,
});

/** 标出每道菜最近的一条记录（列表本身最新在前，首见即最近）。 */
function markLatest(records: EatingRecordEntry[]) {
  const seen = new Set<string>();
  return records.map((record) => {
    const latest = !seen.has(record.dish.id);
    seen.add(record.dish.id);
    return { record, latest };
  });
}

function HistoryRow({
  record,
  latestOfDish,
  poolTier,
  poolPending,
}: {
  record: EatingRecordEntry;
  latestOfDish: boolean;
  poolTier?: Rating;
  poolPending: boolean;
}) {
  const [rating, setRating] = useState(false);
  const rate = useRateRecord();
  const modeNote = historyModeNote(record.mode);
  const canRate = canRateHistoryEntry({
    rating: record.rating,
    latestOfDish,
    poolPending,
  });

  return (
    <li className="space-y-3 px-4 py-3">
      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1.5">
        <div className="flex min-w-0 items-center gap-2.5">
          <Link
            to={`/recipes?dish_id=${encodeURIComponent(record.dish.id)}`}
            className="truncate rounded-sm outline-none hover:text-brand-ink focus-visible:ring-2 focus-visible:ring-ring"
          >
            {record.dish.name}
          </Link>
          {modeNote ? (
            <span className="shrink-0 text-sm text-muted-foreground">
              {modeNote}
            </span>
          ) : null}
        </div>
        <div className="flex shrink-0 items-center gap-2.5">
          <time
            dateTime={mealAtISO(record.accepted_at)}
            className="text-sm text-muted-foreground"
          >
            {dateFormatter.format(new Date(record.accepted_at * 1000))}
          </time>
          {record.rating ? <TierBadge tier={record.rating} /> : null}
          {canRate && poolTier ? (
            // 已在池的菜不催评：静默亮出现役档位（虚线描边区分「现状」
            // 与实心的「这顿的评价」），点开可改，下两档仍走剔池
            <button
              type="button"
              aria-label={`改 ${record.dish.name} 的评价`}
              aria-expanded={rating}
              disabled={rate.isPending}
              onClick={() => setRating((open) => !open)}
              className="cursor-pointer rounded-full outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:opacity-50"
            >
              <TierBadge
                tier={poolTier}
                className="border-dashed bg-transparent"
              />
            </button>
          ) : null}
          {canRate && !poolTier ? (
            <Button
              variant="ghost"
              size="sm"
              className="text-muted-foreground"
              aria-expanded={rating}
              onClick={() => setRating((open) => !open)}
            >
              补一句评价
            </Button>
          ) : null}
        </div>
      </div>
      {rating && canRate ? (
        <div className="flex items-center gap-3">
          <TierScale
            tiers={RATING_TIERS}
            value={poolTier}
            size="sm"
            disabled={rate.isPending}
            onSelect={(value) =>
              rate.mutate(
                { recordId: record.id, rating: value },
                { onSuccess: () => setRating(false) },
              )
            }
          />
          {rate.isPending ? (
            <LoaderCircle
              aria-hidden="true"
              className="size-4 animate-spin text-muted-foreground"
            />
          ) : null}
        </div>
      ) : null}
      {rate.error ? <Notice>{rate.error.message}</Notice> : null}
    </li>
  );
}
