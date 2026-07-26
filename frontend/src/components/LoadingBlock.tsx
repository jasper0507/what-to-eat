import { Skeleton } from "antd";

// 内容区加载一律用形状贴合的 Skeleton（全应用唯一 Spin 在 RequireSession）。
export function LoadingBlock({
  preset,
  label,
}: {
  preset: "home" | "list" | "chat" | "recipe";
  label: string;
}) {
  return (
    <div className="loading-block" role="status" aria-label={label}>
      {preset === "home" ? (
        <>
          <Skeleton active title paragraph={{ rows: 2 }} />
          <Skeleton.Button active block size="large" />
        </>
      ) : null}
      {preset === "list" ? (
        <>
          <Skeleton active title={false} paragraph={{ rows: 2 }} />
          <Skeleton active title={false} paragraph={{ rows: 2 }} />
          <Skeleton active title={false} paragraph={{ rows: 2 }} />
        </>
      ) : null}
      {preset === "chat" ? (
        <>
          <Skeleton.Node active className="chat-skeleton chat-skeleton-left" />
          <Skeleton.Node active className="chat-skeleton chat-skeleton-right" />
        </>
      ) : null}
      {preset === "recipe" ? (
        <Skeleton active title paragraph={{ rows: 8 }} />
      ) : null}
    </div>
  );
}
