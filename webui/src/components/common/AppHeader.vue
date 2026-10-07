<script setup lang="ts">
import { computed } from 'vue'
import StatusPill from '@/components/common/StatusPill.vue'
import { useMode } from '@/composables/useMode'
import type { StatusResponse } from '@/types'

const props = defineProps<{
  status: StatusResponse | null
}>()

const emit = defineEmits<{
  (e: 'navigate-home', event: MouseEvent): void
}>()

const { mode, isDark, toggleMode, toggleTheme } = useMode()

function onBrandClick(event: MouseEvent) {
  emit('navigate-home', event)
}

const isCoordinatorOnline = computed(() => {
  return Boolean(
    props.status?.connected &&
      (props.status.coordinator?.status === 'ready' || props.status.coordinator?.status === 'running')
  )
})

const coordinatorStatusType = computed(() => {
  if (isCoordinatorOnline.value) return 'connected'
  if (props.status?.transport_status === 'reconnecting') return 'warning'
  return 'disconnected'
})

const coordinatorTitle = computed(() => {
  if (isCoordinatorOnline.value) return 'Coordinator: Online'
  if (props.status?.transport_status === 'reconnecting') return 'Coordinator: Connecting...'
  return 'Coordinator: Offline'
})

const isMqttOnline = computed(() => Boolean(props.status?.mqtt_connected))
</script>

<template>
  <header
    class="bg-white dark:bg-[#161b22] border-b border-gray-200 dark:border-[#30363d] px-6 py-3.5 sticky top-0 z-40 transition-colors"
  >
    <div class="max-w-[1300px] mx-auto flex items-center justify-between flex-wrap gap-4">
      <!-- Brand -->
      <a
        href="/"
        class="flex items-center gap-3 group cursor-pointer focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 rounded-sm"
        title="Zigbridge - Home"
        @click="onBrandClick"
      >
        <h1 class="text-lg font-bold tracking-tight text-gray-900 dark:text-gray-100 group-hover:text-blue-600 dark:group-hover:text-blue-400 transition-colors">
          Zigbridge
        </h1>
      </a>

      <!-- Status & Controls -->
      <div class="flex items-center gap-3 flex-wrap">
        <!-- Coordinator Status -->
        <StatusPill
          label="Coordinator"
          :status="coordinatorStatusType"
          :title="coordinatorTitle"
        />

        <!-- MQTT Status -->
        <StatusPill
          label="MQTT"
          :status="isMqttOnline ? 'connected' : 'disconnected'"
          :title="isMqttOnline ? 'MQTT: Connected' : 'MQTT: Offline / Disabled'"
        />

        <!-- Theme Toggle -->
        <button
          type="button"
          class="inline-flex items-center justify-center w-8 h-8 rounded-full border border-gray-200 dark:border-[#30363d] bg-gray-50 dark:bg-[#21262d] text-gray-600 dark:text-gray-300 hover:text-gray-900 dark:hover:text-white transition-colors"
          :title="isDark ? 'Switch to Light Theme' : 'Switch to Dark Theme'"
          @click="toggleTheme"
        >
          <span v-if="isDark" class="text-sm">☀️</span>
          <span v-else class="text-sm">🌙</span>
        </button>

        <!-- Advance Mode Switch -->
        <button
          type="button"
          class="inline-flex items-center gap-2 px-3 py-1.5 rounded-full border text-xs font-semibold transition-all select-none"
          :class="
            mode === 'advanced'
              ? 'bg-purple-500/15 border-purple-500/40 text-purple-600 dark:text-purple-300'
              : 'bg-gray-100 dark:bg-[#21262d] border-gray-200 dark:border-[#30363d] text-gray-600 dark:text-gray-400 hover:border-gray-300 dark:hover:border-gray-600'
          "
          title="Toggle between Simple and Advance technical modes"
          @click="toggleMode"
        >
          <span
            class="px-1.5 py-0.5 rounded-full text-[10px] uppercase font-bold"
            :class="
              mode === 'advanced'
                ? 'bg-purple-600 text-white'
                : 'bg-gray-200 dark:bg-[#30363d] text-gray-500 dark:text-gray-400'
            "
          >
            {{ mode === 'advanced' ? 'ON' : 'OFF' }}
          </span>
          <span>Advance</span>
        </button>
      </div>
    </div>
  </header>
</template>
