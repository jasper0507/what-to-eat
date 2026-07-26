import { m } from "motion/react";
import type { ReactNode } from "react";

import { copy } from "@/lib/copy";
import { springSnappy } from "@/lib/motion";

// 角色配色气泡：助手 = 米色 tint 靠左，你 = 品牌浅橙靠右，说话侧下角 8px。
// 正文是独立段落元素——规格逐字匹配消息文本。
export function ChatBubble({
  role,
  children,
}: {
  role: "user" | "assistant";
  children: ReactNode;
}) {
  return (
    <m.div
      initial={{ opacity: 0, y: 10, scale: 0.98 }}
      animate={{ opacity: 1, y: 0, scale: 1 }}
      transition={springSnappy}
      className={`chat-bubble chat-bubble-${role}`}
    >
      <div className="chat-bubble-role">
        {role === "user"
          ? copy.onboarding.roleUser
          : copy.onboarding.roleAssistant}
      </div>
      <p className="chat-bubble-content">{children}</p>
    </m.div>
  );
}
