import type { ReactNode } from "react";

import { AmbientBackdrop } from "./AmbientBackdrop";

// 壳外路由（登录页）的 <main> 包装：无页头，同一氛围层。
export function PageShell({
  width = "sm",
  children,
}: {
  width?: "sm" | "md" | "lg";
  children: ReactNode;
}) {
  return (
    <>
      <AmbientBackdrop />
      <main id="main" tabIndex={-1} className="app-main centered-page">
        <div className={`container container-${width}`}>{children}</div>
      </main>
    </>
  );
}
