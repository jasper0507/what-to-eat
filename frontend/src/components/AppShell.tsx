import { useEffect, useRef } from "react";
import { Link, Outlet, useLocation } from "react-router-dom";

import { useSession } from "@/api/hooks";
import { copy, greeting } from "@/lib/copy";

import { AmbientBackdrop } from "./AmbientBackdrop";

// 全应用唯一的 <main> 在这里（规格数 main .ant-btn-primary）。
// 页头只放链接与问候——永远不出现按钮，更不允许 primary。
export default function AppShell() {
  const account = useSession().data;
  const location = useLocation();
  const firstRender = useRef(true);

  // 路由切换后把焦点移到主内容（跳过首帧，避免抢初始焦点）
  useEffect(() => {
    if (firstRender.current) {
      firstRender.current = false;
      return;
    }
    document.getElementById("main")?.focus();
  }, [location.pathname]);

  return (
    <>
      <a className="skip-link" href="#main">
        {copy.common.skipToMain}
      </a>
      <AmbientBackdrop />
      <header className="app-header">
        <div className="app-header-inner">
          <Link to="/" className="app-logo">
            {copy.common.appName}
            <span className="app-logo-mark">？</span>
          </Link>
          {account ? (
            <span className="app-greeting">{greeting(account.username)}</span>
          ) : null}
        </div>
      </header>
      <main id="main" tabIndex={-1} className="app-main">
        <Outlet />
      </main>
    </>
  );
}
