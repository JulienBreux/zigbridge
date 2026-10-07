<script setup lang="ts">
import type { Binding, Device } from '@/types'
import { useMode } from '@/composables/useMode'
import { useZigbridgeApi } from '@/composables/useZigbridgeApi'
import { clusterName } from '@/utils/clusters'

const props = defineProps<{
  bindings: Binding[]
  devices: Device[]
  coordinatorOnline: boolean
}>()

const emit = defineEmits<{
  (e: 'create'): void
  (e: 'refresh'): void
}>()

const { isAdvanced } = useMode()
const api = useZigbridgeApi()

function getDeviceName(ieee: string): string {
  const dev = props.devices.find(d => d.ieee === ieee)
  return dev?.friendly_name || ieee || 'Unknown'
}

async function handleUnlink(b: Binding) {
  if (!props.coordinatorOnline) {
    alert('Coordinator is offline. Cannot remove direct links.')
    return
  }

  const srcName = getDeviceName(b.src_ieee)
  const dstName = getDeviceName(b.dst_ieee)
  if (!confirm(`Are you sure you want to remove the direct link between ${srcName} and ${dstName}?`)) {
    return
  }

  try {
    await api.deleteBinding({
      source_ieee: b.src_ieee,
      source_ep: b.src_endpoint,
      target_ieee: b.dst_ieee,
      target_ep: b.dst_endpoint,
      cluster_id: b.cluster_id
    })
    emit('refresh')
  } catch (err: any) {
    alert(`Failed to remove link: ${err?.message || 'Unknown error'}`)
  }
}
</script>

<template>
  <div class="space-y-4">
    <!-- Header / Actions bar -->
    <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 p-4 rounded-xl shadow-xs">
      <div>
        <h2 class="text-sm font-bold text-slate-800 dark:text-slate-100">Direct Bindings</h2>
        <p class="text-xs text-slate-500 dark:text-slate-400 mt-0.5">
          Direct bindings allow switches and remotes to control smart lights directly with instantaneous response—even if the coordinator, server, or Wi-Fi is powered off.
        </p>
      </div>
      <button
        type="button"
        :disabled="!coordinatorOnline"
        :title="!coordinatorOnline ? 'Coordinator offline: creating direct links is disabled' : 'Create direct link'"
        class="px-3.5 py-2 text-xs font-semibold text-white bg-blue-600 hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed rounded-lg shadow-xs transition-colors shrink-0 flex items-center space-x-1"
        @click="emit('create')"
      >
        <span>+ Create Device Link</span>
      </button>
    </div>

    <!-- Bindings Table -->
    <div class="bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl overflow-hidden shadow-xs">
      <div class="overflow-x-auto">
        <table class="w-full text-left border-collapse">
          <thead>
            <tr class="bg-slate-50/75 dark:bg-slate-800/60 border-b border-slate-200 dark:border-slate-800 text-[11px] font-semibold uppercase tracking-wider text-slate-500 dark:text-slate-400">
              <th class="px-4 py-3">Source Device (Switch/Sensor)</th>
              <th v-if="isAdvanced" class="px-4 py-3">Src EP</th>
              <th class="px-4 py-3">Control Action / Cluster</th>
              <th class="px-4 py-3">Target Device (Light/Plug)</th>
              <th v-if="isAdvanced" class="px-4 py-3">Target EP</th>
              <th class="px-4 py-3 text-right">Actions</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-100 dark:divide-slate-800/80">
            <template v-if="bindings.length > 0">
              <tr
                v-for="(b, idx) in bindings"
                :key="`${b.src_ieee}-${b.src_endpoint}-${b.dst_ieee}-${b.dst_endpoint}-${b.cluster_id}-${idx}`"
                class="hover:bg-slate-50/60 dark:hover:bg-slate-800/40 transition-colors"
              >
                <!-- Source Device -->
                <td class="px-4 py-3.5 whitespace-nowrap">
                  <div class="text-xs font-semibold text-slate-800 dark:text-slate-100">
                    {{ getDeviceName(b.src_ieee) }}
                  </div>
                  <div class="font-mono text-[11px] text-slate-400 dark:text-slate-500">
                    {{ b.src_ieee }}
                  </div>
                </td>

                <!-- Src EP (Advanced) -->
                <td v-if="isAdvanced" class="px-4 py-3.5 whitespace-nowrap font-mono text-xs text-slate-600 dark:text-slate-300">
                  {{ b.src_endpoint }}
                </td>

                <!-- Control Action / Cluster -->
                <td class="px-4 py-3.5 whitespace-nowrap text-xs">
                  <span class="inline-block px-2 py-0.5 rounded text-[11px] font-medium bg-purple-50 text-purple-700 dark:bg-purple-950/80 dark:text-purple-300 border border-purple-200 dark:border-purple-800">
                    {{ clusterName(b.cluster_id) }}
                  </span>
                  <span v-if="isAdvanced" class="ml-1.5 font-mono text-[11px] text-slate-400 dark:text-slate-500">
                    0x{{ Number(b.cluster_id).toString(16).toUpperCase().padStart(4, '0') }}
                  </span>
                </td>

                <!-- Target Device -->
                <td class="px-4 py-3.5 whitespace-nowrap">
                  <div class="text-xs font-semibold text-slate-800 dark:text-slate-100">
                    {{ getDeviceName(b.dst_ieee) }}
                  </div>
                  <div class="font-mono text-[11px] text-slate-400 dark:text-slate-500">
                    {{ b.dst_ieee }}
                  </div>
                </td>

                <!-- Target EP (Advanced) -->
                <td v-if="isAdvanced" class="px-4 py-3.5 whitespace-nowrap font-mono text-xs text-slate-600 dark:text-slate-300">
                  {{ b.dst_endpoint }}
                </td>

                <!-- Actions -->
                <td class="px-4 py-3.5 whitespace-nowrap text-right">
                  <button
                    type="button"
                    :disabled="!coordinatorOnline"
                    :title="!coordinatorOnline ? 'Coordinator offline: unlinking is disabled' : 'Unlink devices'"
                    class="px-2.5 py-1 text-xs font-medium text-rose-700 dark:text-rose-300 bg-rose-50 dark:bg-rose-950/60 hover:bg-rose-100 dark:hover:bg-rose-900/60 border border-rose-200 dark:border-rose-900 rounded-md transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                    @click="handleUnlink(b)"
                  >
                    Unlink
                  </button>
                </td>
              </tr>
            </template>
            <tr v-else>
              <td
                :colspan="isAdvanced ? 6 : 4"
                class="px-4 py-12 text-center text-xs text-slate-400 dark:text-slate-500"
              >
                No direct bindings configured. Click <strong>+ Create Device Link</strong> to pair a switch directly to a bulb.
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>
