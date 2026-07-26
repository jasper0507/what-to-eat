import { Button } from "antd";
import { AnimatePresence, m } from "motion/react";

import type { Decision } from "@/api/types";
import { copy } from "@/lib/copy";
import { spring } from "@/lib/motion";

import { StatusTag } from "./StatusTag";

// 决策舞台——全应用唯一的视觉高潮。巨号衬线菜名是屏上唯一的 h2（规格以
// heading level 2 取菜名）；AnimatePresence mode="wait" 保证换字期间 DOM 里
// 也只有一个 h2。DOM 序 [Reroll, Accept] + grid 翻转同时满足移动端堆叠
// （Reroll 在上）与桌面同排（Reroll 在左）两条断言；Accept 是唯一 primary。
export function DecisionPanel({
  decision,
  onReroll,
  onAccept,
  rerolling,
  accepting,
}: {
  decision: Decision;
  onReroll: () => void;
  onAccept: () => void;
  rerolling: boolean;
  accepting: boolean;
}) {
  return (
    <section className="decision-stage">
      <span className="decision-eyebrow" aria-hidden="true">
        {copy.home.decisionEyebrow}
      </span>
      <m.div
        className="decision-panel"
        initial={{ opacity: 0, y: 18 }}
        animate={{ opacity: 1, y: 0 }}
        transition={spring}
      >
        <StatusTag mode={decision.mode} />
        {decision.mode === "discovery" && decision.reason ? (
          <p className="decision-reason">{decision.reason}</p>
        ) : null}
        <p className="decision-category">{decision.dish.category}</p>
        <div aria-live="polite" className="decision-dish-live">
          <AnimatePresence mode="wait" initial={false}>
            <m.h2
              key={decision.id}
              className="decision-dish"
              initial={{ opacity: 0, y: 30, scale: 0.98 }}
              animate={{ opacity: 1, y: 0, scale: 1 }}
              exit={{ opacity: 0, y: -22, scale: 0.99 }}
              transition={spring}
            >
              {decision.dish.name}
            </m.h2>
          </AnimatePresence>
        </div>
        <div className="decision-actions">
          <Button
            size="large"
            loading={rerolling}
            disabled={accepting}
            onClick={onReroll}
          >
            {copy.home.reroll}
          </Button>
          <Button
            type="primary"
            size="large"
            loading={accepting}
            disabled={rerolling}
            onClick={onAccept}
          >
            {copy.home.accept}
          </Button>
        </div>
      </m.div>
    </section>
  );
}
