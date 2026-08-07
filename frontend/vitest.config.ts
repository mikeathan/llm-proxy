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
    projects: [
      {
        test: {
          name: 'unit',
          environment: 'node',
          include: ['src/__TESTS__/**/*.test.ts'],
          exclude: ['src/__TESTS__/**/*.component.test.ts'],
          passWithNoTests: true,
        },
      },
      {
        plugins: [vue()],
        test: {
          name: 'component',
          environment: 'happy-dom',
          include: ['src/__TESTS__/**/*.component.test.ts'],
          passWithNoTests: true,
        },
      },
    ],
  },
})
