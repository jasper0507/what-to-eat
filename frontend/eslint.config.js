import js from "@eslint/js";
import pluginQuery from "@tanstack/eslint-plugin-query";
import prettier from "eslint-config-prettier";
import reactHooks from "eslint-plugin-react-hooks";
import reactRefresh from "eslint-plugin-react-refresh";
import tseslint from "typescript-eslint";

export default tseslint.config(
  { ignores: ["dist"] },
  js.configs.recommended,
  ...tseslint.configs.recommendedTypeChecked,
  ...pluginQuery.configs["flat/recommended"],
  reactHooks.configs.flat["recommended-latest"],
  reactRefresh.configs.vite,
  prettier,
  {
    languageOptions: {
      parserOptions: {
        projectService: true,
        tsconfigRootDir: import.meta.dirname,
      },
    },
  },
  {
    files: ["**/*.js"],
    ...tseslint.configs.disableTypeChecked,
  },
  {
    // shadcn/ui 惯例：组件与其 cva variants 同文件导出
    files: ["src/components/ui/**/*.tsx"],
    rules: { "react-refresh/only-export-components": "off" },
  },
);
