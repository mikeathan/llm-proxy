import pluginVue from "eslint-plugin-vue"
import tseslint from "typescript-eslint"

export default [
  ...pluginVue.configs["flat/base"],
  {
    name: "ts-parser",
    files: ["**/*.ts", "**/*.tsx"],
    languageOptions: {
      parser: tseslint.parser,
      ecmaVersion: "latest",
      sourceType: "module",
    },
  },
  {
    name: "vue-ts-overrides",
    files: ["**/*.vue"],
    languageOptions: {
      parserOptions: {
        parser: tseslint.parser,
      },
    },
    rules: {
      // Fail the build if a component used in a template is not imported.
      "vue/no-undef-components": ["error", { ignorePatterns: ["^router-link$", "^router-view$"] }],
      "vue/multi-word-component-names": "off",
    },
  },
  {
    name: "ignore-build-output",
    ignores: [
      "dist/**",
      "node_modules/**",
      "../backend/internal/transport/http/frontend_dist/**",
      "*.config.js",
      "*.config.ts",
    ],
  },
]
