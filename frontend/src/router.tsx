import type { ComponentType } from "react";
import { createBrowserRouter } from "react-router-dom";

import AppShell from "@/components/AppShell";
import RequireSession from "@/components/RequireSession";
import NotFoundPage from "@/pages/NotFoundPage";

function lazyPage(loader: () => Promise<{ default: ComponentType }>) {
  return async () => ({ Component: (await loader()).default });
}

export const router = createBrowserRouter([
  { path: "/login", lazy: lazyPage(() => import("@/pages/LoginPage")) },
  {
    element: <RequireSession />,
    children: [
      {
        element: <AppShell />,
        children: [
          { path: "/", lazy: lazyPage(() => import("@/pages/HomePage")) },
          {
            path: "/onboarding",
            lazy: lazyPage(() => import("@/pages/OnboardingPage")),
          },
          {
            path: "/candidate-pool",
            lazy: lazyPage(() => import("@/pages/CandidatePoolPage")),
          },
          {
            path: "/recipes",
            lazy: lazyPage(() => import("@/pages/RecipePage")),
          },
          // 真 404（登录后）；未登录的乱路径由 RequireSession 先送去 /login
          { path: "*", element: <NotFoundPage /> },
        ],
      },
    ],
  },
]);
