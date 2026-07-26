import { Tag } from "antd";

import type { Dish } from "@/api/types";

// 菜行主体：菜名 + 分类 Tag + 标签 chips + 来源路径次要行。
// 池列表与 Catalog 结果共用；recipe_path 仅在此展示（身份一律用 dish.id）。
export function DishLine({ dish }: { dish: Dish }) {
  return (
    <div className="dish-row-main">
      <div className="dish-line">
        <strong className="dish-name">{dish.name}</strong>
        <Tag>{dish.category}</Tag>
        {dish.tags.map((tag) => (
          <Tag key={tag}>{tag}</Tag>
        ))}
      </div>
      <span className="dish-path">{dish.recipe_path}</span>
    </div>
  );
}
