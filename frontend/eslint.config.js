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
    name: "types-must-live-in-src-types",
    files: ["src/**/*.ts", "src/**/*.vue"],
    ignores: ["src/types/**"],
    rules: {
      "no-restricted-syntax": [
        "error",
        {
          selector:
            "ExportNamedDeclaration > TSInterfaceDeclaration, ExportNamedDeclaration > TSTypeAliasDeclaration, ExportNamedDeclaration:has(TSInterfaceDeclaration), ExportNamedDeclaration:has(TSTypeAliasDeclaration)",
          message:
            "Export types/interfaces only from src/types/. Move the declaration to src/types/ and import it with `import type`. (Rule: frontend-vue-engineer.md → TypeScript → 'Export types ONLY from src/types/')",
        },
      ],
    },
  },
  {
    // No raw palette classes (plan D21), everywhere: colours come from semantic
    // tokens only. The Phase 5 per-directory list ended with the close-out
    // (2026-09-30); scripts/check-palette.mjs also covers @apply and TS literals.
    name: "no-palette-classes",
    files: ["src/**/*.vue"],
    rules: {
      "vue/no-restricted-class": [
        "error",
        "/^(?:[a-z0-9-]+:)*(?:bg|text|border(?:-[trblxy])?|ring|ring-offset|divide(?:-[xy])?|placeholder|from|to|via|outline|fill|stroke|shadow|accent|caret|decoration)-(?:gray|slate|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose|white|black)(?:-\\d{2,3})?(?:\\/\\d+)?$/",
      ],
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
