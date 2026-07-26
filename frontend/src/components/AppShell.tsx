import { History, LogOut, Soup } from "lucide-react";
import { useEffect, useRef } from "react";
import { Link, NavLink, Outlet, useLocation } from "react-router-dom";

import { useLogout, useSession } from "@/api/hooks";
import { cn } from "@/lib/utils";

// 驾驶舱页头：wordmark、加菜与轻历史入口、账号与登出。安静、发丝线分层，
// 唯一的品牌色是签名陶问号。舞台的戏都在页面里，页头永不抢戏。
export default function AppShell() {
  const account = useSession().data;
  const logout = useLogout();
  const location = useLocation();
  const firstRender = useRef(true);

  // 路由切换后把焦点移到主内容（跳过首帧，避免抢初始焦点）。
  // preventScroll + 显式回顶：否则聚焦会把页头滚出视口。
  useEffect(() => {
    if (firstRender.current) {
      firstRender.current = false;
      return;
    }
    document.getElementById("main")?.focus({ preventScroll: true });
    window.scrollTo(0, 0);
  }, [location.pathname]);

  const navItem = ({ isActive }: { isActive: boolean }) =>
    cn(
      "flex h-8 items-center gap-1.5 rounded-md px-2.5 text-sm outline-none transition-colors duration-150",
      "hover:bg-accent focus-visible:ring-2 focus-visible:ring-ring",
      isActive ? "text-foreground" : "text-muted-foreground",
    );

  return (
    <div className="min-h-dvh bg-background font-sans text-base text-foreground antialiased">
      <a
        href="#main"
        className="sr-only focus-visible:not-sr-only focus-visible:absolute focus-visible:top-2 focus-visible:left-2 focus-visible:z-50 focus-visible:rounded-md focus-visible:bg-card focus-visible:px-3 focus-visible:py-2 focus-visible:ring-2 focus-visible:ring-ring"
      >
        跳到主要内容
      </a>
      <header className="border-b border-border">
        <div className="mx-auto flex h-14 w-full max-w-3xl items-center justify-between gap-3 px-4 sm:px-6">
          <Link
            to="/"
            className="rounded-md font-serif text-lg font-medium tracking-wide outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            今天吃什么<span className="text-brand">？</span>
          </Link>
          <nav className="flex items-center gap-0.5" aria-label="主导航">
            <NavLink to="/candidate-pool" className={navItem}>
              <Soup aria-hidden="true" className="size-4" />
              池子
            </NavLink>
            <NavLink to="/history" className={navItem}>
              <History aria-hidden="true" className="size-4" />
              吃过的
            </NavLink>
            <span
              aria-hidden="true"
              className="mx-2 hidden h-4 w-px bg-border sm:block"
            />
            {account ? (
              <span className="hidden max-w-32 truncate text-sm text-muted-foreground sm:block">
                {account.username}
              </span>
            ) : null}
            <button
              type="button"
              aria-label="登出"
              title="登出"
              disabled={logout.isPending}
              onClick={() => logout.mutate()}
              className="ml-1 flex size-8 cursor-pointer items-center justify-center rounded-md text-muted-foreground outline-none transition-colors duration-150 hover:bg-accent hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50"
            >
              <LogOut aria-hidden="true" className="size-4" />
            </button>
          </nav>
        </div>
      </header>
      <main
        id="main"
        tabIndex={-1}
        className="mx-auto w-full max-w-3xl px-4 pt-8 pb-16 outline-none sm:px-6"
      >
        <Outlet />
      </main>
    </div>
  );
}
