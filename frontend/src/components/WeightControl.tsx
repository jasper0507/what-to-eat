import { Slider } from "antd";
import { useEffect, useRef, useState } from "react";

import type { PoolDish } from "@/api/types";
import { copy } from "@/lib/copy";
import { formatWeight } from "@/lib/format";
import { WEIGHT_MAX, WEIGHT_MIN, WEIGHT_STEP } from "@/lib/ratings";

// 读数是规格锁定格式（Preference weight：X.X，全角冒号 + 一位小数）。
// 提交走两条互补路径：拖动松手的 onChangeComplete 即时提交；
// 键盘方向键在部分 rc-slider 版本不触发 onChangeComplete，
// 因此 onChange 侧带 400ms 去抖兜底——两路用同一清理逻辑互相去重。
export function WeightControl({
  dish,
  disabled,
  onCommit,
}: {
  dish: PoolDish;
  disabled?: boolean;
  onCommit: (value: number) => void;
}) {
  const [draft, setDraft] = useState<number | null>(null);
  const commitTimer = useRef<ReturnType<typeof setTimeout> | undefined>(
    undefined,
  );
  const value = draft ?? dish.preference_weight;

  useEffect(() => () => clearTimeout(commitTimer.current), []);

  const commit = (committed: number) => {
    clearTimeout(commitTimer.current);
    setDraft(null);
    if (committed !== dish.preference_weight) {
      onCommit(committed);
    }
  };

  return (
    <div className="weight-control">
      <span className="num weight-readout">
        {copy.pool.weightPrefix}
        {formatWeight(value)}
      </span>
      <Slider
        min={WEIGHT_MIN}
        max={WEIGHT_MAX}
        step={WEIGHT_STEP}
        value={value}
        disabled={disabled}
        onChange={(next) => {
          setDraft(next);
          clearTimeout(commitTimer.current);
          commitTimer.current = setTimeout(() => commit(next), 400);
        }}
        onChangeComplete={commit}
        ariaLabelForHandle={`${dish.name} Preference weight`}
      />
    </div>
  );
}
