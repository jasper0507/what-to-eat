import type { Transition } from "motion/react";

// 全应用共享的弹簧参数——物理动效的唯一出处。
// MotionConfig reducedMotion="user" 在 main.tsx 全局生效，无需逐处守卫。
export const spring: Transition = {
  type: "spring",
  stiffness: 300,
  damping: 26,
  mass: 0.9,
};
export const springSnappy: Transition = {
  type: "spring",
  stiffness: 420,
  damping: 30,
  mass: 0.7,
};
export const springSoft: Transition = {
  type: "spring",
  stiffness: 200,
  damping: 26,
  mass: 1,
};

/** 页面/区块入场：淡入 + 上浮。 */
export const pageEnter = {
  initial: { opacity: 0, y: 16 },
  animate: { opacity: 1, y: 0 },
  transition: spring,
} as const;

/** 列表项 stagger 用的子项变体。 */
export const riseItem = {
  hidden: { opacity: 0, y: 12 },
  visible: { opacity: 1, y: 0, transition: springSnappy },
} as const;

export const staggerList = {
  hidden: {},
  visible: { transition: { staggerChildren: 0.05 } },
} as const;

/** 按压微缩——所有可点击大控件共用。 */
export const pressable = {
  whileTap: { scale: 0.98 },
  transition: springSnappy,
} as const;
