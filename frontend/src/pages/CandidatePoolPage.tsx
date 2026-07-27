import { LoaderCircle, Search, X } from "lucide-react";
import { useState, type FormEvent } from "react";

import {
  useAddPoolDish,
  useCandidatePool,
  useCatalogSearch,
  useRemovePoolDish,
  useUpdatePoolWeight,
} from "@/api/hooks";
import type { Dish, PoolDish } from "@/api/types";
import { Notice } from "@/components/Notice";
import { TierBadge, TierScale } from "@/components/TierBadge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { DEFAULT_POOL_TIER, POOL_TIERS, TIER_LABELS } from "@/lib/tiers";

// 池子页：这里的菜才会被揭示选中。档位只说人话（情感刻度徽标，点开切换，
// 只见上三档）；数字权重是被处决的旧世界。
export default function CandidatePoolPage() {
  const pool = useCandidatePool();

  return (
    <div className="animate-rise space-y-10 lg:grid lg:grid-cols-[minmax(0,1fr)_minmax(0,24rem)] lg:items-start lg:gap-14 lg:space-y-0">
      <section className="space-y-4">
        <header className="space-y-1">
          <h1 className="font-serif text-2xl font-medium">池子</h1>
          <p className="text-sm text-muted-foreground">
            {pool.data && pool.data.length > 0
              ? `${pool.data.length} 道菜备着。点档位徽标可以改喜欢的程度。`
              : "这一顿只会从池子里挑。"}
          </p>
        </header>

        {pool.isPending ? (
          <div
            role="status"
            aria-label="正在翻池子"
            className="flex justify-center py-10"
          >
            <LoaderCircle
              aria-hidden="true"
              className="size-5 animate-spin text-muted-foreground"
            />
          </div>
        ) : null}
        {pool.isError ? (
          <Notice onRetry={() => void pool.refetch()}>
            {pool.error.message}
          </Notice>
        ) : null}
        {pool.data && pool.data.length === 0 ? (
          <div className="rounded-md border border-border px-4 py-6 text-center text-sm text-muted-foreground">
            池子还空着<span className="text-brand">？</span>
            在下面搜你爱吃的，一道道放进来。
          </div>
        ) : null}
        {pool.data && pool.data.length > 0 ? (
          <ul className="divide-y divide-border rounded-md border border-border">
            {pool.data.map((dish) => (
              <PoolRow key={dish.id} dish={dish} />
            ))}
          </ul>
        ) : null}
      </section>

      <AddSection poolIds={new Set((pool.data ?? []).map((dish) => dish.id))} />
    </div>
  );
}

function PoolRow({ dish }: { dish: PoolDish }) {
  const [editing, setEditing] = useState(false);
  const updateWeight = useUpdatePoolWeight();
  const removeDish = useRemovePoolDish();
  const busy = updateWeight.isPending || removeDish.isPending;

  return (
    <li className="px-4 py-3">
      <div className="flex items-center justify-between gap-3">
        <div className="min-w-0">
          <p className="truncate">{dish.name}</p>
          <p className="text-sm text-muted-foreground">{dish.category}</p>
        </div>
        <div className="flex shrink-0 items-center gap-1.5">
          <button
            type="button"
            aria-expanded={editing}
            aria-label={`改 ${dish.name} 的档位`}
            disabled={busy}
            onClick={() => setEditing((open) => !open)}
            className="cursor-pointer rounded-full outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:opacity-50"
          >
            <TierBadge tier={dish.tier} />
          </button>
          <button
            type="button"
            aria-label={`把 ${dish.name} 移出池子`}
            disabled={busy}
            onClick={() => removeDish.mutate(dish.id)}
            className="flex size-8 cursor-pointer items-center justify-center rounded-md text-muted-foreground outline-none transition-colors duration-150 hover:bg-accent hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50"
          >
            {removeDish.isPending ? (
              <LoaderCircle
                aria-hidden="true"
                className="size-4 animate-spin"
              />
            ) : (
              <X aria-hidden="true" className="size-4" />
            )}
          </button>
        </div>
      </div>
      {editing ? (
        <div className="mt-2.5 flex items-center gap-3">
          <TierScale
            tiers={POOL_TIERS}
            value={dish.tier}
            size="sm"
            disabled={busy}
            onSelect={(tier) => {
              if (tier === dish.tier) {
                setEditing(false);
                return;
              }
              updateWeight.mutate(
                { dish_id: dish.id, tier },
                { onSuccess: () => setEditing(false) },
              );
            }}
          />
          {updateWeight.isPending ? (
            <LoaderCircle
              aria-hidden="true"
              className="size-4 animate-spin text-muted-foreground"
            />
          ) : null}
        </div>
      ) : null}
      {(updateWeight.error ?? removeDish.error) ? (
        <Notice className="mt-2.5">
          {((updateWeight.error ?? removeDish.error) as Error).message}
        </Notice>
      ) : null}
    </li>
  );
}

