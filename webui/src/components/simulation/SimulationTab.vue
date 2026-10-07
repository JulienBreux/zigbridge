<script setup lang="ts">
import type { VirtualDeviceSummary } from '@/types'
import { useZigbridgeApi } from '@/composables/useZigbridgeApi'

defineProps<{
  devices: VirtualDeviceSummary[]
}>()

const emit = defineEmits<{
  (e: 'refresh'): void
  (e: 'spawn'): void
}>()

const api = useZigbridgeApi()

async function triggerAction(ieee: string, action: string) {
  try {
    await api.triggerVirtualDeviceAction(ieee, action)
    emit('refresh')
  } catch (err: any) {
    alert(`Failed to trigger action '${action}': ${err?.message || 'Unknown error'}`)
  }
}

async function sendBattery(ieee: string, battery: number, voltage: number) {
  try {
    await api.sendVirtualDeviceTelemetry(ieee, { battery, voltage })
    emit('refresh')
  } catch (err: any) {
    alert(`Failed to report battery: ${err?.message || 'Unknown error'}`)
  }
}

async function sendSensors(ieee: string, temperature: number, humidity: number) {
  try {
    await api.sendVirtualDeviceTelemetry(ieee, { temperature, humidity })
    emit('refresh')
  } catch (err: any) {
    alert(`Failed to report telemetry: ${err?.message || 'Unknown error'}`)
  }
}

function formatNwk(nwk: number): string {
  if (nwk == null || isNaN(nwk)) return '--'
  return `0x${nwk.toString(16).toUpperCase().padStart(4, '0')}`
}
</script>

