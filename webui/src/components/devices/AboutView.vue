<script setup lang="ts">
import { computed } from 'vue'
import type { Device, DeviceDefinition } from '@/types'
import { inferDeviceType } from '@/utils/formatters'
import { clusterName } from '@/utils/clusters'

const props = defineProps<{
  device: Device
  definition?: DeviceDefinition | null
}>()

const typeInfo = computed(() => inferDeviceType(props.device))
const isBattery = computed(() => props.device.battery !== undefined && props.device.battery !== null)

const lqi = computed(() => props.device.lqi || 0)
const lqiPercent = computed(() => Math.min(100, Math.round((lqi.value / 255) * 100)))

const lastSeenText = computed(() => {
  if (!props.device.last_seen) return 'Never'
  try {
    return new Date(props.device.last_seen).toLocaleString()
  } catch {
    return 'Never'
  }
})

const nwkHex = computed(() => {
  return '0x' + (props.device.nwk || 0).toString(16).toUpperCase().padStart(4, '0')
})

const voltageText = computed(() => {
  if (props.device.voltage) {
    return `${(props.device.voltage / 1000).toFixed(2)} V (${props.device.voltage} mV)`
  }
  if (props.device.state && props.device.state.voltage !== undefined) {
    return `${props.device.state.voltage} V`
  }
  return 'N/A'
})

const endpoints = computed(() => {
  if (props.definition?.device?.endpoints && props.definition.device.endpoints.length > 0) {
    return props.definition.device.endpoints
  }
  if (props.device.endpoints && props.device.endpoints.length > 0) {
    return props.device.endpoints.map(ep => ({
      endpoint: ep,
      input_clusters: props.device.input_clusters || [],
      output_clusters: props.device.output_clusters || []
    }))
  }
  return []
})
</script>

