import { ref } from 'vue'
import { AuthService } from '../../services/auth/authService'
import { errorMessage } from '../../utils/errors'

const CHECK_FAILED = 'The passphrase could not be checked.'

// The "confirm it's you" prompt. Any request that needs the passphrase again (httpClient) waits here; everything that
// asks while the prompt is open shares the one answer.
const open = ref(false)
const saving = ref(false)
const error = ref('')
let pending: { promise: Promise<boolean>; resolve: (ok: boolean) => void } | null = null

/** Opens the prompt; true when the user confirmed, false when they cancelled. */
function requestReauth(): Promise<boolean> {
  if (pending) return pending.promise
  let resolve: (ok: boolean) => void = () => {}
  const promise = new Promise<boolean>((r) => { resolve = r })
  pending = { promise, resolve }
  error.value = ''
  open.value = true
  return promise
}

function finish(ok: boolean): void {
  open.value = false
  const p = pending
  pending = null
  p?.resolve(ok)
}

async function submit(passphrase: string): Promise<void> {
  saving.value = true
  try {
    await AuthService.reauth(passphrase)
    error.value = ''
    finish(true)
  } catch (err) {
    error.value = errorMessage(err, CHECK_FAILED)
  } finally {
    saving.value = false
  }
}

const cancel = () => finish(false)

export function useReauth() {
  return { open, saving, error, requestReauth, submit, cancel }
}
