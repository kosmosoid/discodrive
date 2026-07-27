<script setup lang="ts">
// Spreadsheet renderer: SheetJS parses the workbook (lazy chunk), the table
// below is our own DOM built from cell values through Vue's escaping — no HTML
// from the file ever reaches the page. Rows/cols are sliced in lib/preview/sheet.
import { openWorkbook, type WorkbookView, type SheetSlice } from '~/lib/preview/sheet'

const props = defineProps<{ blob: Blob }>()

const { t } = useI18n()

const busy = ref(true)
const error = ref(false)
const wb = shallowRef<WorkbookView | null>(null)
const active = ref('')
const slice = shallowRef<SheetSlice | null>(null)

onMounted(async () => {
  try {
    wb.value = await openWorkbook(await props.blob.arrayBuffer())
    if (wb.value.names.length) select(wb.value.names[0])
  } catch {
    error.value = true
  } finally {
    busy.value = false
  }
})

function select(name: string) {
  active.value = name
  slice.value = wb.value?.slice(name) ?? null
}
</script>

<template>
  <div v-if="busy" class="py-14 text-center text-sm text-muted">
    <Icon name="lucide:loader-circle" class="mx-auto mb-2 block animate-spin" size="28" />
    {{ t('common.loading') }}
  </div>

  <p v-else-if="error || !slice" class="py-14 text-center text-sm text-danger">
    <Icon name="lucide:triangle-alert" size="15" class="mr-1 inline" />
    {{ t('preview.error') }}
  </p>

  <div v-else>
    <div
      v-if="(wb?.names.length ?? 0) > 1 || slice.truncated"
      class="sticky top-0 z-10 flex flex-wrap items-center gap-1 border-b border-line bg-panel px-3 py-1.5"
    >
      <template v-if="(wb?.names.length ?? 0) > 1">
        <button
          v-for="name in wb!.names"
          :key="name"
          class="rounded px-2 py-0.5 text-xs"
          :class="name === active ? 'bg-accent/15 text-accent ring-1 ring-accent/30' : 'text-muted hover:bg-ink/5 hover:text-ink'"
          @click="select(name)"
        >{{ name }}</button>
      </template>
      <span v-if="slice.truncated" class="ml-auto text-[11px] text-muted">
        {{ t('preview.truncated', { rows: slice.rows.length, cols: slice.rows[0]?.length ?? 0 }) }}
      </span>
    </div>

    <div v-if="!slice.rows.length" class="py-14 text-center text-sm text-muted">
      {{ t('files.folder_empty') }}
    </div>
    <table v-else class="sheet-table m-3 text-xs">
      <tbody>
        <tr v-for="(row, ri) in slice.rows" :key="ri">
          <td class="sheet-rowno">{{ ri + 1 }}</td>
          <td v-for="(cell, ci) in row" :key="ci">{{ cell }}</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<style>
.sheet-table { border-collapse: collapse; }
.sheet-table td {
  border: 1px solid rgb(var(--c-ink) / 0.15);
  padding: 0.25em 0.6em;
  max-width: 24rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.sheet-table .sheet-rowno {
  background: rgb(var(--c-panel2));
  color: rgb(var(--c-muted));
  text-align: right;
  font-size: 10px;
  user-select: none;
}
</style>
