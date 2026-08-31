import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'

// Tests live in src/__TESTS__/ mirroring the source tree (a test for
// src/utils/message/textAppend.ts goes in src/__TESTS__/utils/message/).
//
// Two projects:
//  - "unit": plain node-environment composable/utility tests (no DOM).
//  - "component": happy-dom tests for Vue components (suffixed .component.test.ts).
//    Carries the Vue plugin so .vue SFCs compile under the test runner.
export default defineConfig({
  test: {
    // Root-level: applies to all projects (not a valid per-project option in Vitest 4).
    passWithNoTests: true,
    // Coverage (CI: `npm test -- --coverage`). Local runs are unaffected.
    coverage: {
      provider: 'v8',
      include: ['src/**'],
      exclude: ['src/__TESTS__/**', 'src/**/*.d.ts'],
      reporter: ['text-summary', 'html', 'json-summary'],
      reportsDirectory: 'coverage',
    },
    projects: [
      {
        test: {
          name: 'unit',
          environment: 'node',
          include: ['src/__TESTS__/**/*.test.ts'],
          exclude: ['src/__TESTS__/**/*.component.test.ts'],
        },
      },
      {
        plugins: [vue()],
        test: {
          name: 'component',
          environment: 'happy-dom',
          include: ['src/__TESTS__/**/*.component.test.ts'],
        },
      },
    ],
  },
})
