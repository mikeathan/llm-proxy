// Copies text to the clipboard and says whether it worked. The clipboard API exists only on HTTPS and localhost, and this UI
// is often served over plain HTTP on a LAN name (vertex.local), where navigator.clipboard is undefined and the call throws:
// then a hidden box is selected and the browser's copy command is used. A caller shows "Copy failed" (and, for a secret,
// selects it for the person) when this returns false.
export async function copyText(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
      return true
    }
  } catch {
    // Refused (permission, an insecure page): try the older way below.
  }
  return copyThroughSelection(text)
}

function copyThroughSelection(text: string): boolean {
  const previous = document.activeElement as HTMLElement | null
  const box = document.createElement('textarea')
  box.value = text
  box.setAttribute('readonly', '')
  box.setAttribute('aria-hidden', 'true')
  // Off screen but selectable: a display:none box cannot be selected.
  box.style.cssText = 'position:fixed;top:0;left:-9999px;opacity:0;'
  document.body.appendChild(box)
  try {
    box.select()
    box.setSelectionRange(0, text.length)
    return document.execCommand('copy')
  } catch {
    return false
  } finally {
    box.remove()
    previous?.focus?.()
  }
}
