<script setup lang="ts">
import type { Node } from '~/composables/useVault'
import { WrongPasswordError } from '~/lib/cryptomator/index.js'

const props = defineProps<{ folder: Node }>()
const emit = defineEmits<{ close: [] }>()

const { t } = useI18n()
const vault = useVault()
const { keys, dirStack, entries, unlock, enter, breadcrumbTo, lock } = vault

// --- unlock ---
const password = ref('')
const unlockError = ref('')
const unlocking = ref(false)
const countdown = ref(0)
let countdownTimer: ReturnType<typeof setInterval> | null = null

function startCountdown(until: number) {
  countdown.value = Math.ceil((until - Date.now()) / 1000)
  if (countdownTimer) clearInterval(countdownTimer)
  countdownTimer = setInterval(() => {
    countdown.value = Math.max(0, Math.ceil((until - Date.now()) / 1000))
    if (countdown.value === 0 && countdownTimer) {
      clearInterval(countdownTimer)
      countdownTimer = null
    }
  }, 1000)
}

onBeforeUnmount(() => { if (countdownTimer) clearInterval(countdownTimer) })

async function doUnlock() {
  if (unlocking.value) return
  unlocking.value = true
  unlockError.value = ''
  try {
    await unlock(props.folder, password.value)
    password.value = ''
  } catch (e: any) {
    if (e instanceof WrongPasswordError) {
      unlockError.value = t('vault.error_wrong_password')
      if (import.meta.client) {
        const raw = localStorage.getItem(`kf_vault_lock_${props.folder.id}`)
        if (raw) {
          const rs = JSON.parse(raw)
          if (rs.lockUntil > Date.now()) { unlockError.value = ''; startCountdown(rs.lockUntil) }
        }
      }
    } else if (e?.message?.includes('Too many attempts')) {
      const raw = import.meta.client ? localStorage.getItem(`kf_vault_lock_${props.folder.id}`) : null
      if (raw) {
        const rs = JSON.parse(raw)
        startCountdown(rs.lockUntil)
      } else {
        unlockError.value = e.message
      }
    } else {
      unlockError.value = e?.message || t('vault.error_unlock')
    }
  } finally {
    unlocking.value = false
  }
}

// --- file preview ---
const preview = reactive({
  open: false, name: '', text: null as string | null,
  imgUrl: null as string | null, blobUrl: null as string | null,
  error: '', busy: false,
})

const imageExts = new Set(['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg'])

function extOf(name: string): string {
  const i = name.lastIndexOf('.')
  return i >= 0 ? name.slice(i + 1).toLowerCase() : ''
}

function mimeByExt(name: string): string {
  const ext = extOf(name)
  const map: Record<string, string> = {
    png: 'image/png', jpg: 'image/jpeg', jpeg: 'image/jpeg',
    gif: 'image/gif', webp: 'image/webp', svg: 'image/svg+xml',
  }
  return map[ext] ?? 'application/octet-stream'
}

function closePreview() {
  if (preview.blobUrl) URL.revokeObjectURL(preview.blobUrl)
  Object.assign(preview, { open: false, name: '', text: null, imgUrl: null, blobUrl: null, error: '', busy: false })
}

async function openEntry(entry: (typeof entries.value)[0]) {
  if (entry.isDir) { await enter(entry); return }
  preview.busy = true; preview.open = true; preview.name = entry.name
  preview.text = null; preview.imgUrl = null; preview.blobUrl = null; preview.error = ''
  try {
    const result = await vault.openFile(entry)
    const ext = extOf(result.name)
    if (imageExts.has(ext)) {
      const blob = new Blob([result.bytes as BlobPart], { type: mimeByExt(result.name) })
      const url = URL.createObjectURL(blob)
      preview.blobUrl = url; preview.imgUrl = url
    } else {
      try {
        preview.text = new TextDecoder('utf-8', { fatal: true }).decode(result.bytes)
        const blob = new Blob([result.bytes as BlobPart], { type: 'text/plain' })
        preview.blobUrl = URL.createObjectURL(blob)
      } catch {
        const blob = new Blob([result.bytes as BlobPart], { type: 'application/octet-stream' })
        preview.blobUrl = URL.createObjectURL(blob)
      }
    }
  } catch (e: any) {
    preview.error = e?.message || t('vault.error_unlock')
  } finally {
    preview.busy = false
  }
}

function downloadPreview() {
  if (!preview.blobUrl) return
  const a = document.createElement('a')
  a.href = preview.blobUrl; a.download = preview.name; a.click()
}

function doLock() { lock() }

const isUnlockOpen = computed(() => !keys.value)
const isPreviewOpen = computed(() => preview.open)

useModalEscape(isPreviewOpen, closePreview)
useModalEscape(computed(() => isUnlockOpen.value && !isPreviewOpen.value), () => emit('close'))
useModalEscape(computed(() => !!keys.value && !isPreviewOpen.value), () => emit('close'))
</script>

