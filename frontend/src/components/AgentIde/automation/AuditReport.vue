<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { parseAuditReport, fixesSummary, notesSummary } from "../../../utils/netaudit/auditReport";
import Disclosure from "../../common/layout/Disclosure.vue";
import MarkdownViewer from "../../common/display/MarkdownViewer.vue";

// A saved audit report as the operator reads it: the assessment first (what to look at first), then what the code wrote. What the
// operator acts on (what is new, what is still open) stays open; reference parts (what was resolved, the exposed services, the last
// day's movement, the fixes, the notes) fold to one line that says what is inside. A text that is not such a report is plain markdown.

const ASSESSMENT_TITLE = "Assessment";
const FIXES_TITLE = "Suggested fixes";
const NOTES_TITLE = "Notes";
const NOTES_KEY = "notes";

const props = defineProps<{ content: string }>();

const view = computed(() => parseAuditReport(props.content));

// Which folded parts are open now. A new report starts from its defaults.
const opened = ref<Record<string, boolean>>({});
watch(
  view,
  (parsed) => {
    opened.value = Object.fromEntries((parsed?.sections ?? []).map((section) => [section.id, section.open]));
  },
  { immediate: true },
);
const fixesKey = (id: string) => `${id}/fixes`;
const setOpen = (key: string, open: boolean) => {
  opened.value = { ...opened.value, [key]: open };
};
</script>

<template>
  <MarkdownViewer v-if="!view" :content="content" />
  <div v-else class="flex flex-col gap-5" data-test="audit-report">
    <MarkdownViewer v-if="view.preface" :content="view.preface" variant="compact" />

    <section v-if="view.assessment" class="flex flex-col gap-2" data-test="audit-assessment">
      <h3 class="m-0 font-mono text-[length:var(--text-small)] font-medium uppercase tracking-[0.04em] text-primary">{{ ASSESSMENT_TITLE }}</h3>
      <MarkdownViewer :content="view.assessment" />
    </section>

    <MarkdownViewer v-if="view.lead" :content="view.lead" />

    <template v-for="section in view.sections" :key="section.id">
      <section v-if="section.open" class="flex flex-col gap-2" :data-test="`audit-open-${section.title}`">
        <h3 class="m-0 font-mono text-[length:var(--text-small)] font-medium uppercase tracking-[0.04em] text-primary">{{ section.title }}</h3>
        <MarkdownViewer :content="section.body" />
        <Disclosure v-if="section.fixes" :title="FIXES_TITLE" :summary="fixesSummary(section.fixes)" :open="!!opened[fixesKey(section.id)]" @update:open="setOpen(fixesKey(section.id), $event)">
          <MarkdownViewer :content="section.fixes" />
        </Disclosure>
      </section>
      <Disclosure v-else :title="section.title" :summary="section.summary" :open="!!opened[section.id]" :data-test="`audit-folded-${section.title}`" @update:open="setOpen(section.id, $event)">
        <MarkdownViewer :content="section.body" />
        <Disclosure v-if="section.fixes" :title="FIXES_TITLE" :summary="fixesSummary(section.fixes)" :open="!!opened[fixesKey(section.id)]" @update:open="setOpen(fixesKey(section.id), $event)">
          <MarkdownViewer :content="section.fixes" />
        </Disclosure>
      </Disclosure>
    </template>

    <Disclosure v-if="view.notes.length" :title="NOTES_TITLE" :summary="notesSummary(view.notes)" :open="!!opened[NOTES_KEY]" data-test="audit-notes" @update:open="setOpen(NOTES_KEY, $event)">
      <MarkdownViewer :content="view.notes.join('\n\n')" variant="compact" />
    </Disclosure>
  </div>
</template>
