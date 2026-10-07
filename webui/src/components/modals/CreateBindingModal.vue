<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import { useMode } from '@/composables/useMode'
import { useZigBridgeApi } from '@/composables/useZigBridgeApi'
import { sortDevicesByFriendlyName, getDeviceDisplayName } from '@/utils/formatters'
import type { Device } from '@/types'

const props = defineProps<{
  show: boolean
  devices: Device[]
  coordinatorOnline: boolean
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'created'): void
}>()

const { isAdvanced } = useMode()
const api = useZigBridgeApi()

const sortedDevices = computed(() => sortDevicesByFriendlyName([...props.devices]))

const sourceIeee = ref('')
const sourceEp = ref(1)
const targetIeee = ref('')
const targetEp = ref(1)
const clusterId = ref(6)

const isSubmitting = ref(false)
const errorMessage = ref<string | null>(null)
const warningMessage = ref<string | null>(null)

// Watch when modal is opened or devices change
watch(
  () => props.show,
  (open) => {
    if (open) {
      errorMessage.value = null
      warningMessage.value = null
      if (sortedDevices.value.length > 0) {
        if (!sourceIeee.value || !sortedDevices.value.find(d => d.ieee === sourceIeee.value)) {
          sourceIeee.value = sortedDevices.value[0].ieee
        }
        if (!targetIeee.value || !sortedDevices.value.find(d => d.ieee === targetIeee.value)) {
          targetIeee.value = sortedDevices.value.length > 1 ? sortedDevices.value[1].ieee : sortedDevices.value[0].ieee
        }
      }
    }
  }
)

const currentSourceDevice = computed(() => props.devices.find(d => d.ieee === sourceIeee.value))
const currentTargetDevice = computed(() => props.devices.find(d => d.ieee === targetIeee.value))

const sourceEndpoints = computed(() => {
  const eps = currentSourceDevice.value?.endpoints
  return eps && eps.length > 0 ? eps : [1]
})

const targetEndpoints = computed(() => {
  const eps = currentTargetDevice.value?.endpoints
  return eps && eps.length > 0 ? eps : [1]
})

watch(sourceEndpoints, (eps) => {
  if (!eps.includes(sourceEp.value)) {
    sourceEp.value = eps[0] || 1
  }
}, { immediate: true })

watch(targetEndpoints, (eps) => {
  if (!eps.includes(targetEp.value)) {
    targetEp.value = eps[0] || 1
  }
}, { immediate: true })

const isOptimistic = computed(() => {
  const dev = currentTargetDevice.value
  return !dev || !dev.endpoints || dev.endpoints.length === 0
})

async function handleSubmit() {
  if (!props.coordinatorOnline) {
    errorMessage.value = 'Coordinator is offline. Cannot create direct links.'
    return
  }
  if (!sourceIeee.value || !targetIeee.value) {
    errorMessage.value = 'Please select both source and target devices.'
    return
  }

  isSubmitting.value = true
  errorMessage.value = null
  warningMessage.value = null

  try {
    const res = await api.createBinding({
      source_ieee: sourceIeee.value,
      source_ep: Number(sourceEp.value),
      target_ieee: targetIeee.value,
      target_ep: Number(targetEp.value),
      cluster_id: Number(clusterId.value)
    })

    if (res.warning) {
      alert(`Link established with note:\n${res.warning}`)
    }

    emit('created')
    emit('close')
  } catch (err: any) {
    errorMessage.value = err?.message || 'Failed to create link'
  } finally {
    isSubmitting.value = false
  }
}
</script>

