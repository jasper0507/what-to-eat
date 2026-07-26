import type { ThemeConfig } from "antd";

// 「夜市灯光」设计体系的唯一 token 来源。样式只允许经由这里的
// ConfigProvider token 与 styles.css 的 CSS 变量下发，禁止覆盖 .ant-* 内部类。
// controlHeight 44 / controlHeightLG 48 结构性满足验收规格的 ≥44px 触控目标。
export const appTheme: ThemeConfig = {
  token: {
    colorPrimary: "#d85c27",
    colorLink: "#b8481c",
    colorLinkHover: "#d85c27",
    colorLinkActive: "#93380f",
    colorTextBase: "#2c201a",
    colorBgLayout: "#fff8ed",
    colorBgContainer: "#fffdf8",
    colorBgElevated: "#fffdf8",
    colorBorder: "#ecd9bf",
    colorBorderSecondary: "#f5ead8",
    colorSuccess: "#5a8a2d",
    colorWarning: "#d59114",
    colorError: "#c8402e",
    colorInfo: "#2f7d8c",
    borderRadius: 14,
    borderRadiusLG: 20,
    borderRadiusSM: 10,
    fontSize: 16,
    fontSizeHeading1: 30,
    fontSizeHeading2: 24,
    fontSizeHeading3: 18,
    lineHeightHeading1: 1.27,
    lineHeightHeading2: 1.33,
    lineHeightHeading3: 1.45,
    fontFamily:
      '-apple-system, "PingFang SC", "Hiragino Sans GB", "Noto Sans SC", "Microsoft YaHei", "Segoe UI", system-ui, sans-serif',
    controlHeight: 44,
    controlHeightLG: 48,
    controlHeightSM: 32,
    controlOutline: "rgba(216, 92, 39, 0.25)",
    controlOutlineWidth: 3,
    boxShadowSecondary: "0 12px 32px rgba(122, 74, 42, 0.14)",
  },
  components: {
    Button: {
      fontWeight: 600,
      contentFontSizeLG: 18,
      primaryShadow: "0 2px 8px rgba(216, 92, 39, 0.24)",
    },
    Card: { bodyPadding: 24 },
    List: { itemPadding: "16px 0" },
    Segmented: {
      trackBg: "#f6e8d2",
      itemSelectedBg: "#fffdf8",
      itemSelectedColor: "#b8481c",
    },
    Slider: {
      railSize: 6,
      handleSize: 18,
      handleSizeHover: 20,
      railBg: "#f6e8d2",
      railHoverBg: "#eddcc0",
    },
    Tag: { defaultBg: "#fdf0dc", defaultColor: "#766b62" },
    Form: { itemMarginBottom: 20, verticalLabelPadding: "0 0 6px" },
    Skeleton: { gradientFromColor: "#f2e6d4", gradientToColor: "#faf3e6" },
    Alert: { borderRadiusLG: 14 },
  },
};
