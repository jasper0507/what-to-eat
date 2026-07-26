import { Spin } from "antd";
import { LoaderCircle } from "lucide-react";
import { Navigate, Outlet } from "react-router-dom";

import { useSession } from "@/api/hooks";
import { copy } from "@/lib/copy";

import { ErrorAlert } from "./ErrorAlert";

// 会话布局路由：加载 → 全应用唯一的 Spin；匿名/过期 → 声明式跳 /login。
// 全局 401 漏斗把 session 缓存置 null 后，也是由这里完成跳转。
export default function RequireSession() {
  const session = useSession();

  if (session.isPending) {
    return (
      <main
        className="centered-state"
        aria-label={copy.common.sessionLoadingLabel}
      >
        <Spin
          indicator={
            <LoaderCircle className="w2e-spin" size={40} strokeWidth={1.8} />
          }
        />
      </main>
    );
  }
  if (session.isError) {
    return (
      <main className="centered-state">
        <ErrorAlert
          error={session.error}
          onRetry={() => void session.refetch()}
        />
      </main>
    );
  }
  if (session.data === null) {
    return <Navigate to="/login" replace />;
  }
  return <Outlet />;
}
