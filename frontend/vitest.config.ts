import { defineConfig } from 'vitest/config'

// Tests live in src/__TESTS__/ mirroring the source tree (a test for
// src/utils/message/textAppend.ts goes in src/__TESTS__/utils/message/).
export default defineConfig({
  test: {
    environment: 'node',
    include: ['src/__TESTS__/**/*.test.ts'],
    passWithNoTests: true,
  },
})