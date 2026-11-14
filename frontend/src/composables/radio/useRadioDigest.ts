import { ref } from 'vue'
import { RadioService } from '../../services/radio/radioService'
import { errorMessage } from '../../utils/errors'
import { RADIO_FINDINGS_FAILED } from '../../constants/radio'
import type { RadioDigest } from '../../types/radio'

/** Radio · Findings: the digest of what the stack found. */
export function useRadioDigest() {
  const digest = ref<RadioDigest | null>(null)
  const error = ref('')

  async function load(): Promise<void> {
    try {
      digest.value = await RadioService.digest()
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, RADIO_FINDINGS_FAILED)
    }
  }

  return { digest, error, load }
}
