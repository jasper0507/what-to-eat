import type { ReactNode } from "react";

// 每页恰一个 h1 由这里产出（标题层级政策见 lib/copy.ts 文件头）。
export function PageHeader({
  title,
  intro,
  trailing,
}: {
  title: string;
  intro?: string;
  trailing?: ReactNode;
}) {
  return (
    <div className="page-stack page-stack-tight">
      <div className="page-header-row">
        <h1 className="page-title">{title}</h1>
        {trailing}
      </div>
      {intro ? <p className="page-intro">{intro}</p> : null}
    </div>
  );
}