<template>
  <div class="space-y-6">
    <!-- Header / Actions bar -->
    <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 p-4 rounded-xl shadow-xs">
      <div>
        <h2 class="text-sm font-bold text-slate-800 dark:text-slate-100">Virtual Device Simulation Lab</h2>
        <p class="text-xs text-slate-500 dark:text-slate-400 mt-0.5">
          Simulate physical device joins, button clicks (single, double, long), and sensor telemetry without hardware.
        </p>
      </div>
      <div class="flex items-center space-x-2 shrink-0">
        <button
          type="button"
          class="px-3 py-1.5 text-xs font-semibold text-slate-700 dark:text-slate-300 bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 rounded-lg transition-colors"
          @click="emit('refresh')"
        >
          Refresh Lab
        </button>
        <button
          type="button"
          class="px-3.5 py-1.5 text-xs font-semibold text-white bg-blue-600 hover:bg-blue-700 rounded-lg shadow-xs transition-colors flex items-center space-x-1"
          @click="emit('spawn')"
        >
          <span>+ Spawn Simulated Device</span>
        </button>
      </div>
    </div>

    <!-- Virtual Devices Grid -->
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
      <template v-if="devices.length > 0">
        <div
          v-for="dev in devices"
          :key="dev.ieee"
          class="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl p-4 shadow-xs space-y-4"
        >
          <!-- Card Header -->
          <div class="flex items-start justify-between pb-3 border-b border-slate-100 dark:border-slate-800">
            <div>
              <div class="text-xs font-bold text-slate-800 dark:text-slate-100">
                {{ dev.vendor || 'Unknown' }} {{ dev.model || 'Generic' }}
              </div>
              <div v-if="dev.description" class="text-[11px] text-slate-500 dark:text-slate-400 mt-0.5">
                {{ dev.description }}
              </div>
            </div>
            <span class="px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wider rounded-full bg-purple-50 text-purple-700 dark:bg-purple-950/80 dark:text-purple-300 border border-purple-200 dark:border-purple-800">
              virtual
            </span>
          </div>

          <!-- IEEE & NWK -->
          <div class="flex items-center justify-between text-xs font-mono text-slate-500 dark:text-slate-400">
            <span>IEEE: <strong class="text-slate-700 dark:text-slate-200 font-semibold">{{ dev.ieee }}</strong></span>
            <span>NWK: <strong class="text-slate-700 dark:text-slate-200 font-semibold">{{ formatNwk(dev.nwk) }}</strong></span>
          </div>

          <!-- State Badges -->
          <div class="flex flex-wrap gap-1.5">
            <span
              v-if="dev.state?.battery != null"
              class="px-2 py-0.5 text-[11px] font-medium rounded bg-blue-50 text-blue-700 dark:bg-blue-950/80 dark:text-blue-300 border border-blue-200 dark:border-blue-800"
            >
              🔋 {{ dev.state.battery }}%
            </span>
            <span
              v-if="dev.state?.voltage != null"
              class="px-2 py-0.5 text-[11px] font-mono rounded bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-400 border border-slate-200 dark:border-slate-700"
            >
              {{ dev.state.voltage }} mV
            </span>
            <span
              v-if="dev.state?.action"
              class="px-2 py-0.5 text-[11px] font-medium rounded bg-purple-50 text-purple-700 dark:bg-purple-950/80 dark:text-purple-300 border border-purple-200 dark:border-purple-800"
            >
              ⚡ {{ dev.state.action }}
            </span>
            <span
              v-else
              class="px-2 py-0.5 text-[11px] font-medium rounded bg-slate-100 dark:bg-slate-800 text-slate-400 dark:text-slate-500"
            >
              ⚡ idle
            </span>
            <span
              v-if="dev.state?.temperature != null"
              class="px-2 py-0.5 text-[11px] font-medium rounded bg-emerald-50 text-emerald-700 dark:bg-emerald-950/80 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800"
            >
              🌡️ {{ dev.state.temperature.toFixed(1) }}°C
            </span>
            <span
              v-if="dev.state?.humidity != null"
              class="px-2 py-0.5 text-[11px] font-medium rounded bg-cyan-50 text-cyan-700 dark:bg-cyan-950/80 dark:text-cyan-300 border border-cyan-200 dark:border-cyan-800"
            >
              💧 {{ dev.state.humidity.toFixed(1) }}%
            </span>
          </div>

          <!-- Physical Button Press Simulation -->
          <div v-if="dev.actions && dev.actions.length > 0" class="pt-2">
            <div class="text-[11px] font-semibold uppercase tracking-wider text-slate-500 dark:text-slate-400 mb-1.5">
              Simulate Physical Button Press
            </div>
            <div class="flex flex-wrap gap-1.5">
              <button
                v-for="act in dev.actions"
                :key="act"
                type="button"
                class="px-2.5 py-1 text-xs font-medium rounded-md bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-200 transition-colors"
                @click="triggerAction(dev.ieee, act)"
              >
                {{ act === 'single' ? '👆 Single Click' : act === 'double' ? '✌️ Double Click' : act === 'long' ? '⏳ Long Press' : act }}
              </button>
            </div>
          </div>

          <!-- Battery Simulation -->
          <div v-if="dev.has_battery" class="pt-3 border-t border-slate-100 dark:border-slate-800">
            <div class="text-[11px] font-semibold uppercase tracking-wider text-slate-500 dark:text-slate-400 mb-1.5">
              Simulate Battery State
            </div>
            <div class="flex flex-wrap gap-1.5">
              <button
                type="button"
                class="px-2.5 py-1 text-xs font-medium rounded-md bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-200 transition-colors"
                @click="sendBattery(dev.ieee, 100, 3100)"
              >
                100% (3.1V)
              </button>
              <button
                type="button"
                class="px-2.5 py-1 text-xs font-medium rounded-md bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-200 transition-colors"
                @click="sendBattery(dev.ieee, 75, 2900)"
              >
                75% (2.9V)
              </button>
              <button
                type="button"
                class="px-2.5 py-1 text-xs font-medium rounded-md bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-200 transition-colors"
                @click="sendBattery(dev.ieee, 25, 2700)"
              >
                25% (2.7V)
              </button>
              <button
                type="button"
                class="px-2.5 py-1 text-xs font-medium rounded-md bg-rose-50 text-rose-700 hover:bg-rose-100 dark:bg-rose-950/60 dark:text-rose-300 dark:hover:bg-rose-900/60 border border-rose-200 dark:border-rose-900 transition-colors"
                @click="sendBattery(dev.ieee, 5, 2500)"
              >
                5% Low Batt
              </button>
            </div>
          </div>

          <!-- Climate Sensor Simulation -->
          <div v-if="dev.has_temperature || dev.has_humidity" class="pt-3 border-t border-slate-100 dark:border-slate-800">
            <div class="text-[11px] font-semibold uppercase tracking-wider text-slate-500 dark:text-slate-400 mb-1.5">
              Simulate Climate Telemetry
            </div>
            <div class="flex flex-wrap gap-1.5">
              <button
                type="button"
                class="px-2.5 py-1 text-xs font-medium rounded-md bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-200 transition-colors"
                @click="sendSensors(dev.ieee, 21.5, 48.0)"
              >
                Normal (21.5°C, 48%)
              </button>
              <button
                type="button"
                class="px-2.5 py-1 text-xs font-medium rounded-md bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-200 transition-colors"
                @click="sendSensors(dev.ieee, 28.0, 75.0)"
              >
                Warm/Humid (28°C, 75%)
              </button>
              <button
                type="button"
                class="px-2.5 py-1 text-xs font-medium rounded-md bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-200 transition-colors"
                @click="sendSensors(dev.ieee, 16.0, 35.0)"
              >
                Cold/Dry (16°C, 35%)
              </button>
            </div>
          </div>
        </div>
      </template>

      <!-- Empty state -->
      <div
        v-else
        class="col-span-full p-12 text-center bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl text-xs text-slate-400 dark:text-slate-500"
      >
        No simulated devices spawned yet. Click <strong>+ Spawn Simulated Device</strong> above to inject a virtual SONOFF SNZB-01P or other device.
      </div>
    </div>
  </div>
</template>
