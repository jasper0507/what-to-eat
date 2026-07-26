import { LoaderCircle } from "lucide-react";
import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";

import { useRecipe } from "@/api/hooks";
import type { Dish } from "@/api/types";
import { Notice } from "@/components/Notice";
import { buttonVariants } from "@/components/ui/button";
import { cn } from "@/lib/utils";

// 菜谱页：全应用唯一放图片的地方（图为信息，不为装饰）。图源画质参差，
// 一律进统一容器（固定比例 + object-cover + 发丝边框）驯化。
export default function RecipePage() {
  const [params] = useSearchParams();
  const dishId = params.get("dish_id") ?? "";
  const recipe = useRecipe(dishId);

  return (
    <div className="animate-rise mx-auto max-w-3xl space-y-8">
      {dishId === "" ? (
        <Notice tone="error">地址少了菜名，回主页重新开一顿吧。</Notice>
      ) : null}
      {recipe.isPending && dishId !== "" ? (
        <div
          role="status"
          aria-label="正在翻菜谱"
          className="flex justify-center py-16"
        >
          <LoaderCircle
            aria-hidden="true"
            className="size-5 animate-spin text-muted-foreground"
          />
        </div>
      ) : null}
      {recipe.isError ? (
        <Notice tone="error" onRetry={() => void recipe.refetch()}>
          {recipe.error.message}
        </Notice>
      ) : null}

      {recipe.data ? (
        <article className="space-y-6">
          <header className="space-y-2">
            <h1 className="font-serif text-3xl leading-tight font-medium tracking-wide">
              {recipe.data.dish.name}
            </h1>
            <p className="text-sm text-muted-foreground">
              {recipeMeta(recipe.data.dish)}
            </p>
          </header>

          {recipe.data.images.length > 0 ? (
            <RecipeImage
              reference={recipe.data.images[0]}
              alt={`${recipe.data.dish.name} 成品图`}
              className="rounded-lg"
              imageClassName="aspect-video"
            />
          ) : null}

          <RecipeBody content={recipe.data.content} />

          {recipe.data.images.length > 1 ? (
            <section aria-label="步骤图" className="space-y-2">
              <h2 className="font-serif text-xl font-medium">步骤图</h2>
              <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
                {recipe.data.images.slice(1).map((image) => (
                  <RecipeImage
                    key={image}
                    reference={image}
                    alt={`${recipe.data.dish.name} 步骤图`}
                    lazy
                    className="rounded-md"
                    imageClassName="aspect-square"
                  />
                ))}
              </div>
            </section>
          ) : null}
        </article>
      ) : null}

      <div className="border-t border-border pt-6">
        <Link to="/" className={cn(buttonVariants({ size: "lg" }), "w-full")}>
          开始下一顿
        </Link>
      </div>
    </div>
  );
}

function imageSource(reference: string): string {
  if (reference.startsWith("http://") || reference.startsWith("https://")) {
    return reference;
  }
  return `/api/catalog/assets/${encodeURI(reference)}`;
}

/**
 * 统一容器 + 坏图自弃：语料里存在坏图源（如未取回的 Git LFS 指针文件），
 * 解码失败就整个容器退场，页面自然回到纯文字——绝不给用户看碎图标。
 */
function RecipeImage({
  reference,
  alt,
  lazy,
  className,
  imageClassName,
}: {
  reference: string;
  alt: string;
  lazy?: boolean;
  className: string;
  imageClassName: string;
}) {
  const [broken, setBroken] = useState(false);
  if (broken) {
    return null;
  }
  return (
    <figure
      className={cn("overflow-hidden border border-border bg-muted", className)}
    >
      <img
        src={imageSource(reference)}
        alt={alt}
        loading={lazy ? "lazy" : undefined}
        onError={() => setBroken(true)}
        className={cn("w-full object-cover", imageClassName)}
      />
    </figure>
  );
}

function recipeMeta(dish: Dish): string {
  const parts = [dish.category];
  if (dish.cook_minutes) {
    parts.push(`约 ${dish.cook_minutes} 分钟`);
  }
  if (dish.difficulty) {
    const numeral = ["一", "二", "三", "四", "五"][dish.difficulty - 1];
    if (numeral) {
      parts.push(`难度${numeral}星`);
    }
  }
  if (dish.calories) {
    parts.push(`约 ${dish.calories} 大卡`);
  }
  return parts.join(" · ");
}

// ---- 极小的确定性 Markdown 渲染（不引依赖）----
// 只认这份语料真实出现的形态：标题、列表（含缩进）、段落。
// 首个 h1（菜名）与图片行剔除；「预估难度/卡路里」行剔除（页头小字已表达）。

interface ListItem {
  indent: number;
  text: string;
}

type Block =
  | { type: "heading"; level: 3 | 4; text: string }
  | { type: "paragraph"; text: string }
  | { type: "list"; items: ListItem[] };

function stripInline(text: string): string {
  return text
    .replace(/!\[[^\]]*\]\([^)]*\)/g, "")
    .replace(/\[([^\]]+)\]\([^)]*\)/g, "$1")
    .replace(/\*\*([^*]+)\*\*/g, "$1")
    .replace(/`([^`]+)`/g, "$1")
    .trim();
}

function parseBlocks(content: string): Block[] {
  const blocks: Block[] = [];
  for (const rawLine of content.split("\n")) {
    const line = rawLine.replace(/\s+$/, "");
    const trimmed = line.trim();
    if (
      trimmed === "" ||
      trimmed.startsWith("![") ||
      /^#\s/.test(trimmed) ||
      trimmed.startsWith("预估烹饪难度") ||
      trimmed.startsWith("预估卡路里")
    ) {
      continue;
    }
    const heading = /^(#{2,6})\s+(.*)$/.exec(trimmed);
    if (heading) {
      blocks.push({
        type: "heading",
        level: heading[1].length === 2 ? 3 : 4,
        text: stripInline(heading[2]),
      });
      continue;
    }
    const listMatch = /^(\s*)(?:[-*]|\d+\.)\s+(.*)$/.exec(line);
    if (listMatch) {
      const item: ListItem = {
        indent: Math.min(Math.floor(listMatch[1].length / 2), 3),
        text: stripInline(listMatch[2]),
      };
      const last = blocks[blocks.length - 1];
      if (last?.type === "list") {
        last.items.push(item);
      } else {
        blocks.push({ type: "list", items: [item] });
      }
      continue;
    }
    const text = stripInline(trimmed);
    if (text !== "") {
      blocks.push({ type: "paragraph", text });
    }
  }
  return blocks;
}

const LIST_INDENT = ["ml-0", "ml-5", "ml-10", "ml-14"];

function RecipeBody({ content }: { content: string }) {
  return (
    <div className="space-y-4">
      {parseBlocks(content).map((block, index) => {
        if (block.type === "heading") {
          return block.level === 3 ? (
            <h2 key={index} className="pt-2 font-serif text-xl font-medium">
              {block.text}
            </h2>
          ) : (
            <h3 key={index} className="font-serif text-base font-medium">
              {block.text}
            </h3>
          );
        }
        if (block.type === "list") {
          return (
            <ul key={index} className="space-y-1.5">
              {block.items.map((item, itemIndex) => (
                <li
                  key={itemIndex}
                  className={cn(
                    "relative pl-4 before:absolute before:left-0 before:text-muted-foreground before:content-['·']",
                    LIST_INDENT[item.indent],
                  )}
                >
                  {item.text}
                </li>
              ))}
            </ul>
          );
        }
        return (
          <p key={index} className="leading-relaxed text-foreground/90">
            {block.text}
          </p>
        );
      })}
    </div>
  );
}
