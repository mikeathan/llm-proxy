import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import WorkspaceExplorer from '../../../../components/AgentIde/workspace/WorkspaceExplorer.vue'

const stubs = {
  Icon: true,
  InlineConfirm: true,
  NotificationDot: true,
}

function mountExplorer() {
  return mount(WorkspaceExplorer, {
    props: {
      workspaces: [],
      workspaceFiles: {},
      selectedWorkspace: null,
      selectedFile: null,
      loading: false,
    },
    global: { stubs },
  })
}

describe('WorkspaceExplorer', () => {
  it('keeps the New-workspace panel open when focus moves to the Add button, so the click creates the workspace', async () => {
    const wrapper = mountExplorer()

    // Open the collapsible "New workspace" input panel and type a name.
    await wrapper.get('button[title="New workspace"]').trigger('click')
    const input = wrapper.get('input[placeholder="New workspace name..."]')
    await input.setValue('my-workspace')
    ;(input.element as HTMLInputElement).focus()

    const addButton = wrapper.get('button[title="Create Workspace"]')

    // Reproduce the browser focus-loss that happens when the Add button is
    // clicked: the input blurs with the button as the new focus target.
    // A naive "@blur -> close panel" would unmount the v-if-gated panel (and
    // the button) before the click lands, swallowing the create. The fix keeps
    // the panel open when focus moves within the action bar.
    const blur = new FocusEvent('blur', { relatedTarget: addButton.element })
    input.element.dispatchEvent(blur)
    await wrapper.vm.$nextTick()

    // The panel must still be mounted so the Add button is clickable.
    expect(wrapper.find('button[title="Create Workspace"]').exists()).toBe(true)

    await addButton.trigger('click')

    expect(wrapper.emitted('create-workspace')).toBeTruthy()
    expect(wrapper.emitted('create-workspace')![0]).toEqual(['my-workspace'])
  })

  it('closes the New-workspace panel when focus moves outside it', async () => {
    const wrapper = mountExplorer()

    await wrapper.get('button[title="New workspace"]').trigger('click')
    const input = wrapper.get('input[placeholder="New workspace name..."]')
    await input.setValue('my-workspace')
    ;(input.element as HTMLInputElement).focus()

    // Focus leaving the panel entirely (relatedTarget outside the bar) closes it.
    const blur = new FocusEvent('blur', { relatedTarget: document.body })
    input.element.dispatchEvent(blur)
    await wrapper.vm.$nextTick()

    expect(wrapper.find('button[title="Create Workspace"]').exists()).toBe(false)
  })
})
