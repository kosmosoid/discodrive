<script setup lang="ts">
interface TreeNode {
  id: string
  name: string
  is_dir: boolean
  children?: TreeNode[] | null
  expanded?: boolean
  loading?: boolean
}

const props = defineProps<{
  nodes: TreeNode[]
  selectedId: string | null
  depth?: number
  // Icon for leaf (file) nodes. Defaults to a music note; the book editor passes a book icon.
  fileIcon?: string
}>()
const fileIcon = props.fileIcon ?? 'lucide:music'

defineEmits<{
  toggle: [node: TreeNode]
  select: [node: TreeNode]
}>()

const { t } = useI18n()
</script>

<template>
  <ul :class="depth ? 'ml-4' : ''" class="select-none">
    <li v-for="node in nodes" :key="node.id">
      <div class="group flex w-full items-center rounded hover:bg-ink/5" :class="selectedId === node.id ? 'bg-accent/10' : ''">
        <button
          class="flex min-w-0 flex-1 items-center gap-1.5 px-2 py-1 text-left text-sm"
          :class="selectedId === node.id ? 'text-accent' : 'text-ink'"
          @click="node.is_dir ? $emit('toggle', node) : $emit('select', node)"
        >
          <Icon
            v-if="node.is_dir"
            :name="node.expanded ? 'lucide:chevron-down' : 'lucide:chevron-right'"
            size="14"
            class="shrink-0 text-muted"
          />
          <span v-else class="ml-3.5 shrink-0" />
          <Icon
            :name="node.is_dir ? (node.expanded ? 'lucide:folder-open' : 'lucide:folder') : fileIcon"
            size="16"
            :class="node.is_dir ? 'shrink-0 text-accent' : 'shrink-0 text-muted'"
          />
          <span class="min-w-0 flex-1 truncate">
            {{ node.name }}
          </span>
          <Icon v-if="node.loading" name="lucide:loader-circle" class="animate-spin text-muted" size="14" />
        </button>
        <!-- Folder: "edit tags" affordance — appears on hover or when selected -->
        <button
          v-if="node.is_dir"
          class="mr-1 shrink-0 rounded p-0.5 text-muted opacity-0 transition-opacity hover:text-accent group-hover:opacity-100"
          :class="selectedId === node.id ? 'opacity-100 text-accent' : ''"
          :title="t('music.edit_folder_tags')"
          @click.stop="$emit('select', node)"
        >
          <Icon name="lucide:tag" size="13" />
        </button>
      </div>
      <!-- Recurse into expanded folders -->
      <FileTreeList
        v-if="node.is_dir && node.expanded && node.children?.length"
        :nodes="node.children"
        :selected-id="selectedId"
        :depth="(depth ?? 0) + 1"
        :file-icon="fileIcon"
        @toggle="$emit('toggle', $event)"
        @select="$emit('select', $event)"
      />
    </li>
  </ul>
</template>
