<script setup lang="ts">
// Equalizer panel: enable toggle, presets, 10 vertical band sliders. Rendered
// inside a popover (PlayerBar) or overlay (pop-out player) — pure UI, all the
// audio logic sits in useEq.
import { EQ_BANDS, EQ_PRESETS, EQ_GAIN_MIN, EQ_GAIN_MAX } from '~/lib/player/eq'

const { t } = useI18n()
const eq = useEq()
const { enabled, preset, gains } = eq

// closable: show an in-panel X — needed where the panel covers the button that
// opened it (the pop-out mini window).
defineProps<{ closable?: boolean }>()
const emit = defineEmits<{ (e: 'close'): void }>()

function bandLabel(freq: number): string {
  return freq >= 1000 ? `${freq / 1000}k` : String(freq)
}
function onGain(i: number, e: Event) {
  eq.setGain(i, parseFloat((e.target as HTMLInputElement).value))
}
function onPreset(e: Event) {
  const v = (e.target as HTMLSelectElement).value
  if (v !== 'custom') eq.applyPreset(v)
}
</script>

<template>
  <div class="w-72 select-none p-3">
    <div class="mb-2 flex items-center justify-between gap-2">
      <span class="text-sm font-medium">{{ t('player.eq') }}</span>
      <div class="flex items-center gap-2">
        <button
          class="relative h-5 w-9 rounded-full transition"
          :class="enabled ? 'bg-accent' : 'bg-ink/20'"
          :title="t('player.eq_enable')"
          @click="eq.setEnabled(!enabled)"
        >
          <span
            class="absolute top-0.5 h-4 w-4 rounded-full bg-white transition-all"
            :class="enabled ? 'left-[18px]' : 'left-0.5'"
          />
        </button>
        <button v-if="closable" class="btn-ghost px-1.5 py-1" :title="t('common.close')" @click="emit('close')">
          <Icon name="lucide:x" size="16" />
        </button>
      </div>
    </div>

    <div class="mb-3 flex items-center gap-2">
      <select
        class="input h-8 flex-1 py-0 text-xs"
        :value="preset"
        :disabled="!enabled"
        @change="onPreset"
      >
        <option v-for="(_, name) in EQ_PRESETS" :key="name" :value="name">{{ t(`player.preset_${name}`) }}</option>
        <option v-if="preset === 'custom'" value="custom">{{ t('player.preset_custom') }}</option>
      </select>
      <button class="btn-ghost px-2 py-1 text-xs" :disabled="!enabled" :title="t('player.eq_reset')" @click="eq.applyPreset('flat')">
        <Icon name="lucide:rotate-ccw" size="14" />
      </button>
    </div>

    <div class="flex items-end justify-between gap-1" :class="!enabled ? 'pointer-events-none opacity-40' : ''">
      <div v-for="(freq, i) in EQ_BANDS" :key="freq" class="flex flex-col items-center gap-1">
        <span class="text-[9px] tabular-nums text-muted">{{ gains[i] > 0 ? '+' : '' }}{{ gains[i] }}</span>
        <input
          type="range"
          orient="vertical"
          :min="EQ_GAIN_MIN" :max="EQ_GAIN_MAX" step="1"
          :value="gains[i]"
          class="h-24 w-4 accent-accent"
          style="writing-mode: vertical-lr; direction: rtl"
          @input="onGain(i, $event)"
        />
        <span class="text-[9px] text-muted">{{ bandLabel(freq) }}</span>
      </div>
    </div>
  </div>
</template>
