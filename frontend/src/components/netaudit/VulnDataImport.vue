<script setup lang="ts">
import { computed, ref } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import { useVulnData } from "../../composables/netaudit/useVulnData"
import { VULN_IMPORT_FILES, VULN_IMPORT_INTRO } from "../../constants/netaudit"
import type { VulnImportSource } from "../../types/netaudit"

// Settings · Vulnerability data · "Import a file instead": for a computer that cannot reach the Internet, the person
// fetches the file elsewhere and hands it over. The server checks that it is real data and that it is not too large;
// nothing in the file is ever shown. NVD's file does not say which year it is, so the person does.
const { busy, importFile } = useVulnData()

const source = ref<VulnImportSource | "">("")
const file = ref<File | null>(null)
const picker = ref<HTMLInputElement | null>(null)

// The five years the data covers: this year and the four before (the server accepts exactly these).
const THIS_YEAR = new Date().getFullYear()
const YEARS = Array.from({ length: 5 }, (_, i) => THIS_YEAR - i)
const year = ref(THIS_YEAR)

const canImport = computed(() => !!file.value && source.value !== "" && !busy.value)

function onPick(event: Event) {
  file.value = (event.target as HTMLInputElement).files?.[0] ?? null
}

async function run() {
  if (!file.value || source.value === "") return
  await importFile(source.value, file.value, source.value === "nvd" ? year.value : undefined)
  if (picker.value) picker.value.value = ""
  file.value = null
}
</script>

<template>
  <div class="flex flex-col gap-3" data-test="vuln-import">
    <p class="m-0 font-semibold text-primary">Import a file instead</p>
    <p class="m-0 text-[length:var(--text-small)] text-muted">{{ VULN_IMPORT_INTRO }}</p>
    <ul class="m-0 flex list-none flex-col gap-0.5 p-0 text-[length:var(--text-small)] text-muted" aria-label="Where the files are">
      <li v-for="f in VULN_IMPORT_FILES" :key="f.source"><span class="text-secondary">{{ f.name }}:</span> <span class="break-all font-mono">{{ f.where }}</span></li>
    </ul>
    <div role="radiogroup" aria-label="Which file it is" data-test="import-source" class="flex flex-wrap gap-x-4 gap-y-1">
      <label v-for="f in VULN_IMPORT_FILES" :key="f.source" class="flex min-h-11 items-center gap-2 text-secondary sm:min-h-0">
        <input v-model="source" type="radio" name="vuln-import-source" :value="f.source" /> {{ f.name }}
      </label>
    </div>
    <label v-if="source === 'nvd'" class="flex flex-wrap items-center gap-2 text-secondary">
      Year of the file
      <select v-model.number="year" data-test="import-year" class="form-control w-auto max-sm:min-h-11">
        <option v-for="y in YEARS" :key="y" :value="y">{{ y }}</option>
      </select>
    </label>
    <input ref="picker" type="file" accept=".json,.gz,application/json,application/gzip" aria-label="The data file" class="max-w-full text-[length:var(--text-small)] text-secondary" @change="onPick" />
    <div>
      <BaseButton variant="secondary" class-name="max-sm:!h-11" :disabled="!canImport" :loading="busy" @click="run">Import file</BaseButton>
    </div>
  </div>
</template>
