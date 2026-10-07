<script setup lang="ts">
import { ref, watch } from 'vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import { useMode } from '@/composables/useMode'
import { useZigBridgeApi } from '@/composables/useZigBridgeApi'
import type { Device } from '@/types'

const props = defineProps<{
  show: boolean
  device: Device | null
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'saved', ieee: string, newName: string): void
}>()

const { isAdvanced } = useMode()
const api = useZigBridgeApi()

const friendlyName = ref('')
const isSubmitting = ref(false)
const errorMessage = ref<string | null>(null)

watch(
  () => props.device,
  (d) => {
    friendlyName.value = d?.friendly_name || ''
    errorMessage.value = null
  },
  { immediate: true }
)

async function handleSubmit() {
  if (!props.device) return
  isSubmitting.value = true
  errorMessage.value = null

  try {
    const trimmed = friendlyName.value.trim()
    await api.renameDevice(props.device.ieee, trimmed)
    emit('saved', props.device.ieee, trimmed)
    emit('close')
  } catch (err: any) {
    errorMessage.value = err?.message || 'Failed to rename device'
  } finally {
    isSubmitting.value = false
  }
}
</script>

<template>
  <ModalDialog :show="show" title="Rename Device" @close="emit('close')">
    <div v-if="device" class="space-y-4">
      <div v-if="errorMessage" class="p-3 text-xs text-red-700 bg-red-50 dark:bg-red-950/40 dark:text-red-300 rounded-md border border-red-200 dark:border-red-900">
        {{ errorMessage }}
      </div>

      <div v-if="isAdvanced" class="space-y-1.5">
        <label class="block text-xs font-medium text-slate-500 dark:text-slate-400">IEEE Address</label>
        <input
          type="text"
          :value="device.ieee"
          disabled
          class="w-full px-3 py-2 text-xs font-mono bg-slate-100 dark:bg-slate-800 text-slate-500 dark:text-slate-400 border border-slate-300 dark:border-slate-700 rounded-md cursor-not-allowed"
        />
      </div>

      <div class="space-y-1.5">
        <label for="rename-input" class="block text-xs font-medium text-slate-700 dark:text-slate-300">
          Friendly Name
        </label>
        <input
          id="rename-input"
          v-model="friendlyName"
          type="text"
          placeholder="e.g. Living Room Lamp, Kitchen Motion Sensor"
          class="w-full px-3 py-2 text-sm bg-white dark:bg-slate-900 border border-slate-300 dark:border-slate-700 rounded-md text-slate-800 dark:text-slate-200 focus:outline-none focus:ring-2 focus:ring-blue-500 dark:focus:ring-blue-400"
          @keyup.enter="handleSubmit"
        />
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
        :disabled="isSubmitting || !device"
        class="px-4 py-2 text-xs font-medium text-white bg-blue-600 hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed rounded-md transition-colors flex items-center space-x-1.5 shadow-sm"
        @click="handleSubmit"
      >
        <span v-if="isSubmitting">Saving...</span>
        <span v-else>Save Name</span>
      </button>
    </template>
  </ModalDialog>
</template>
