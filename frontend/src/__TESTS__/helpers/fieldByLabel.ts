import type { DOMWrapper, VueWrapper } from '@vue/test-utils'

const CONTROLS = 'input, select, textarea'

/**
 * The form control a visible label names — by `for`, by nesting, or (for
 * older markup) the first control in the label's group, falling back to a
 * placeholder — so a test reads like a user finding a field, whatever the markup.
 */
export function fieldByLabel(w: VueWrapper, text: RegExp): DOMWrapper<HTMLElement> {
  const label = w.findAll('label').find((l) => text.test(l.text()))
  if (!label) {
    const byPlaceholder = w.findAll<HTMLElement>('input, textarea').find((i) => text.test(i.attributes('placeholder') ?? ''))
    if (!byPlaceholder) throw new Error(`No label or placeholder matching ${text}`)
    return byPlaceholder
  }
  const el = label.element as HTMLLabelElement
  const target =
    (el.htmlFor ? w.element.querySelector(`#${CSS.escape(el.htmlFor)}`) : null) ??
    el.querySelector(CONTROLS) ??
    el.parentElement?.querySelector(CONTROLS)
  if (!target) throw new Error(`No control for label ${text}`)
  return w.find<HTMLElement>(`#${ensureId(target)}`)
}

let next = 0
function ensureId(el: Element): string {
  if (!el.id) el.id = `field-by-label-${++next}`
  return CSS.escape(el.id)
}
