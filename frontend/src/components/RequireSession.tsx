import { LoaderCircle } from "lucide-react";
import { Navigate, Outlet } from "react-router-dom";

import { useSession } from "@/api/hooks";
import { Notice } from "@/components/Notice";

// 会话布局路由：加载 → 全应用唯一的全屏等待；匿名/过期 → 声明式跳 /login。
// 全局 401 漏斗把 session 缓存置 null 后，也是由这里完成跳转。
export default function RequireSession() {
  const session = useSession();

  if (session.isPending) {
    return (
      <main
        role="status"
        aria-label="正在恢复登录状态"
        className="flex min-h-dvh items-center justify-center bg-background"
      >
        <LoaderCircle
          aria-hidden="true"
          className="size-5 animate-spin text-muted-foreground"
        />
      </main>
    );
  }
  if (session.isError) {
    return (
      <main className="flex min-h-dvh items-center justify-center bg-background px-6 font-sans text-base text-foreground antialiased">
        <Notice onRetry={() => void session.refetch()}>
          {session.error.message}
        </Notice>
      </main>
    );
  }
  if (session.data === null) {
    return <Navigate to="/login" replace />;
  }
  return <Outlet />;
}
