import { ref } from 'vue'

export type DialogType = 'info' | 'warning' | 'error'

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

export function useConfirm() {
  const confirm = (opts: ConfirmOptions): Promise<boolean> => {
    options.value = {
      type: 'warning',
      confirmText: 'Confirm',
      cancelText: 'Cancel',
      ...opts
    }
    isOpen.value = true
    return new Promise((resolve) => {
      resolvePromise = resolve
    })
  }

  const handleConfirm = () => {
    isOpen.value = false
    if (resolvePromise) resolvePromise(true)
  }

  const handleCancel = () => {
    isOpen.value = false
    if (resolvePromise) resolvePromise(false)
  }

  return {
    isOpen,
    options,
    confirm,
    handleConfirm,
    handleCancel
  }
}
