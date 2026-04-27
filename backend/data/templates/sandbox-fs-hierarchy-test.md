## Task: Sandbox File Hierarchy Test

**ID:** `sandbox-fs-hierarchy-test`
**Category:** System

Test the agent's permission levels and ability to manage nested directory structures and configuration files within its workspace.

### Execution Strategy

#### Action: Multi-Level Structure Creation

**Action:** Construct a multi-folder project and manage dependencies via configuration files.

- **Directory Structure**: Create `project-alpha/src`.
- **Logic Script**: Write a Fibonacci sequence generator (first 10 numbers) in `src/index.ts`.
- **Configuration**: Generate a valid `tsconfig.json` in the root of `project-alpha`.
- **Build**: Compile the project and execute the resulting JavaScript using a robust runtime command (e.g., `npx tsc` and `node`).

### Analysis Goals

- Verify recursive directory creation permissions.
- Check if the agent correctly handles relative file paths during compilation.
- Ensure the agent can generate and utilize auxiliary configuration files (`tsconfig.json`).

### Output Format

1. Workspace Tree Visualization
2. Generated `tsconfig.json` content
3. Fibonacci Sequence Output
