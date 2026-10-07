<script setup lang="ts">
import { ref } from 'vue'
import type { Recommendation, Device } from '@/types'
import { clusterName } from '@/utils/clusters'
import { useZigBridgeApi } from '@/composables/useZigBridgeApi'

const props = defineProps<{
  recommendations: Recommendation[]
  devices: Device[]
  coordinatorOnline: boolean
}>()

const emit = defineEmits<{
  (e: 'update:recommendations', recs: Recommendation[]): void
  (e: 'applied'): void
}>()

const api = useZigBridgeApi()
const isLoading = ref(false)
const isDisabledInConfig = ref(false)
const errorMessage = ref<string | null>(null)
const hasRun = ref(false)

function getDeviceName(ieee: string): string {
  const dev = props.devices.find(d => d.ieee === ieee)
  return dev?.friendly_name || ieee || 'Unknown'
}

async function runAnalysis() {
  isLoading.value = true
  errorMessage.value = null
  isDisabledInConfig.value = false
  hasRun.value = true

  try {
    const recs = await api.getRecommendations()
    emit('update:recommendations', recs || [])
  } catch (err: any) {
    if (err?.message?.includes('403')) {
      isDisabledInConfig.value = true
    } else {
      errorMessage.value = err?.message || 'Failed to inspect suggestions'
    }
  } finally {
    isLoading.value = false
  }
}

async function handleApply(recId: string) {
  if (!props.coordinatorOnline) {
    alert('Coordinator is offline. Cannot apply direct link suggestions.')
    return
  }

  try {
    await api.applyRecommendation(recId)
    alert('Direct link enabled successfully!')
    emit('applied')
    runAnalysis()
  } catch (err: any) {
    alert(`Failed to apply suggestion: ${err?.message || 'Unknown error'}`)
  }
}
</script>

<template>
  <div class="space-y-4">
    <!-- Header / Actions bar -->
    <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 p-4 rounded-xl shadow-xs">
      <div>
        <h2 class="text-sm font-bold text-slate-800 dark:text-slate-100">Smart Link Suggestions</h2>
        <p class="text-xs text-slate-500 dark:text-slate-400 mt-0.5">
          Analyzes network topology and device types to automatically suggest direct peer-to-peer bindings.
        </p>
      </div>
      <button
        type="button"
        :disabled="isLoading"
        class="px-3.5 py-2 text-xs font-semibold text-white bg-blue-600 hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed rounded-lg shadow-xs transition-colors shrink-0 flex items-center space-x-1.5"
        @click="runAnalysis"
      >
        <span v-if="isLoading">⚡ Finding Suggestions...</span>
        <span v-else>⚡ Find Suggestions</span>
      </button>
    </div>

    <!-- Disabled in config note -->
    <div
      v-if="isDisabledInConfig"
      class="p-4 bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl text-xs text-slate-500 dark:text-slate-400"
    >
      The AI recommendation engine is currently disabled in configuration (<code class="font-mono text-slate-700 dark:text-slate-300">ai.enabled: false</code>).
    </div>

    <!-- Error message -->
    <div
      v-else-if="errorMessage"
      class="p-4 bg-red-50 dark:bg-red-950/40 text-red-700 dark:text-red-300 border border-red-200 dark:border-red-900 rounded-xl text-xs"
    >
      Failed to inspect suggestions: {{ errorMessage }}
    </div>

    <!-- Initial state before search -->
    <div
      v-else-if="!hasRun && recommendations.length === 0"
      class="p-10 text-center bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl text-xs text-slate-400 dark:text-slate-500"
    >
      Click <strong>⚡ Find Suggestions</strong> to inspect your devices and propose autonomous direct links.
    </div>

    <!-- Empty results -->
    <div
      v-else-if="hasRun && recommendations.length === 0"
      class="p-10 text-center bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl text-xs text-slate-400 dark:text-slate-500"
    >
      No automatic suggestions detected yet. Connect a switch and a bulb to see autonomous link proposals.
    </div>

    <!-- Recommendations List -->
    <div v-else class="space-y-3">
      <div
        v-for="rec in recommendations"
        :key="rec.id"
        class="flex flex-col sm:flex-row sm:items-center justify-between p-4 bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl gap-4 shadow-xs"
      >
        <div class="flex-1 min-w-[280px]">
          <div class="flex items-center space-x-2 mb-1.5">
            <span class="px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wider rounded bg-purple-50 text-purple-700 dark:bg-purple-950/80 dark:text-purple-300 border border-purple-200 dark:border-purple-800">
              {{ rec.type }}
            </span>
            <span
              :class="[
                'px-2 py-0.5 text-[10px] font-semibold rounded border',
                Math.round(rec.confidence * 100) >= 80
                  ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/80 dark:text-emerald-300 border-emerald-200 dark:border-emerald-800'
                  : 'bg-amber-50 text-amber-700 dark:bg-amber-950/80 dark:text-amber-300 border-amber-200 dark:border-amber-800'
              ]"
            >
              Score: {{ Math.round(rec.confidence * 100) }}%
            </span>
            <span class="px-2 py-0.5 text-[10px] font-medium rounded bg-blue-50 text-blue-700 dark:bg-blue-950/80 dark:text-blue-300 border border-blue-200 dark:border-blue-800">
              {{ clusterName(rec.cluster_id) }}
            </span>
          </div>

          <div class="text-xs font-semibold text-slate-800 dark:text-slate-100 mb-1">
            {{ getDeviceName(rec.source_ieee) }} ➔ {{ getDeviceName(rec.target_ieee) }}
          </div>

          <div class="text-xs text-slate-500 dark:text-slate-400">
            {{ rec.reasoning }}
          </div>
        </div>

        <div class="shrink-0">
          <span
            v-if="rec.status === 'applied'"
            class="px-3 py-1.5 text-xs font-medium rounded-md bg-emerald-50 text-emerald-700 dark:bg-emerald-950/80 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800"
          >
            ✓ Applied
          </span>
          <button
            v-else
            type="button"
            :disabled="!coordinatorOnline"
            :title="!coordinatorOnline ? 'Coordinator offline: direct linking is disabled' : 'Enable Direct Link'"
            class="px-3.5 py-1.5 text-xs font-semibold text-white bg-emerald-600 hover:bg-emerald-700 disabled:opacity-50 disabled:cursor-not-allowed rounded-lg shadow-xs transition-colors"
            @click="handleApply(rec.id)"
          >
            Enable Direct Link
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
