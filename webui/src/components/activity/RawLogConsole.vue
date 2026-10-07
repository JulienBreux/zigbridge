<script setup lang="ts">
import type { RawLogItem } from '@/types'

defineProps<{
  logs: RawLogItem[]
}>()

function getTypeColor(typeClass: RawLogItem['typeClass']): string {
  switch (typeClass) {
    case 'join':
      return 'text-emerald-400'
    case 'binding':
      return 'text-purple-400'
    case 'warn':
      return 'text-amber-400'
    case 'error':
      return 'text-rose-400'
    default:
      return 'text-blue-400'
  }
}
</script>

<template>
  <div class="rounded-xl overflow-hidden border border-slate-800 bg-slate-950 shadow-inner">
    <div class="flex items-center justify-between px-3.5 py-2 bg-slate-900 border-b border-slate-800 text-[11px] font-mono text-slate-400">
      <span>RAW ZCL &amp; WEBSOCKET LOG STREAM</span>
      <span class="text-[10px] text-slate-500">{{ logs.length }} events</span>
    </div>

    <div class="p-3 font-mono text-xs max-h-96 overflow-y-auto space-y-1 select-text">
      <div
        v-for="log in logs"
        :key="log.id"
        class="flex items-start space-x-2 leading-relaxed"
      >
        <span class="text-slate-500 shrink-0">[{{ log.time }}]</span>
        <span :class="['font-semibold shrink-0 uppercase', getTypeColor(log.typeClass)]">
          [{{ log.type }}]
        </span>
        <span class="text-slate-300 break-all">
          {{ log.message }}
        </span>
      </div>
      <div v-if="logs.length === 0" class="text-slate-600 text-center py-6">
        No log events recorded yet.
      </div>
    </div>
  </div>
</template>