<template>
  <ModalDialog :show="show" title="Create Direct Device Link" @close="emit('close')">
    <div class="space-y-4">
      <p class="text-xs text-slate-500 dark:text-slate-400">
        Direct binding links a switch or sensor directly to a smart light or plug over the air with zero hub latency.
      </p>

      <div v-if="errorMessage" class="p-3 text-xs text-red-700 bg-red-50 dark:bg-red-950/40 dark:text-red-300 rounded-md border border-red-200 dark:border-red-900">
        {{ errorMessage }}
      </div>

      <!-- Source Device -->
      <div class="space-y-1.5">
        <label for="bind-src-device" class="block text-xs font-medium text-slate-700 dark:text-slate-300">
          Source Device (Controller/Switch)
        </label>
        <select
          id="bind-src-device"
          v-model="sourceIeee"
          class="w-full px-3 py-2 text-xs bg-white dark:bg-slate-900 border border-slate-300 dark:border-slate-700 rounded-md text-slate-800 dark:text-slate-200 focus:outline-none focus:ring-2 focus:ring-blue-500"
        >
          <option v-for="dev in sortedDevices" :key="dev.ieee" :value="dev.ieee">
            {{ getDeviceDisplayName(dev) }} ({{ dev.ieee }})
          </option>
        </select>
      </div>

      <!-- Source Endpoint (Advanced) -->
      <div v-if="isAdvanced" class="space-y-1.5">
        <label for="bind-src-ep" class="block text-xs font-medium text-slate-700 dark:text-slate-300">
          Source Endpoint
        </label>
        <select
          id="bind-src-ep"
          v-model="sourceEp"
          class="w-full px-3 py-2 text-xs bg-white dark:bg-slate-900 border border-slate-300 dark:border-slate-700 rounded-md text-slate-800 dark:text-slate-200 focus:outline-none focus:ring-2 focus:ring-blue-500"
        >
          <option v-for="ep in sourceEndpoints" :key="ep" :value="ep">{{ ep }}</option>
        </select>
      </div>

      <!-- Target Device -->
      <div class="space-y-1.5">
        <label for="bind-target-device" class="block text-xs font-medium text-slate-700 dark:text-slate-300">
          Target Device (Light/Appliance)
        </label>
        <select
          id="bind-target-device"
          v-model="targetIeee"
          class="w-full px-3 py-2 text-xs bg-white dark:bg-slate-900 border border-slate-300 dark:border-slate-700 rounded-md text-slate-800 dark:text-slate-200 focus:outline-none focus:ring-2 focus:ring-blue-500"
        >
          <option v-for="dev in sortedDevices" :key="dev.ieee" :value="dev.ieee">
            {{ getDeviceDisplayName(dev) }} ({{ dev.ieee }})
          </option>
        </select>
      </div>

      <!-- Target Endpoint (Advanced) -->
      <div v-if="isAdvanced" class="space-y-1.5">
        <label for="bind-target-ep" class="block text-xs font-medium text-slate-700 dark:text-slate-300">
          Target Endpoint
        </label>
        <select
          id="bind-target-ep"
          v-model="targetEp"
          class="w-full px-3 py-2 text-xs bg-white dark:bg-slate-900 border border-slate-300 dark:border-slate-700 rounded-md text-slate-800 dark:text-slate-200 focus:outline-none focus:ring-2 focus:ring-blue-500"
        >
          <option v-for="ep in targetEndpoints" :key="ep" :value="ep">{{ ep }}</option>
        </select>
      </div>

      <!-- Cluster / Action -->
      <div class="space-y-1.5">
        <label for="bind-cluster" class="block text-xs font-medium text-slate-700 dark:text-slate-300">
          Action / Cluster
        </label>
        <select
          id="bind-cluster"
          v-model="clusterId"
          class="w-full px-3 py-2 text-xs bg-white dark:bg-slate-900 border border-slate-300 dark:border-slate-700 rounded-md text-slate-800 dark:text-slate-200 focus:outline-none focus:ring-2 focus:ring-blue-500"
        >
          <option :value="6">Turn On / Off (Cluster 0x0006)</option>
          <option :value="8">Dimming / Brightness (Cluster 0x0008)</option>
          <option :value="768">Color Control (Cluster 0x0300)</option>
        </select>
      </div>

      <!-- Optimistic link notice -->
      <div
        v-if="isOptimistic"
        class="p-2.5 text-xs text-amber-800 dark:text-amber-200 bg-amber-50 dark:bg-amber-950/40 border border-amber-200 dark:border-amber-800 rounded-md"
      >
        ⚠️ Optimistic Link: Target device endpoints or clusters are not yet fully discovered. The binding will be registered with a warning.
      </div>
    </div>

    <template #footer>
      <button
        type="button"
        class="px-4 py-2 text-xs font-medium text-slate-700 dark:text-slate-300 bg-slate-100 dark:bg-slate-800 hover:bg-slate-200 dark:hover:bg-slate-700 rounded-md transition-colors"
        @click="emit('close')"
      >
        Cancel
      </button>
      <button
        type="button"
        :disabled="isSubmitting || !coordinatorOnline || !sourceIeee || !targetIeee"
        class="px-4 py-2 text-xs font-medium text-white bg-blue-600 hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed rounded-md transition-colors flex items-center space-x-1.5 shadow-sm"
        @click="handleSubmit"
      >
        <span v-if="isSubmitting">Creating...</span>
        <span v-else>Establish Link</span>
      </button>
    </template>
  </ModalDialog>
</template>