<template>
  <div class="fixed inset-0 z-20 flex items-center justify-center bg-black/50 p-4" @click.self="emit('close')">

    <!-- UNLOCK MODAL -->
    <div v-if="!keys" class="card w-full max-w-sm p-6">
      <div class="mb-4 flex items-center justify-between">
        <h2 class="flex items-center gap-2 font-semibold">
          <Icon name="lucide:lock" size="18" class="text-accent" />
          {{ t('vault.title') }}
        </h2>
        <button class="btn-ghost px-1.5 py-1" @click="emit('close')">
          <Icon name="lucide:x" size="18" />
        </button>
      </div>

      <p class="mb-4 text-sm text-muted">{{ folder.name }}</p>

      <!-- rate limit -->
      <div v-if="countdown > 0" class="mb-4 rounded-md bg-danger/10 px-3 py-2.5 text-sm text-danger">
        <Icon name="lucide:clock" size="15" class="mr-1 inline" />
        {{ t('vault.rate_limit') }}
        <span class="font-mono font-semibold">{{ Math.floor(countdown / 60) }}:{{ String(countdown % 60).padStart(2, '0') }}</span>
      </div>

      <div v-else class="space-y-3">
        <input
          v-model="password"
          type="password"
          class="input"
          :placeholder="t('vault.password_ph')"
          autofocus
          :disabled="unlocking"
          @keyup.enter="doUnlock"
        />

        <p v-if="unlockError" class="flex items-center gap-1.5 text-sm text-danger">
          <Icon name="lucide:triangle-alert" size="15" />
          {{ unlockError }}
        </p>

        <button class="btn-accent w-full justify-center" :disabled="unlocking || !password" @click="doUnlock">
          <Icon v-if="unlocking" name="lucide:loader-circle" class="animate-spin" size="18" />
          <Icon v-else name="lucide:unlock" size="18" />
          {{ t('vault.btn_unlock') }}
        </button>
      </div>

      <p class="mt-5 text-[11px] leading-relaxed text-muted/70">
        {{ t('vault.security_note') }}
      </p>
    </div>

    <!-- VAULT BROWSER -->
    <div v-else class="card flex w-full max-w-2xl flex-col" style="max-height: 85vh">
      <div class="flex items-center justify-between border-b border-line px-5 py-3.5">
        <nav class="flex flex-wrap items-center gap-1 text-sm min-w-0">
          <template v-for="(crumb, i) in dirStack" :key="i">
            <Icon v-if="i > 0" name="lucide:chevron-right" size="14" class="shrink-0 text-muted" />
            <button
              class="rounded px-1.5 py-0.5 hover:bg-ink/5 truncate max-w-[160px]"
              :class="i === dirStack.length - 1 ? 'text-ink' : 'text-muted'"
              :title="crumb.name"
              @click="breadcrumbTo(i)"
            >{{ crumb.name }}</button>
          </template>
        </nav>

        <div class="flex shrink-0 items-center gap-1 pl-3">
          <button class="btn-ghost px-2 py-1 text-xs" @click="doLock">
            <Icon name="lucide:lock" size="15" />
            {{ t('vault.btn_lock') }}
          </button>
          <button class="btn-ghost px-1.5 py-1" @click="emit('close')">
            <Icon name="lucide:x" size="18" />
          </button>
        </div>
      </div>

      <div class="overflow-y-auto">
        <div v-if="!entries.length" class="p-10 text-center text-sm text-muted">
          <Icon name="lucide:folder-open" size="28" class="mx-auto mb-2 block opacity-50" />
          {{ t('vault.folder_empty') }}
        </div>
        <table v-else class="w-full text-sm">
          <tbody>
            <tr
              v-for="entry in entries"
              :key="entry.name + (entry.dirId ?? entry.nodeId ?? '')"
              class="group border-b border-line/50 last:border-0 hover:bg-ink/5"
            >
              <td class="w-8 py-2.5 pl-4">
                <Icon
                  :name="entry.isDir ? 'lucide:folder' : 'lucide:file'"
                  :class="entry.isDir ? 'text-accent' : 'text-muted'"
                  size="18"
                />
              </td>
              <td class="py-2.5 pr-4">
                <button class="hover:underline text-left w-full" @click="openEntry(entry)">
                  {{ entry.name }}
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>

  <!-- FILE PREVIEW -->
  <div
    v-if="preview.open"
    class="fixed inset-0 z-30 flex items-center justify-center bg-black/60 p-4"
    @click.self="closePreview"
  >
    <div class="card flex w-full max-w-3xl flex-col" style="max-height: 90vh">
      <div class="flex items-center justify-between border-b border-line px-5 py-3.5">
        <span class="truncate font-medium text-sm pr-4">{{ preview.name }}</span>
        <div class="flex shrink-0 items-center gap-1">
          <button v-if="preview.blobUrl" class="btn-accent px-2 py-1 text-xs" @click="downloadPreview">
            <Icon name="lucide:download" size="15" />
            {{ t('common.download') }}
          </button>
          <button class="btn-ghost px-1.5 py-1" @click="closePreview">
            <Icon name="lucide:x" size="18" />
          </button>
        </div>
      </div>

      <div class="overflow-auto p-4">
        <div v-if="preview.busy" class="py-10 text-center text-sm text-muted">
          <Icon name="lucide:loader-circle" class="animate-spin mx-auto mb-2 block" size="28" />
          {{ t('vault.decrypting') }}
        </div>

        <p v-else-if="preview.error" class="text-sm text-danger">
          <Icon name="lucide:triangle-alert" size="15" class="mr-1 inline" />
          {{ preview.error }}
        </p>

        <img
          v-else-if="preview.imgUrl"
          :src="preview.imgUrl"
          class="mx-auto max-w-full rounded"
          :alt="preview.name"
        />

        <pre
          v-else-if="preview.text !== null"
          class="whitespace-pre-wrap break-words font-mono text-xs text-ink leading-relaxed"
        >{{ preview.text }}</pre>

        <div v-else-if="preview.blobUrl" class="py-10 text-center">
          <Icon name="lucide:file-question" size="32" class="mx-auto mb-3 block text-muted/60" />
          <p class="text-sm text-muted">{{ t('vault.binary_no_preview') }}</p>
        </div>
      </div>
    </div>
  </div>
</template>
