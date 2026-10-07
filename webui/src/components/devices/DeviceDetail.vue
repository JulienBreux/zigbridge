<script setup lang="ts">
import { ref, computed } from 'vue'
import type { Device, DeviceDefinition } from '@/types'
import { inferDeviceType } from '@/utils/formatters'
import ExposesView from '@/components/devices/ExposesView.vue'
import AboutView from '@/components/devices/AboutView.vue'

const props = defineProps<{
  device: Device
  definition?: DeviceDefinition | null
  coordinatorOnline: boolean
}>()

const emit = defineEmits<{
  (e: 'back'): void
  (e: 'rename', device: Device): void
  (e: 'updated'): void
}>()

const currentSubtab = ref<'exposes' | 'about'>('exposes')

const typeInfo = computed(() => inferDeviceType(props.device))
const isBattery = computed(() => props.device.battery !== undefined && props.device.battery !== null)

const lastSeenText = computed(() => {
  if (!props.device.last_seen) return 'Never'
  try {
    return new Date(props.device.last_seen).toLocaleTimeString()
  } catch {
    return 'Never'
  }
})
</script>

<template>
  <div class="space-y-4">
    <!-- Header Card -->
    <div class="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl p-5 shadow-xs">
      <!-- Back & Actions bar -->
      <div class="flex items-center justify-between pb-4 border-b border-slate-100 dark:border-slate-800">
        <button
          type="button"
          class="inline-flex items-center text-xs font-medium text-slate-600 dark:text-slate-400 hover:text-blue-600 dark:hover:text-blue-400 transition-colors"
          @click="emit('back')"
        >
          ← Back to Devices
        </button>

        <button
          type="button"
          class="px-3 py-1.5 text-xs font-medium bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-200 rounded-md transition-colors flex items-center space-x-1"
          @click="emit('rename', device)"
        >
          <span>✏️</span>
          <span>Rename</span>
        </button>
      </div>

      <!-- Main device identity -->
      <div class="flex items-center space-x-4 pt-4">
        <div class="w-14 h-14 rounded-xl bg-slate-100 dark:bg-slate-800 flex items-center justify-center text-2xl shrink-0">
          {{ typeInfo.icon }}
        </div>

        <div class="flex-1 min-w-0">
          <div class="flex flex-wrap items-center gap-2">
            <h2 class="text-base font-bold text-slate-800 dark:text-slate-100 truncate">
              {{ device.friendly_name || device.ieee }}
            </h2>
            <span class="px-2 py-0.5 text-[11px] font-medium rounded-full bg-purple-50 text-purple-700 dark:bg-purple-950/80 dark:text-purple-300 border border-purple-200 dark:border-purple-800">
              {{ device.model || definition?.device?.model || 'Generic Device' }}
            </span>
            <span
              :class="[
                'px-2 py-0.5 text-[11px] font-medium rounded-full border',
                isBattery
                  ? 'bg-blue-50 text-blue-700 dark:bg-blue-950/80 dark:text-blue-300 border-blue-200 dark:border-blue-800'
                  : 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/80 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800'
              ]"
            >
              {{ isBattery ? `🔋 ${device.battery}%` : '⚡ Mains' }}
            </span>
          </div>

          <div class="flex flex-wrap items-center gap-2 text-xs text-slate-500 dark:text-slate-400 mt-1">
            <span class="font-mono">{{ device.ieee }}</span>
            <span>•</span>
            <span>{{ device.manufacturer || definition?.device?.vendor || 'Zigbee Device' }}</span>
            <span>•</span>
            <span>LQI: {{ device.lqi || 0 }} / 255</span>
            <span>•</span>
            <span>Last Seen: {{ lastSeenText }}</span>
          </div>
        </div>
      </div>

      <!-- Subtab Buttons -->
      <div class="flex items-center space-x-2 mt-6 pt-4 border-t border-slate-100 dark:border-slate-800">
        <button
          type="button"
          :class="[
            'px-3.5 py-1.5 text-xs font-semibold rounded-lg transition-colors',
            currentSubtab === 'exposes'
              ? 'bg-blue-50 text-blue-700 dark:bg-blue-950/60 dark:text-blue-300'
              : 'text-slate-600 dark:text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800'
          ]"
          @click="currentSubtab = 'exposes'"
        >
          🎛️ Exposes &amp; Controls
        </button>
        <button
          type="button"
          :class="[
            'px-3.5 py-1.5 text-xs font-semibold rounded-lg transition-colors',
            currentSubtab === 'about'
              ? 'bg-blue-50 text-blue-700 dark:bg-blue-950/60 dark:text-blue-300'
              : 'text-slate-600 dark:text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-800'
          ]"
          @click="currentSubtab = 'about'"
        >
          ℹ️ About &amp; Diagnostics
        </button>
      </div>
    </div>

    <!-- Subtab Content -->
    <div>
      <ExposesView
        v-if="currentSubtab === 'exposes'"
        :device="device"
        :definition="definition"
        :coordinator-online="coordinatorOnline"
        @updated="emit('updated')"
      />
      <AboutView
        v-else-if="currentSubtab === 'about'"
        :device="device"
        :definition="definition"
      />
    </div>
  </div>
</template>
