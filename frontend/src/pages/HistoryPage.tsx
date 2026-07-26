import { LoaderCircle } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";

import { useEatingRecords, useRateRecord } from "@/api/hooks";
import type { EatingRecordEntry, Rating } from "@/api/types";
import { Notice } from "@/components/Notice";
import { TierBadge, TierScale } from "@/components/TierBadge";
import { Button } from "@/components/ui/button";
import { mealAtISO } from "@/lib/format";
import { RATING_TIERS } from "@/lib/tiers";

// 轻历史：最近吃过 / 最近爱吃。补评分是邀请，不是义务——绝不拦路。
export default function HistoryPage() {
  const records = useEatingRecords();

  return (
    <div className="animate-rise space-y-10">
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
        <Notice tone="error" onRetry={() => void records.refetch()}>
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
              {records.data.map((record) => (
                <HistoryRow key={record.id} record={record} />
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
    if (!record.rating || record.rating < 4 || seen.has(record.dish.id)) {
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

const MODE_NOTES: Record<EatingRecordEntry["mode"], string | null> = {
  pool: null,
  discovery: "新尝试",
  hand_pick: "亲自点的",
};

function HistoryRow({ record }: { record: EatingRecordEntry }) {
  const [rating, setRating] = useState(false);
  const rate = useRateRecord();
  const modeNote = MODE_NOTES[record.mode];

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
          {record.rating ? (
            <TierBadge tier={record.rating} />
          ) : (
            <Button
              variant="ghost"
              size="sm"
              className="text-muted-foreground"
              aria-expanded={rating}
              onClick={() => setRating((open) => !open)}
            >
              补一句评价
            </Button>
          )}
        </div>
      </div>
      {rating && !record.rating ? (
        <div className="flex items-center gap-3">
          <TierScale
            tiers={RATING_TIERS}
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
      {rate.error ? <Notice tone="error">{rate.error.message}</Notice> : null}
    </li>
  );
}