function AddSection({ poolIds }: { poolIds: Set<string> }) {
  const [searchInput, setSearchInput] = useState("");
  const [submittedQuery, setSubmittedQuery] = useState("");
  const search = useCatalogSearch(submittedQuery);
  const addDish = useAddPoolDish();

  const handleSubmit = (event: FormEvent) => {
    event.preventDefault();
    setSubmittedQuery(searchInput.trim());
  };

  return (
    <section className="space-y-4 lg:sticky lg:top-8" aria-label="往池子加菜">
      <header className="space-y-1">
        <h2 className="font-serif text-xl font-medium">往池子加菜</h2>
        <p className="text-sm text-muted-foreground">
          搜菜谱库里的名字，入池默认「{TIER_LABELS[DEFAULT_POOL_TIER]}
          」，之后随时改。
        </p>
      </header>

      <form className="flex gap-2" onSubmit={handleSubmit}>
        <Input
          type="search"
          name="q"
          maxLength={100}
          placeholder="比如：番茄"
          aria-label="搜索菜谱库"
          value={searchInput}
          onChange={(event) => setSearchInput(event.target.value)}
        />
        <Button
          type="submit"
          variant="outline"
          aria-busy={search.isFetching}
          disabled={search.isFetching}
        >
          {search.isFetching ? (
            <LoaderCircle aria-hidden="true" className="size-4 animate-spin" />
          ) : (
            <Search aria-hidden="true" className="size-4" />
          )}
          搜索
        </Button>
      </form>

      {addDish.error ? <Notice>{addDish.error.message}</Notice> : null}
      {search.isError ? (
        <Notice onRetry={() => void search.refetch()}>
          {search.error.message}
        </Notice>
      ) : null}
      {search.data && search.data.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          菜谱库里没有这道。换个写法试试，比如只搜两个字。
        </p>
      ) : null}
      {search.data && search.data.length > 0 ? (
        <ul className="divide-y divide-border rounded-md border border-border">
          {search.data.map((dish) => (
            <CatalogRow
              key={dish.id}
              dish={dish}
              inPool={poolIds.has(dish.id)}
              adding={
                addDish.isPending && addDish.variables?.dish_id === dish.id
              }
              otherBusy={
                addDish.isPending && addDish.variables?.dish_id !== dish.id
              }
              onAdd={() =>
                addDish.mutate({ dish_id: dish.id, tier: DEFAULT_POOL_TIER })
              }
            />
          ))}
        </ul>
      ) : null}
    </section>
  );
}

function CatalogRow({
  dish,
  inPool,
  adding,
  otherBusy,
  onAdd,
}: {
  dish: Dish;
  inPool: boolean;
  adding: boolean;
  otherBusy: boolean;
  onAdd: () => void;
}) {
  return (
    <li className="flex items-center justify-between gap-3 px-4 py-3">
      <div className="min-w-0">
        <p className="truncate">{dish.name}</p>
        <p className="text-sm text-muted-foreground">{dish.category}</p>
      </div>
      <Button
        variant="outline"
        size="sm"
        disabled={inPool || adding || otherBusy}
        aria-busy={adding}
        onClick={onAdd}
      >
        {adding ? (
          <LoaderCircle aria-hidden="true" className="size-4 animate-spin" />
        ) : null}
        {inPool ? "已在池里" : "入池"}
      </Button>
    </li>
  );
}
