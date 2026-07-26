import { App as AntApp, Button, Empty, Input, List } from "antd";
import { m } from "motion/react";
import { useState } from "react";
import { Link } from "react-router-dom";

import {
  useAddPoolDish,
  useCandidatePool,
  useCatalogSearch,
  useRemovePoolDish,
  useUpdatePoolWeight,
} from "@/api/hooks";
import type { Dish, PoolDish } from "@/api/types";
import { DishLine } from "@/components/DishLine";
import { ErrorAlert } from "@/components/ErrorAlert";
import { LoadingBlock } from "@/components/LoadingBlock";
import { PageHeader } from "@/components/PageHeader";
import { WeightControl } from "@/components/WeightControl";
import { copy } from "@/lib/copy";
import { pageEnter, springSnappy } from "@/lib/motion";
import { DEFAULT_ADD_WEIGHT } from "@/lib/ratings";

const rowEnter = (index: number) => ({
  initial: { opacity: 0, y: 12 },
  animate: { opacity: 1, y: 0 },
  transition: { ...springSnappy, delay: Math.min(index * 0.04, 0.3) },
});

export default function CandidatePoolPage() {
  const { message } = AntApp.useApp();
  const pool = useCandidatePool();
  const updateWeight = useUpdatePoolWeight();
  const removeDish = useRemovePoolDish();
  const addDish = useAddPoolDish();

  const [searchInput, setSearchInput] = useState("");
  const [submittedQuery, setSubmittedQuery] = useState("");
  const search = useCatalogSearch(submittedQuery);

  const poolIds = new Set((pool.data ?? []).map((dish) => dish.id));
  const poolBusy = updateWeight.isPending || removeDish.isPending;

  const renderPoolRow = (dish: PoolDish, index: number) => (
    <List.Item key={dish.id}>
      <m.div {...rowEnter(index)} className="dish-row">
        <DishLine dish={dish} />
        <WeightControl
          dish={dish}
          disabled={poolBusy}
          onCommit={(value) =>
            updateWeight.mutate({ dish_id: dish.id, preference_weight: value })
          }
        />
        <div className="dish-row-actions">
          <Button
            danger
            loading={removeDish.isPending && removeDish.variables === dish.id}
            disabled={poolBusy && removeDish.variables !== dish.id}
            onClick={() =>
              removeDish.mutate(dish.id, {
                onSuccess: () => {
                  void message.success(copy.pool.removedToast);
                },
              })
            }
          >
            {copy.pool.remove}
          </Button>
        </div>
      </m.div>
    </List.Item>
  );

  const renderCatalogRow = (dish: Dish, index: number) => {
    const inPool = poolIds.has(dish.id);
    const adding = addDish.isPending && addDish.variables?.dish_id === dish.id;
    return (
      <List.Item key={dish.id}>
        <m.div {...rowEnter(index)} className="dish-row dish-row-catalog">
          <DishLine dish={dish} />
          <div className="dish-row-actions">
            <Button
              loading={adding}
              disabled={inPool || (addDish.isPending && !adding)}
              onClick={() =>
                addDish.mutate(
                  { dish_id: dish.id, preference_weight: DEFAULT_ADD_WEIGHT },
                  {
                    onSuccess: () => {
                      void message.success(copy.pool.addedToast);
                    },
                  },
                )
              }
            >
              {inPool ? copy.pool.added : copy.pool.add}
            </Button>
          </div>
        </m.div>
      </List.Item>
    );
  };

  return (
    <m.div {...pageEnter} className="container container-lg page-stack">
      <PageHeader
        title={copy.pool.title}
        intro={copy.pool.intro}
        trailing={<Link to="/">{copy.pool.back}</Link>}
      />

      <section
        className="page-stack page-stack-tight"
        aria-label={copy.pool.title}
      >
        <ErrorAlert error={updateWeight.error ?? removeDish.error} />
        {pool.isPending ? (
          <LoadingBlock preset="list" label={copy.pool.loadingLabel} />
        ) : null}
        {pool.isError ? (
          <ErrorAlert error={pool.error} onRetry={() => void pool.refetch()} />
        ) : null}
        {pool.data && pool.data.length === 0 ? (
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            description={copy.pool.empty}
          />
        ) : null}
        {pool.data && pool.data.length > 0 ? (
          <List dataSource={pool.data} renderItem={renderPoolRow} />
        ) : null}
      </section>

      <section
        className="page-stack page-stack-tight"
        aria-label={copy.pool.addTitle}
      >
        <h2 className="section-title">{copy.pool.addTitle}</h2>
        <p className="page-intro">{copy.pool.addIntro}</p>
        <Input.Search
          size="large"
          allowClear
          maxLength={100}
          placeholder={copy.pool.searchPlaceholder}
          enterButton={copy.pool.searchButton}
          value={searchInput}
          onChange={(event) => setSearchInput(event.target.value)}
          onSearch={(value) => setSubmittedQuery(value.trim())}
          loading={search.isFetching}
        />
        <ErrorAlert error={addDish.error} />
        {search.isError ? (
          <ErrorAlert
            error={search.error}
            onRetry={() => void search.refetch()}
          />
        ) : null}
        {submittedQuery === "" ? (
          <p className="catalog-hint">{copy.pool.searchHint}</p>
        ) : null}
        {search.data && search.data.length === 0 ? (
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            description={copy.pool.noResults}
          />
        ) : null}
        {search.data && search.data.length > 0 ? (
          <List dataSource={search.data} renderItem={renderCatalogRow} />
        ) : null}
      </section>
    </m.div>
  );
}
