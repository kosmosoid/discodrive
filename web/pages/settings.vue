<script setup lang="ts">
import GeneralPanel from '~/components/settings/GeneralPanel.vue'
import SecurityPanel from '~/components/settings/SecurityPanel.vue'
import AccessPanel from '~/components/settings/AccessPanel.vue'
import SyncPanel from '~/components/settings/SyncPanel.vue'
import MusicPanel from '~/components/settings/MusicPanel.vue'
import BooksPanel from '~/components/settings/BooksPanel.vue'
import NotificationsPanel from '~/components/settings/NotificationsPanel.vue'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()

const tabs = [
  { id: 'general', component: GeneralPanel },
  { id: 'security', component: SecurityPanel },
  { id: 'access', component: AccessPanel },
  { id: 'sync', component: SyncPanel },
  { id: 'music', component: MusicPanel },
  { id: 'books', component: BooksPanel },
  { id: 'notifications', component: NotificationsPanel },
] as const

const ids = tabs.map((x) => x.id)
// Active tab is driven by ?tab=; fall back to 'general' for missing/unknown values.
const active = computed(() => {
  const q = route.query.tab
  return typeof q === 'string' && (ids as readonly string[]).includes(q) ? q : 'general'
})
const activeComponent = computed(() => tabs.find((x) => x.id === active.value)!.component)

function select(id: string) {
  router.replace({ query: { ...route.query, tab: id } })
}
</script>

<template>
  <div>
    <h1 class="mb-4 text-xl font-semibold">{{ t('settings.title') }}</h1>

    <!-- Tab bar: horizontal, scrolls on narrow screens -->
    <div class="mb-6 flex gap-1 overflow-x-auto border-b border-line">
      <button
        v-for="tab in tabs"
        :key="tab.id"
        :class="[
          'shrink-0 border-b-2 px-4 py-2 text-sm transition',
          active === tab.id
            ? 'border-accent text-accent'
            : 'border-transparent text-muted hover:text-ink',
        ]"
        @click="select(tab.id)"
      >
        {{ t(`settings.tabs.${tab.id}`) }}
      </button>
    </div>

    <component :is="activeComponent" />
  </div>
</template>