<template>
  <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
    <!-- Card 1: Hardware Identity -->
    <div class="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl p-4 shadow-xs">
      <div class="flex items-center justify-between pb-3 border-b border-slate-100 dark:border-slate-800">
        <h3 class="text-xs font-semibold uppercase tracking-wider text-slate-700 dark:text-slate-300">
          Hardware Identity
        </h3>
        <span class="px-2 py-0.5 text-[11px] font-medium rounded-full bg-blue-50 text-blue-700 dark:bg-blue-950/80 dark:text-blue-300 border border-blue-200 dark:border-blue-800">
          {{ typeInfo.name }}
        </span>
      </div>

      <div class="mt-3 divide-y divide-slate-100 dark:divide-slate-800 text-xs">
        <div class="flex justify-between py-2">
          <span class="text-slate-500 dark:text-slate-400">Friendly Name</span>
          <span class="font-medium text-slate-800 dark:text-slate-200">{{ device.friendly_name || device.ieee }}</span>
        </div>
        <div class="flex justify-between py-2">
          <span class="text-slate-500 dark:text-slate-400">Model Identifier</span>
          <span class="font-mono text-slate-800 dark:text-slate-200">{{ device.model || definition?.device?.model || '--' }}</span>
        </div>
        <div class="flex justify-between py-2">
          <span class="text-slate-500 dark:text-slate-400">Vendor / Manufacturer</span>
          <span class="text-slate-800 dark:text-slate-200">{{ device.manufacturer || definition?.device?.vendor || '--' }}</span>
        </div>
        <div class="flex justify-between py-2">
          <span class="text-slate-500 dark:text-slate-400">Description</span>
          <span class="text-slate-800 dark:text-slate-200 text-right max-w-[240px]">
            {{ definition?.device?.description || `${typeInfo.name} connected to Zigbee mesh` }}
          </span>
        </div>
        <div class="flex justify-between py-2">
          <span class="text-slate-500 dark:text-slate-400">Supported Models</span>
          <span class="font-mono text-slate-800 dark:text-slate-200 text-right max-w-[240px]">
            {{ definition?.device?.zigbee_models?.join(', ') || device.model || '--' }}
          </span>
        </div>
      </div>
    </div>

    <!-- Card 2: Zigbee Mesh Telemetry -->
    <div class="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl p-4 shadow-xs">
      <div class="flex items-center justify-between pb-3 border-b border-slate-100 dark:border-slate-800">
        <h3 class="text-xs font-semibold uppercase tracking-wider text-slate-700 dark:text-slate-300">
          Zigbee Mesh Telemetry
        </h3>
        <span class="px-2 py-0.5 text-[11px] font-medium rounded-full bg-emerald-50 text-emerald-700 dark:bg-emerald-950/80 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800">
          Operational
        </span>
      </div>

      <div class="mt-3 divide-y divide-slate-100 dark:divide-slate-800 text-xs">
        <div class="flex justify-between py-2">
          <span class="text-slate-500 dark:text-slate-400">IEEE Address (EUI-64)</span>
          <span class="font-mono text-slate-800 dark:text-slate-200">{{ device.ieee }}</span>
        </div>
        <div class="flex justify-between py-2">
          <span class="text-slate-500 dark:text-slate-400">16-bit Network Address (NWK)</span>
          <span class="font-mono text-slate-800 dark:text-slate-200">{{ nwkHex }}</span>
        </div>
        <div class="flex justify-between items-center py-2">
          <span class="text-slate-500 dark:text-slate-400">Signal Quality (LQI)</span>
          <div class="flex items-center space-x-2">
            <span class="font-mono text-slate-800 dark:text-slate-200">{{ lqi }} / 255 ({{ lqiPercent }}%)</span>
            <div class="w-16 bg-slate-200 dark:bg-slate-700 rounded-full h-1.5 overflow-hidden">
              <div class="bg-blue-500 h-1.5 rounded-full" :style="{ width: `${lqiPercent}%` }"></div>
            </div>
          </div>
        </div>
        <div class="flex justify-between py-2">
          <span class="text-slate-500 dark:text-slate-400">Last Seen</span>
          <span class="text-slate-800 dark:text-slate-200">{{ lastSeenText }}</span>
        </div>
      </div>
    </div>

    <!-- Card 3: Power Configuration -->
    <div class="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl p-4 shadow-xs">
      <div class="flex items-center justify-between pb-3 border-b border-slate-100 dark:border-slate-800">
        <h3 class="text-xs font-semibold uppercase tracking-wider text-slate-700 dark:text-slate-300">
          Power Configuration
        </h3>
        <span
          :class="[
            'px-2 py-0.5 text-[11px] font-medium rounded-full border',
            isBattery
              ? 'bg-blue-50 text-blue-700 dark:bg-blue-950/80 dark:text-blue-300 border-blue-200 dark:border-blue-800'
              : 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/80 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800'
          ]"
        >
          {{ isBattery ? 'Battery-Powered' : 'Mains' }}
        </span>
      </div>

      <div class="mt-3 divide-y divide-slate-100 dark:divide-slate-800 text-xs">
        <div class="flex justify-between py-2">
          <span class="text-slate-500 dark:text-slate-400">Power Source</span>
          <span class="text-slate-800 dark:text-slate-200">
            {{ device.power_source || (isBattery ? 'Battery' : 'Mains (AC)') }}
          </span>
        </div>
        <div class="flex justify-between py-2">
          <span class="text-slate-500 dark:text-slate-400">Battery Level</span>
          <span class="text-slate-800 dark:text-slate-200">
            {{ isBattery ? `${device.battery}%` : 'N/A (Mains-powered)' }}
          </span>
        </div>
        <div class="flex justify-between py-2">
          <span class="text-slate-500 dark:text-slate-400">Battery Voltage</span>
          <span class="text-slate-800 dark:text-slate-200">{{ voltageText }}</span>
        </div>
      </div>
    </div>

    <!-- Card 4: Endpoints & Clusters -->
    <div class="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl p-4 shadow-xs">
      <div class="flex items-center justify-between pb-3 border-b border-slate-100 dark:border-slate-800">
        <h3 class="text-xs font-semibold uppercase tracking-wider text-slate-700 dark:text-slate-300">
          Endpoints &amp; Clusters
        </h3>
        <span class="px-2 py-0.5 text-[11px] font-medium rounded-full bg-purple-50 text-purple-700 dark:bg-purple-950/80 dark:text-purple-300 border border-purple-200 dark:border-purple-800">
          {{ endpoints.length }} Endpoint{{ endpoints.length !== 1 ? 's' : '' }}
        </span>
      </div>

      <div class="mt-3 space-y-3">
        <div v-if="endpoints.length === 0" class="text-slate-400 dark:text-slate-500 text-xs py-2">
          No endpoints registered.
        </div>
        <div
          v-for="ep in endpoints"
          :key="ep.endpoint"
          class="p-3 bg-slate-50 dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/80 rounded-lg text-xs"
        >
          <div class="font-semibold text-slate-800 dark:text-slate-200 mb-2">
            Endpoint {{ ep.endpoint }}
          </div>

          <div class="space-y-1.5">
            <div>
              <span class="text-[11px] font-medium text-slate-500 dark:text-slate-400 block mb-1">
                Input / Server Clusters:
              </span>
              <div class="flex flex-wrap gap-1">
                <span
                  v-for="c in ep.input_clusters || []"
                  :key="'in-' + c"
                  class="px-1.5 py-0.5 text-[10px] rounded bg-white dark:bg-slate-900 text-slate-700 dark:text-slate-300 border border-slate-200 dark:border-slate-700"
                >
                  {{ clusterName(c) }}
                </span>
                <span v-if="!ep.input_clusters || ep.input_clusters.length === 0" class="text-[10px] text-slate-400">
                  None
                </span>
              </div>
            </div>

            <div>
              <span class="text-[11px] font-medium text-slate-500 dark:text-slate-400 block mb-1">
                Output / Client Clusters:
              </span>
              <div class="flex flex-wrap gap-1">
                <span
                  v-for="c in ep.output_clusters || []"
                  :key="'out-' + c"
                  class="px-1.5 py-0.5 text-[10px] rounded bg-blue-50 dark:bg-blue-950/60 text-blue-700 dark:text-blue-300 border border-blue-200 dark:border-blue-800"
                >
                  {{ clusterName(c) }}
                </span>
                <span v-if="!ep.output_clusters || ep.output_clusters.length === 0" class="text-[10px] text-slate-400">
                  None
                </span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
