<script setup lang="ts">
import { computed } from 'vue'
import type { Device } from '@/types'
import { inferDeviceType } from '@/utils/formatters'
import { clusterName } from '@/utils/clusters'

const props = defineProps<{
  device: Device
  isAdvanced: boolean
}>()

const emit = defineEmits<{
  (e: 'select', ieee: string): void
}>()

const typeInfo = computed(() => inferDeviceType(props.device))

const signal = computed(() => {
  const lqi = props.device.lqi || 0
  if (lqi >= 180) {
    return { label: '● Excellent', class: 'text-emerald-700 bg-emerald-50 dark:text-emerald-300 dark:bg-emerald-950/60 border-emerald-200 dark:border-emerald-800' }
  } else if (lqi >= 110) {
    return { label: '● Good', class: 'text-blue-700 bg-blue-50 dark:text-blue-300 dark:bg-blue-950/60 border-blue-200 dark:border-blue-800' }
  } else if (lqi >= 50) {
    return { label: '● Fair', class: 'text-amber-700 bg-amber-50 dark:text-amber-300 dark:bg-amber-950/60 border-amber-200 dark:border-amber-800' }
  }
  return { label: '● Poor', class: 'text-rose-700 bg-rose-50 dark:text-rose-300 dark:bg-rose-950/60 border-rose-200 dark:border-rose-800' }
})

const lqiPercent = computed(() => Math.min(100, Math.round(((props.device.lqi || 0) / 255) * 100)))

const lastSeenText = computed(() => {
  if (!props.device.last_seen) return 'Never'
  try {
    return new Date(props.device.last_seen).toLocaleTimeString()
  } catch {
    return 'Never'
  }
})

const nwkHex = computed(() => {
  return '0x' + (props.device.nwk || 0).toString(16).toUpperCase().padStart(4, '0')
})
</script>

