import { onMounted, onUnmounted, ref } from 'vue'
import { McpApiService } from '../../services/mcp/mcpService'
import { POLL_INTERVAL_MS } from '../../constants/api'
import { MCP_ACTION_FAILED, MCP_TEST_FAILED } from '../../constants/mcp'
import { errorMessage } from '../../utils/errors'
import type { McpServer, McpServerBody, McpServerForm, McpServersAnswer, McpTestRequest, McpTrust } from '../../types/mcp'

const LOAD_FAILED = 'The MCP servers could not be read.'
// While a server is still connecting its status changes within seconds, so the list is read faster until it settles.
const CONNECTING_POLL_MS = 2000

/** The key the form's own test uses, for an entry that is not saved. */
export const NEW_ENTRY = '__new_entry__'

// The MCP servers, shared by everything that shows them: one copy of the data and one timer however many are mounted. The timer runs while at least
// one consumer is mounted, pauses while the tab is hidden and refreshes when it is shown again; when the last consumer goes, the data is dropped.
const answer = ref<McpServersAnswer | null>(null)
const loading = ref(true)
const error = ref<string | null>(null)
const actionError = ref('')
const saving = ref(false)
// The server a test is running for ('' when none; the form's own test uses NEW_ENTRY).
const testing = ref('')
// The last answer of a test, by server name (or NEW_ENTRY for the form), shown where it was asked.
const tests = ref<Record<string, McpServer>>({})

let consumers = 0
let timer: ReturnType<typeof setTimeout> | null = null
// Bumped when the data is dropped, so a read that was in flight is ignored.
let epoch = 0

const anyConnecting = (): boolean => answer.value?.servers.some((s) => s.status === 'connecting') ?? false

async function load(): Promise<void> {
  const mine = epoch
  try {
    const next = await McpApiService.list()
    if (mine !== epoch) return
    answer.value = { enabled: next.enabled, servers: next.servers ?? [] }
    error.value = null
  } catch (e) {
    if (mine !== epoch) return
    error.value = errorMessage(e, LOAD_FAILED)
  } finally {
    if (mine === epoch) loading.value = false
  }
}

function schedule(): void {
  if (timer) clearTimeout(timer)
  timer = null
  if (consumers === 0 || document.visibilityState === 'hidden') return
  timer = setTimeout(async () => {
    await load()
    schedule()
  }, anyConnecting() ? CONNECTING_POLL_MS : POLL_INTERVAL_MS)
}

function onVisibilityChange(): void {
  if (document.visibilityState === 'hidden') {
    if (timer) clearTimeout(timer)
    timer = null
  } else {
    void load().then(schedule)
  }
}

function acquire(): void {
  consumers++
  if (consumers > 1) return
  void load().then(schedule)
  document.addEventListener('visibilitychange', onVisibilityChange)
}

function release(): void {
  consumers--
  if (consumers > 0) return
  if (timer) clearTimeout(timer)
  timer = null
  document.removeEventListener('visibilitychange', onVisibilityChange)
  epoch++
  answer.value = null
  loading.value = true
  error.value = null
  actionError.value = ''
  tests.value = {}
}

/** The body of a write from the form; the entry's own switch and switched-off tools are kept. */
export function bodyFromForm(form: McpServerForm, current?: McpServer): McpServerBody {
  const { secret, ...rest } = form
  return { ...rest, enabled: current?.enabled ?? true, disabled_tools: current?.disabled_tools ?? [], ...(secret.trim() ? { secret } : {}) }
}

/** The body of a write that changes one thing about a saved server and leaves the rest (and its stored sign-in value) as it is. */
export function bodyFromServer(server: McpServer, change: Partial<McpServerBody> = {}): McpServerBody {
  return {
    name: server.name, url: server.url, enabled: server.enabled, trust: server.trust, auth_kind: server.auth_kind,
    auth_header: server.auth_header ?? '', ca_cert: server.ca_cert ?? '', disabled_tools: server.disabled_tools, ...change,
  }
}

async function change(action: () => Promise<unknown>): Promise<boolean> {
  saving.value = true
  actionError.value = ''
  try {
    await action()
    await load()
    schedule()
    return true
  } catch (e) {
    actionError.value = errorMessage(e, MCP_ACTION_FAILED)
    return false
  } finally {
    saving.value = false
  }
}

const add = (form: McpServerForm) => change(() => McpApiService.add(bodyFromForm(form)))
const save = (form: McpServerForm, current: McpServer) => change(() => McpApiService.update(bodyFromForm(form, current)))
const remove = (name: string) => change(() => McpApiService.remove(name))
const setTrust = (server: McpServer, trust: McpTrust) => change(() => McpApiService.update(bodyFromServer(server, { trust })))
const setEnabled = (server: McpServer, enabled: boolean) => change(() => McpApiService.update(bodyFromServer(server, { enabled })))
const setToolOff = (server: McpServer, tool: string, off: boolean) =>
  change(() => McpApiService.update(bodyFromServer(server, { disabled_tools: off ? [...new Set([...server.disabled_tools, tool])] : server.disabled_tools.filter((t) => t !== tool) })))

async function runTest(key: string, request: McpTestRequest): Promise<void> {
  testing.value = key
  actionError.value = ''
  try {
    tests.value = { ...tests.value, [key]: await McpApiService.test(request) }
  } catch (e) {
    actionError.value = errorMessage(e, MCP_TEST_FAILED)
  } finally {
    testing.value = ''
  }
}

const testSaved = (name: string) => runTest(name, { name })
const testForm = (form: McpServerForm, current?: McpServer) => runTest(current?.name ?? NEW_ENTRY, { ...bodyFromForm(form, current), enabled: true })
const clearTest = (key: string): void => {
  const rest = { ...tests.value }
  delete rest[key]
  tests.value = rest
}

export function useMcpServers() {
  onMounted(acquire)
  onUnmounted(release)
  return { answer, loading, error, actionError, saving, testing, tests, load, add, save, remove, setTrust, setEnabled, setToolOff, testSaved, testForm, clearTest }
}
