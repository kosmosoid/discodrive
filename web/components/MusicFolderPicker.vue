<script setup lang="ts">
interface FileNode { id: string; name: string; is_dir: boolean }

const props = defineProps<{ modelValue: { id: string; name: string } | null }>()
const emit = defineEmits<{
  'update:modelValue': [value: { id: string; name: string } | null]
  close: []
}>()

const { t } = useI18n()
const { request } = useApi()

const stack = ref<{ id: string; name: string }[]>([{ id: '', name: t('files.root_name') }])
const nodes = ref<FileNode[]>([])
const busy = ref(false)
const error = ref('')

const parentId = computed(() => stack.value[stack.value.length - 1].id)
const dirs = computed(() => nodes.value.filter((n) => n.is_dir).sort((a, b) => a.name.localeCompare(b.name)))

async function load() {
  busy.value = true
  error.value = ''
  try {
    const q = parentId.value ? `?parent_id=${parentId.value}` : ''
    const all = await request<FileNode[]>(`/files${q}`)
    nodes.value = all
  } catch (e: any) {
    error.value = e?.data?.error || t('common.error_load')
  } finally {
    busy.value = false
  }
}

onMounted(load)

function enter(n: FileNode) {
  stack.value.push({ id: n.id, name: n.name })
  load()
}

function crumbTo(i: number) {
  stack.value = stack.value.slice(0, i + 1)
  load()
}

function selectCurrent() {
  const top = stack.value[stack.value.length - 1]
  // Don't allow selecting the root (empty id)
  if (!top.id) return
  emit('update:modelValue', { id: top.id, name: top.name })
  emit('close')
}

useModalEscape(ref(true), () => emit('close'))
</script>

<template>
  <div class="fixed inset-0 z-20 flex items-center justify-center bg-black/50 p-4" @click.self="emit('close')">
    <div class="card flex w-full max-w-md flex-col" style="max-height: 80vh">
      <div class="flex items-center justify-between border-b border-line px-5 py-3">
        <h2 class="font-semibold">{{ t('settings.music_pick_folder') }}</h2>
        <button class="btn-ghost px-1.5 py-1" @click="emit('close')">
          <Icon name="lucide:x" size="18" />
        </button>
      </div>

      <!-- breadcrumb -->
      <nav class="flex flex-wrap items-center gap-1 border-b border-line px-4 py-2 text-sm">
        <template v-for="(c, i) in stack" :key="i">
          <Icon v-if="i > 0" name="lucide:chevron-right" size="14" class="text-muted" />
          <button
            class="rounded px-1.5 py-0.5 hover:bg-ink/5"
            :class="i === stack.length - 1 ? 'text-ink font-medium' : 'text-muted'"
            @click="crumbTo(i)"
          >{{ c.name }}</button>
        </template>
      </nav>

      <!-- directory list -->
      <div class="min-h-0 flex-1 overflow-y-auto">
        <div v-if="busy && !dirs.length" class="p-6 text-sm text-muted">{{ t('common.loading') }}</div>
        <div v-else-if="!dirs.length && !busy" class="p-6 text-center text-sm text-muted">
          <Icon name="lucide:folder-open" size="24" class="mx-auto mb-2 block opacity-50" />
          {{ t('settings.music_no_subfolders') }}
        </div>
        <table v-else class="w-full text-sm">
          <tbody>
            <tr
              v-for="n in dirs"
              :key="n.id"
              class="group border-b border-line/50 last:border-0 hover:bg-ink/5"
            >
              <td class="w-8 py-2.5 pl-4">
                <Icon name="lucide:folder" class="text-accent" size="18" />
              </td>
              <td class="py-2.5">
                <button class="hover:underline" @click="enter(n)">{{ n.name }}</button>
              </td>
              <td class="py-2.5 pr-4 text-right">
                <button class="btn-ghost px-2 py-1 opacity-0 transition group-hover:opacity-100" @click="() => { emit('update:modelValue', { id: n.id, name: n.name }); emit('close') }">
                  <Icon name="lucide:check" size="15" /> {{ t('settings.music_select') }}
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- footer: select current folder -->
      <div class="flex items-center justify-between border-t border-line px-5 py-3">
        <p v-if="error" class="text-xs text-danger">{{ error }}</p>
        <span v-else class="text-xs text-muted">{{ stack[stack.length - 1].name }}</span>
        <button
          class="btn-accent"
          :disabled="!parentId"
          @click="selectCurrent"
        >
          <Icon name="lucide:check" size="16" />
          {{ t('settings.music_select_this') }}
        </button>
      </div>
    </div>
  </div>
</template>
