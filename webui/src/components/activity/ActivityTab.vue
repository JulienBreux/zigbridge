<script setup lang="ts">
import type { ActivityItem, RawLogItem } from '@/types'
import { useMode } from '@/composables/useMode'
import RawLogConsole from '@/components/activity/RawLogConsole.vue'

defineProps<{
  activityItems: ActivityItem[]
  rawLogs: RawLogItem[]
}>()

const emit = defineEmits<{
  (e: 'clear'): void
}>()

const { isAdvanced } = useMode()

function getIconEmoji(type: ActivityItem['iconType']): string {
  switch (type) {
    case 'light':
      return '💡'
    case 'motion':
      return '🏃'
    case 'switch':
      return '🖲️'
    case 'join':
      return '✨'
    case 'system':
      return '⚙️'
    default:
      return '📡'
  }
}

function getIconBadgeClass(iconClass: string): string {
  switch (iconClass) {
    case 'join':
      return 'bg-emerald-50 dark:bg-emerald-950/80 text-emerald-600 dark:text-emerald-400 border-emerald-200 dark:border-emerald-800'
    case 'light':
      return 'bg-amber-50 dark:bg-amber-950/80 text-amber-600 dark:text-amber-400 border-amber-200 dark:border-amber-800'
    case 'switch':
      return 'bg-blue-50 dark:bg-blue-950/80 text-blue-600 dark:text-blue-400 border-blue-200 dark:border-blue-800'
    default:
      return 'bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-400 border-slate-200 dark:border-slate-700'
  }
}
</script>

<template>
  <div class="space-y-6">
    <!-- Header / Actions bar -->
    <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 p-4 rounded-xl shadow-xs">
      <div>
        <h2 class="text-sm font-bold text-slate-800 dark:text-slate-100">Network Activity</h2>
        <p class="text-xs text-slate-500 dark:text-slate-400 mt-0.5">
          Real-time feed of device interactions, state changes, and network announcements.
        </p>
      </div>
      <div>
        <button
          type="button"
          class="px-3 py-1.5 text-xs font-medium text-slate-700 dark:text-slate-300 bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 rounded-md transition-colors"
          @click="emit('clear')"
        >
          Clear Activity
        </button>
      </div>
    </div>

    <!-- Friendly Activity Timeline -->
    <div class="space-y-2">
      <template v-if="activityItems.length > 0">
        <div
          v-for="item in activityItems"
          :key="item.id"
          class="flex items-center justify-between p-3.5 bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl shadow-xs gap-3 hover:bg-slate-50/60 dark:hover:bg-slate-800/40 transition-colors"
        >
          <div class="flex items-center space-x-3 min-w-0">
            <div :class="['w-8 h-8 rounded-lg flex items-center justify-center text-base border shrink-0', getIconBadgeClass(item.iconClass)]">
              {{ getIconEmoji(item.iconType) }}
            </div>
            <div class="min-w-0">
              <div class="text-xs font-semibold text-slate-800 dark:text-slate-100 truncate">
                {{ item.title }}
              </div>
              <div class="text-[11px] text-slate-500 dark:text-slate-400 truncate">
                {{ item.meta }}
              </div>
            </div>
          </div>

          <div class="text-[11px] font-mono text-slate-400 dark:text-slate-500 shrink-0">
            {{ item.time }}
          </div>
        </div>
      </template>
      <div
        v-else
        class="p-10 text-center bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl text-xs text-slate-400 dark:text-slate-500"
      >
        Listening for network activity... Real-time events will appear here.
      </div>
    </div>

    <!-- Raw Technical Log Stream (Only visible in Advanced mode) -->
    <div v-if="isAdvanced" class="space-y-2 pt-2">
      <div class="flex items-center justify-between">
        <h3 class="text-xs font-semibold uppercase tracking-wider text-slate-500 dark:text-slate-400">
          Raw ZCL &amp; WebSocket Log Stream
        </h3>
        <span class="text-[11px] text-slate-400 dark:text-slate-500">Filtered for advanced inspection</span>
      </div>
      <RawLogConsole :logs="rawLogs" />
    </div>
  </div>
</template>
