import { Button, Tag } from "antd";
import { m } from "motion/react";
import { useNavigate, useSearchParams } from "react-router-dom";

import { useRecipe } from "@/api/hooks";
import { ErrorAlert } from "@/components/ErrorAlert";
import { LoadingBlock } from "@/components/LoadingBlock";
import { PageHeader } from "@/components/PageHeader";
import { copy } from "@/lib/copy";
import { pageEnter } from "@/lib/motion";

// 菜名是本页唯一的 h2（规格以 heading level 2 校验接受的 Dish）。
// 正文保持 pre-wrap 纯文本；若未来引入 Markdown 渲染，必须把 md 标题降级
// 到 h3+，否则破坏「至多一个 h2」契约。
export default function RecipePage() {
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const dishId = params.get("dish_id") ?? "";
  const recipe = useRecipe(dishId);

  return (
    <m.div {...pageEnter} className="container page-stack">
      <PageHeader title={copy.recipe.title} />

      {dishId === "" ? (
        <ErrorAlert error={new Error(copy.recipe.missing)} />
      ) : null}
      {recipe.isPending && dishId !== "" ? (
        <LoadingBlock preset="recipe" label={copy.recipe.loadingLabel} />
      ) : null}
      {recipe.isError ? <ErrorAlert error={recipe.error} /> : null}

      {recipe.data ? (
        <div className="page-stack page-stack-tight">
          <div>
            <Tag>{recipe.data.dish.category}</Tag>
          </div>
          <h2 className="recipe-dish">{recipe.data.dish.name}</h2>
          <span className="dish-path">{recipe.data.dish.recipe_path}</span>
          <pre className="recipe-body">{recipe.data.content}</pre>
        </div>
      ) : null}

      <div>
        <Button
          block
          type="primary"
          size="large"
          href="/"
          onClick={(event) => {
            event.preventDefault();
            void navigate("/");
          }}
        >
          {copy.recipe.next}
        </Button>
      </div>
    </m.div>
  );
}
