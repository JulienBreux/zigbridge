<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    label: string
    status: 'connected' | 'disconnected' | 'warning' | 'info' | 'purple'
    title?: string
    customText?: string
  }>(),
  {
    title: '',
    customText: '',
  }
)

const colorClasses = computed(() => {
  switch (props.status) {
    case 'connected':
      return 'bg-emerald-500/15 text-emerald-500 border-emerald-500/30 dark:bg-emerald-500/15 dark:text-emerald-400 dark:border-emerald-500/30'
    case 'warning':
      return 'bg-amber-500/15 text-amber-500 border-amber-500/30 dark:bg-amber-500/15 dark:text-amber-400 dark:border-amber-500/30'
    case 'info':
      return 'bg-blue-500/15 text-blue-500 border-blue-500/30 dark:bg-blue-500/15 dark:text-blue-400 dark:border-blue-500/30'
    case 'purple':
      return 'bg-purple-500/15 text-purple-500 border-purple-500/30 dark:bg-purple-500/15 dark:text-purple-400 dark:border-purple-500/30'
    case 'disconnected':
    default:
      return 'bg-rose-500/15 text-rose-500 border-rose-500/30 dark:bg-rose-500/15 dark:text-rose-400 dark:border-rose-500/30'
  }
})
</script>

<template>
  <div
    class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-semibold border transition-colors select-none"
    :class="colorClasses"
    :title="title || `${label}: ${status}`"
  >
    <span class="w-2 h-2 rounded-full bg-current"></span>
    <span>{{ customText || label }}</span>
  </div>
</template>