<template>
  <tr
    class="border-b border-slate-100 dark:border-slate-800/80 hover:bg-slate-50/80 dark:hover:bg-slate-800/50 cursor-pointer transition-colors"
    @click="emit('select', device.ieee)"
  >
    <!-- Device Friendly Name & Icon -->
    <td class="px-4 py-3.5 whitespace-nowrap">
      <div class="flex items-center space-x-3">
        <div class="w-9 h-9 flex items-center justify-center text-lg rounded-lg bg-slate-100 dark:bg-slate-800 text-slate-700 dark:text-slate-200 shrink-0">
          {{ typeInfo.icon }}
        </div>
        <div>
          <div class="text-xs font-semibold text-slate-800 dark:text-slate-100">
            {{ device.friendly_name || device.ieee }}
          </div>
          <div class="text-[11px] text-slate-500 dark:text-slate-400 truncate max-w-[200px]">
            {{ device.model || 'Generic Device' }}
          </div>
        </div>
      </div>
    </td>

    <!-- Device Inferred Type -->
    <td class="px-4 py-3.5 whitespace-nowrap text-xs text-slate-600 dark:text-slate-300">
      {{ typeInfo.name }}
    </td>

    <!-- Status / Power -->
    <td class="px-4 py-3.5 whitespace-nowrap text-xs">
      <template v-if="device.state">
        <span
          v-if="typeof device.state.state === 'string'"
          :class="[
            'px-2 py-0.5 rounded text-[11px] font-semibold uppercase tracking-wider',
            device.state.state.toUpperCase() === 'ON'
              ? 'bg-emerald-100 text-emerald-800 dark:bg-emerald-950/80 dark:text-emerald-300 border border-emerald-300 dark:border-emerald-800'
              : 'bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-400 border border-slate-300 dark:border-slate-700'
          ]"
        >
          {{ device.state.state }}
        </span>
        <span
          v-else-if="device.state.occupancy !== undefined"
          :class="[
            'px-2 py-0.5 rounded text-[11px] font-medium',
            device.state.occupancy
              ? 'bg-purple-100 text-purple-800 dark:bg-purple-950/80 dark:text-purple-300'
              : 'text-slate-400 dark:text-slate-500'
          ]"
        >
          {{ device.state.occupancy ? 'Motion' : 'Clear' }}
        </span>
        <span
          v-else-if="device.state.temperature !== undefined"
          class="px-2 py-0.5 rounded text-[11px] font-medium bg-blue-50 text-blue-700 dark:bg-blue-950/80 dark:text-blue-300 border border-blue-200 dark:border-blue-800"
        >
          {{ device.state.temperature }}°C
        </span>
        <span v-else class="text-slate-400 dark:text-slate-500 text-xs">Idle</span>
      </template>
      <span v-else class="text-slate-400 dark:text-slate-500 text-xs">Idle</span>
    </td>

    <!-- Battery -->
    <td class="px-4 py-3.5 whitespace-nowrap text-xs">
      <span
        v-if="device.battery !== undefined && device.battery !== null"
        class="inline-flex items-center px-2 py-0.5 rounded text-[11px] font-medium bg-slate-100 dark:bg-slate-800 text-slate-700 dark:text-slate-300 border border-slate-200 dark:border-slate-700"
      >
        🔋 {{ device.battery }}%
      </span>
      <span v-else class="text-slate-400 dark:text-slate-500 text-xs flex items-center space-x-1">
        <span>⚡</span> <span>Mains</span>
      </span>
    </td>

    <!-- Signal Strength -->
    <td class="px-4 py-3.5 whitespace-nowrap text-xs">
      <span :class="['inline-flex items-center px-2 py-0.5 rounded text-[11px] font-medium border', signal.class]">
        {{ signal.label }}
      </span>
    </td>

    <!-- Advanced: IEEE Address -->
    <td v-if="isAdvanced" class="px-4 py-3.5 whitespace-nowrap font-mono text-xs text-slate-600 dark:text-slate-300">
      {{ device.ieee }}
    </td>

    <!-- Advanced: NWK Address -->
    <td v-if="isAdvanced" class="px-4 py-3.5 whitespace-nowrap font-mono text-xs text-slate-500 dark:text-slate-400">
      {{ nwkHex }}
    </td>

    <!-- Advanced: Endpoints & Clusters -->
    <td v-if="isAdvanced" class="px-4 py-3.5 text-xs text-slate-600 dark:text-slate-300 max-w-xs">
      <div class="text-[11px] font-medium text-slate-500 dark:text-slate-400 mb-1">
        EP: [{{ (device.endpoints || []).join(', ') }}]
      </div>
      <div class="flex flex-wrap gap-1">
        <span
          v-for="c in device.input_clusters || []"
          :key="'in-' + c"
          title="Server / Input"
          class="inline-block px-1.5 py-0.5 text-[10px] rounded bg-slate-100 dark:bg-slate-800 text-slate-700 dark:text-slate-300 border border-slate-200 dark:border-slate-700"
        >
          {{ clusterName(c) }}
        </span>
        <span
          v-for="c in device.output_clusters || []"
          :key="'out-' + c"
          title="Client / Output"
          class="inline-block px-1.5 py-0.5 text-[10px] rounded bg-blue-50 dark:bg-blue-950/60 text-blue-700 dark:text-blue-300 border border-blue-200 dark:border-blue-800"
        >
          {{ clusterName(c) }}
        </span>
      </div>
    </td>

    <!-- Advanced: Numerical LQI -->
    <td v-if="isAdvanced" class="px-4 py-3.5 whitespace-nowrap text-xs">
      <div class="font-mono text-[11px] text-slate-600 dark:text-slate-300">
        {{ device.lqi || 0 }} / 255
      </div>
      <div class="w-20 bg-slate-200 dark:bg-slate-700 rounded-full h-1.5 mt-1 overflow-hidden">
        <div class="bg-blue-500 h-1.5 rounded-full" :style="{ width: `${lqiPercent}%` }"></div>
      </div>
    </td>

    <!-- Advanced: Last Seen -->
    <td v-if="isAdvanced" class="px-4 py-3.5 whitespace-nowrap text-[11px] text-slate-400 dark:text-slate-500">
      {{ lastSeenText }}
    </td>
  </tr>
</template>
