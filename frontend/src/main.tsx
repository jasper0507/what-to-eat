import { QueryClientProvider } from "@tanstack/react-query";
import { App as AntApp, ConfigProvider } from "antd";
import zhCN from "antd/locale/zh_CN";
import { LazyMotion, MotionConfig } from "motion/react";
import React from "react";
import ReactDOM from "react-dom/client";
import { RouterProvider } from "react-router-dom";
import "@fontsource-variable/noto-serif-sc";
import "@fontsource-variable/noto-sans-sc";
import "./styles/antd-reset.css";
import "./styles.css";
import "./styles/globals.css";

import { queryClient } from "@/api/queryClient";
import { router } from "@/router";
import { appTheme } from "@/theme";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    {/* autoInsertSpace 会把两个汉字的按钮渲染成「发 送」，破坏规格的角色名匹配 */}
    <ConfigProvider
      locale={zhCN}
      theme={appTheme}
      button={{ autoInsertSpace: false }}
    >
      <AntApp>
        <QueryClientProvider client={queryClient}>
          {/* strict：只允许 m.* 组件；特性包异步加载，不阻塞首屏 */}
          <LazyMotion
            features={() =>
              import("@/lib/motionFeatures").then((mod) => mod.default)
            }
            strict
          >
            <MotionConfig reducedMotion="user">
              <RouterProvider router={router} />
            </MotionConfig>
          </LazyMotion>
        </QueryClientProvider>
      </AntApp>
    </ConfigProvider>
  </React.StrictMode>,
);
