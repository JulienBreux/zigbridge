<script setup lang="ts">
import { ref, computed } from 'vue'
import type { Device } from '@/types'
import { useMode } from '@/composables/useMode'
import DeviceRow from '@/components/devices/DeviceRow.vue'

const props = defineProps<{
  devices: Device[]
  permitJoinRemaining: number
  coordinatorOnline: boolean
}>()

const emit = defineEmits<{
  (e: 'selectDevice', ieee: string): void
  (e: 'openPermitJoin'): void
}>()

const { isAdvanced } = useMode()
const searchQuery = ref('')

const filteredDevices = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return props.devices
  return props.devices.filter(d =>
    (d.friendly_name || '').toLowerCase().includes(q) ||
    (d.ieee || '').toLowerCase().includes(q) ||
    (d.model || '').toLowerCase().includes(q) ||
    (d.manufacturer || '').toLowerCase().includes(q)
  )
})
</script>

<template>
  <div class="space-y-4">
    <!-- Actions Bar -->
    <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
      <!-- Search Input -->
      <div class="relative flex-1 max-w-md">
        <input
          v-model="searchQuery"
          type="text"
          placeholder="Search devices by name, model, or room..."
          class="w-full pl-9 pr-3 py-2 text-xs bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-lg text-slate-800 dark:text-slate-200 placeholder-slate-400 focus:outline-none focus:ring-2 focus:ring-blue-500"
        />
        <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none text-slate-400">
          <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
          </svg>
        </div>
      </div>

      <!-- Add / Permit Join Button -->
      <div class="flex items-center space-x-2">
        <button
          type="button"
          :disabled="!coordinatorOnline"
          :title="!coordinatorOnline ? 'Coordinator offline: pairing disabled' : 'Open network to pair new devices'"
          class="px-3.5 py-2 text-xs font-semibold text-white bg-blue-600 hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed rounded-lg shadow-xs transition-colors flex items-center space-x-1.5 shrink-0"
          @click="emit('openPermitJoin')"
        >
          <svg viewBox="0 0 16 16" width="13" height="13" fill="currentColor" aria-hidden="true">
            <path d="M7.75 2a.75.75 0 0 1 .75.75V7h4.25a.75.75 0 0 1 0 1.5H8.5v4.25a.75.75 0 0 1-1.5 0V8.5H2.75a.75.75 0 0 1 0-1.5H7V2.75A.75.75 0 0 1 7.75 2Z" />
          </svg>
          <span>Add</span>
          <span
            v-if="permitJoinRemaining > 0"
            class="ml-1 px-1.5 py-0.2 text-[10px] font-bold bg-amber-400 text-amber-950 rounded-full animate-pulse"
          >
            ({{ permitJoinRemaining }}s)
          </span>
        </button>
      </div>
    </div>

    <!-- Devices Table -->
    <div class="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl overflow-hidden shadow-xs">
      <div class="overflow-x-auto">
        <table class="w-full text-left border-collapse">
          <thead>
            <tr class="bg-slate-50/75 dark:bg-slate-800/60 border-b border-slate-200 dark:border-slate-800 text-[11px] font-semibold uppercase tracking-wider text-slate-500 dark:text-slate-400">
              <th class="px-4 py-3">Device</th>
              <th class="px-4 py-3">Type</th>
              <th class="px-4 py-3">Status / Power</th>
              <th class="px-4 py-3">Battery</th>
              <th class="px-4 py-3">Signal</th>
              <th v-if="isAdvanced" class="px-4 py-3">IEEE Address</th>
              <th v-if="isAdvanced" class="px-4 py-3">NWK</th>
              <th v-if="isAdvanced" class="px-4 py-3">Endpoints &amp; Clusters</th>
              <th v-if="isAdvanced" class="px-4 py-3">LQI</th>
              <th v-if="isAdvanced" class="px-4 py-3">Last Seen</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-100 dark:divide-slate-800/80">
            <template v-if="filteredDevices.length > 0">
              <DeviceRow
                v-for="dev in filteredDevices"
                :key="dev.ieee"
                :device="dev"
                :is-advanced="isAdvanced"
                @select="emit('selectDevice', $event)"
              />
            </template>
            <tr v-else>
              <td
                :colspan="isAdvanced ? 10 : 5"
                class="px-4 py-12 text-center text-xs text-slate-400 dark:text-slate-500"
              >
                <div v-if="searchQuery">
                  No devices match "<strong>{{ searchQuery }}</strong>".
                </div>
                <div v-else>
                  No devices paired yet. Click <strong>+ Add</strong> above to connect smart switches, lights, or sensors.
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>
