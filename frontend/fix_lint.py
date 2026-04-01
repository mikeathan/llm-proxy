with open('../frontend/src/composables/useWorkspaces.ts', 'r') as f:
    c = f.read()

c = c.replace("import { ref, shallowRef } from 'vue'", "import { ref } from 'vue'")
c = c.replace("import type { Workspace, WorkspaceConfig } from '../types/workspace'", "import type { Workspace } from '../types/workspace'")

with open('../frontend/src/composables/useWorkspaces.ts', 'w') as f:
    f.write(c)
