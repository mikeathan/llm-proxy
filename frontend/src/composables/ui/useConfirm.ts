import { ref } from 'vue'
import type { DialogType } from '../../types/ui'

interface ConfirmOptions {
  title: string
  message: string
  type?: DialogType
  confirmText?: string
  cancelText?: string
}

const isOpen = ref(false)
const options = ref<ConfirmOptions>({
  title: '',
  message: '',
  type: 'info',
  confirmText: 'Confirm',
  cancelText: 'Cancel'
})

let resolvePromise: (value: boolean) => void
let pendingQueue: { opts: ConfirmOptions; resolve: (value: boolean) => void }[] = []

export function useConfirm() {
  const confirm = (opts: ConfirmOptions): Promise<boolean> => {
    return new Promise((resolve) => {
      pendingQueue.push({ opts, resolve })
      if (!isOpen.value) showNext()
    })
  }

  const showNext = () => {
    const next = pendingQueue.shift()
    if (!next) return
    options.value = {
      type: 'warning',
      confirmText: 'Confirm',
      cancelText: 'Cancel',
      ...next.opts,
    }
    isOpen.value = true
    resolvePromise = (val: boolean) => {
      next.resolve(val)
      isOpen.value = false
      showNext()
    }
  }

  const handleConfirm = () => {
    if (resolvePromise) resolvePromise(true)
  }

  const handleCancel = () => {
    if (resolvePromise) resolvePromise(false)
  }

  return {
    isOpen,
    options,
    confirm,
    handleConfirm,
    handleCancel,
  }
}
