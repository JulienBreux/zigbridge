<script setup lang="ts">
import { computed } from 'vue'
import type { BridgeStatus } from '@/types'
import { formatUptime, formatHex } from '@/utils/formatters'

const props = defineProps<{
  status: BridgeStatus | null
}>()

const emit = defineEmits<{
  (e: 'refresh'): void
}>()

const uptimeStr = computed(() => formatUptime(props.status?.uptime_seconds || 0))

const panIdHex = computed(() => {
  if (!props.status?.coordinator?.pan_id) return '--'
  return formatHex(props.status.coordinator.pan_id)
})

const isOnline = computed(() => props.status?.coordinator?.online ?? false)
const isConnected = computed(() => props.status?.connected ?? false)

const appVersion = computed(() => {
  if (!props.status?.version) return 'dev'
  return props.status.version.startsWith('v') ? props.status.version : `v${props.status.version}`
})

const permitJoinText = computed(() => {
  const rem = props.status?.permit_join_remaining || 0
  return rem > 0 ? `Enabled (${rem}s remaining)` : 'Disabled'
})
</script>

<template>
  <div class="space-y-6">
    <!-- Header / Actions bar -->
    <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 p-4 rounded-xl shadow-xs">
      <div>
        <h2 class="text-sm font-bold text-slate-800 dark:text-slate-100">System Diagnostics</h2>
        <p class="text-xs text-slate-500 dark:text-slate-400 mt-0.5">
          Coordinator hardware telemetry, Zigbee radio parameters, and bridge transport status.
        </p>
      </div>
      <div>
        <button
          type="button"
          class="px-3.5 py-1.5 text-xs font-semibold text-slate-700 dark:text-slate-300 bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 rounded-lg transition-colors"
          @click="emit('refresh')"
        >
          Refresh Diagnostics
        </button>
      </div>
    </div>

    <!-- 4 Overview Metric Cards -->
    <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
      <!-- Card 1: Coordinator Adapter -->
      <div class="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl p-4 shadow-xs">
        <div class="flex items-center justify-between">
          <span class="text-xs font-medium text-slate-500 dark:text-slate-400">Coordinator Adapter</span>
          <span class="px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wider rounded bg-purple-50 text-purple-700 dark:bg-purple-950/80 dark:text-purple-300 border border-purple-200 dark:border-purple-800">
            {{ status?.coordinator?.type || 'Coordinator' }}
          </span>
        </div>
        <div class="text-base font-bold text-slate-800 dark:text-slate-100 mt-2">
          {{ status?.coordinator?.version || 'Zigbee 3.0' }}
        </div>
        <div class="font-mono text-[11px] text-slate-400 dark:text-slate-500 mt-0.5 truncate">
          {{ status?.coordinator?.ieee || '--:--:--:--:--:--:--:--' }}
        </div>
      </div>

      <!-- Card 2: Zigbee Radio -->
      <div class="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl p-4 shadow-xs">
        <div class="flex items-center justify-between">
          <span class="text-xs font-medium text-slate-500 dark:text-slate-400">Zigbee Radio</span>
          <span class="px-2 py-0.5 text-[10px] font-semibold rounded bg-blue-50 text-blue-700 dark:bg-blue-950/80 dark:text-blue-300 border border-blue-200 dark:border-blue-800">
            2.4 GHz
          </span>
        </div>
        <div class="text-base font-bold text-slate-800 dark:text-slate-100 mt-2">
          CH {{ status?.coordinator?.channel ?? '--' }} | PAN {{ panIdHex }}
        </div>
        <div class="font-mono text-[11px] text-slate-400 dark:text-slate-500 mt-0.5 truncate">
          Ext PAN: {{ status?.coordinator?.ext_pan_id || '--' }}
        </div>
      </div>

      <!-- Card 3: Transport Link -->
      <div class="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl p-4 shadow-xs">
        <div class="flex items-center justify-between">
          <span class="text-xs font-medium text-slate-500 dark:text-slate-400">Transport Link</span>
          <span
            :class="[
              'px-2 py-0.5 text-[10px] font-semibold rounded border',
              isConnected
                ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/80 dark:text-emerald-300 border-emerald-200 dark:border-emerald-800'
                : 'bg-rose-50 text-rose-700 dark:bg-rose-950/80 dark:text-rose-300 border-rose-200 dark:border-rose-800'
            ]"
          >
            TCP Socket
          </span>
        </div>
        <div class="text-base font-bold text-slate-800 dark:text-slate-100 mt-2">
          {{ isConnected ? 'Active Link' : (status?.transport_status || 'Disconnected') }}
        </div>
        <div class="text-[11px] text-slate-500 dark:text-slate-400 mt-0.5">
          NVRAM: <strong class="text-emerald-600 dark:text-emerald-400">Preserved</strong>
        </div>
      </div>

      <!-- Card 4: Bridge Uptime -->
      <div class="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl p-4 shadow-xs">
        <div class="flex items-center justify-between">
          <span class="text-xs font-medium text-slate-500 dark:text-slate-400">Bridge Uptime</span>
          <span class="px-2 py-0.5 text-[10px] font-semibold rounded bg-emerald-50 text-emerald-700 dark:bg-emerald-950/80 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800">
            Operational
          </span>
        </div>
        <div class="text-base font-bold text-slate-800 dark:text-slate-100 mt-2 font-mono">
          {{ uptimeStr }}
        </div>
        <div class="text-[11px] text-slate-500 dark:text-slate-400 mt-0.5">
          {{ status?.device_count || 0 }} Devices · {{ status?.binding_count || 0 }} Bindings
        </div>
      </div>
    </div>

    <!-- Detailed Technical Breakdown Cards -->
    <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
      <!-- Detailed Card 1: Coordinator & Radio Telemetry -->
      <div class="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl p-4 shadow-xs">
        <div class="flex items-center justify-between pb-3 border-b border-slate-100 dark:border-slate-800">
          <h3 class="text-xs font-semibold uppercase tracking-wider text-slate-700 dark:text-slate-300">
            Coordinator &amp; Radio Telemetry
          </h3>
          <span
            :class="[
              'px-2 py-0.5 text-[11px] font-medium rounded-full border',
              isOnline
                ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/80 dark:text-emerald-300 border-emerald-200 dark:border-emerald-800'
                : 'bg-rose-50 text-rose-700 dark:bg-rose-950/80 dark:text-rose-300 border-rose-200 dark:border-rose-800'
            ]"
          >
            {{ isOnline ? 'Ready' : 'Offline' }}
          </span>
        </div>

        <div class="mt-3 divide-y divide-slate-100 dark:divide-slate-800 text-xs">
          <div class="flex justify-between py-2">
            <span class="text-slate-500 dark:text-slate-400">Adapter Type / Driver</span>
            <span class="font-medium text-slate-800 dark:text-slate-200">{{ status?.coordinator?.type || '--' }}</span>
          </div>
          <div class="flex justify-between py-2">
            <span class="text-slate-500 dark:text-slate-400">Firmware / Version</span>
            <span class="font-mono text-slate-800 dark:text-slate-200">{{ status?.coordinator?.version || '--' }}</span>
          </div>
          <div class="flex justify-between py-2">
            <span class="text-slate-500 dark:text-slate-400">Coordinator IEEE (EUI-64)</span>
            <span class="font-mono text-slate-800 dark:text-slate-200">{{ status?.coordinator?.ieee || '--' }}</span>
          </div>
          <div class="flex justify-between py-2">
            <span class="text-slate-500 dark:text-slate-400">Radio Channel</span>
            <span class="text-slate-800 dark:text-slate-200">
              {{ status?.coordinator?.channel ? `Channel ${status.coordinator.channel} (2.4 GHz)` : '--' }}
            </span>
          </div>
          <div class="flex justify-between py-2">
            <span class="text-slate-500 dark:text-slate-400">16-bit PAN Identifier</span>
            <span class="font-mono text-slate-800 dark:text-slate-200">{{ panIdHex }}</span>
          </div>
          <div class="flex justify-between py-2">
            <span class="text-slate-500 dark:text-slate-400">Extended PAN Identifier</span>
            <span class="font-mono text-slate-800 dark:text-slate-200">{{ status?.coordinator?.ext_pan_id || '--' }}</span>
          </div>
        </div>
      </div>

      <!-- Detailed Card 2: Host & Service Connectivity -->
      <div class="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl p-4 shadow-xs">
        <div class="flex items-center justify-between pb-3 border-b border-slate-100 dark:border-slate-800">
          <h3 class="text-xs font-semibold uppercase tracking-wider text-slate-700 dark:text-slate-300">
            Host &amp; Service Connectivity
          </h3>
          <span
            :class="[
              'px-2 py-0.5 text-[11px] font-medium rounded-full border',
              isConnected
                ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/80 dark:text-emerald-300 border-emerald-200 dark:border-emerald-800'
                : 'bg-rose-50 text-rose-700 dark:bg-rose-950/80 dark:text-rose-300 border-rose-200 dark:border-rose-800'
            ]"
          >
            {{ isConnected ? 'Active' : 'Disconnected' }}
          </span>
        </div>

        <div class="mt-3 divide-y divide-slate-100 dark:divide-slate-800 text-xs">
          <div class="flex justify-between py-2">
            <span class="text-slate-500 dark:text-slate-400">Physical Transport</span>
            <span class="text-slate-800 dark:text-slate-200">
              {{ isConnected ? `Connected (${status?.transport_status || 'Active'})` : (status?.transport_status || 'Disconnected') }}
            </span>
          </div>
          <div class="flex justify-between py-2">
            <span class="text-slate-500 dark:text-slate-400">MQTT Connection</span>
            <span class="text-slate-800 dark:text-slate-200">
              {{ status?.mqtt_connected ? 'Connected' : 'Offline / Disabled' }}
            </span>
          </div>
          <div class="flex justify-between py-2">
            <span class="text-slate-500 dark:text-slate-400">Zigbridge Version</span>
            <span class="font-mono text-slate-800 dark:text-slate-200">{{ appVersion }}</span>
          </div>
          <div class="flex justify-between py-2">
            <span class="text-slate-500 dark:text-slate-400">Git Commit</span>
            <span class="font-mono text-slate-800 dark:text-slate-200">{{ status?.commit || 'unknown' }}</span>
          </div>
          <div class="flex justify-between py-2">
            <span class="text-slate-500 dark:text-slate-400">Bridge System Uptime</span>
            <span class="font-mono text-slate-800 dark:text-slate-200">{{ uptimeStr }}</span>
          </div>
          <div class="flex justify-between py-2">
            <span class="text-slate-500 dark:text-slate-400">Permit Join Status</span>
            <span class="text-slate-800 dark:text-slate-200">{{ permitJoinText }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
